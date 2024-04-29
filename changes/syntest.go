// Copyright 2024 Cisco Systems, Inc. and its affiliates
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//
// SPDX-License-Identifier: Apache-2.0

package pluginmanager

import (
	"context"
	"crypto/md5"
	"fmt"
	"github.com/cisco-open/synthetic-heart/agent/utils"
	"github.com/cisco-open/synthetic-heart/common"
	"github.com/cisco-open/synthetic-heart/common/proto"
	"github.com/docker/distribution/uuid"
	"github.com/hashicorp/go-hclog"
	"github.com/hashicorp/go-plugin"
	"github.com/pkg/errors"
	"io"
	"io/ioutil"
	"math/rand"
	"os"
	"os/exec"
	"strings"
	"time"
)

// SynTestRoutine handles communication between the synthetic heart binary and a synthetic test plugin
type SynTestRoutine struct {
	agentId         string
	config          proto.SynTestConfig
	plugin          plugin.Plugin
	broadcaster     *utils.Broadcaster
	storageHandler  *ExtStorageHandler
	logger          hclog.Logger
	logWaitTime     time.Duration
	printPluginLogs PrintPluginLogOption
}

func (str *SynTestRoutine) Run(ctx context.Context) error {
	// Initialise the routine
	str.logger = hclog.New(&hclog.LoggerOptions{
		Name:            "pm." + str.config.Name + ".routine",
		Level:           hclog.LevelFromString(os.Getenv("LOG_LEVEL")),
		Color:           hclog.ForceColor,
		IncludeLocation: true,
	})

	// Parse timeouts and duration
	initTimeout, err := time.ParseDuration(str.config.Timeouts.Init)
	if err != nil {
		str.logger.Warn("error parsing init timeout duration, using default", "err", err, "default", common.DefaultInitTimeout.String())
		initTimeout = common.DefaultInitTimeout
	}
	testTimeout, err := time.ParseDuration(str.config.Timeouts.Run)
	if err != nil {
		str.logger.Warn("error parsing test timeout duration, using default", "err", err, "default", common.DefaultRunTimeout.String())
		testTimeout = common.DefaultRunTimeout
	}
	finishTimeout, err := time.ParseDuration(str.config.Timeouts.Finish)
	if err != nil {
		str.logger.Warn("error parsing finish timeout duration, using default", "err", err, "default", common.DefaultFinishTimeout.String())
		finishTimeout = common.DefaultFinishTimeout
	}
	logWaitTime, err := time.ParseDuration(str.config.LogWaitTime)
	if err != nil {
		str.logger.Warn("error parsing logWaitTime duration, using default", "err", err, "default", common.DefaultLogWaitTime.String())
		logWaitTime = common.DefaultLogWaitTime
	}
	str.logWaitTime = logWaitTime
	testRepeatDuration, err := time.ParseDuration(str.config.Repeat)
	if err != nil {
		return errors.Wrap(err, "error parsing repeat duration")
	}

	// Iterate over triggers and set defaults
	dependantTestMap := map[string]bool{}
	for _, dependsOnTest := range str.config.DependsOn {
		dependantTestMap[dependsOnTest] = true
	}

	// Create a channel on which we get timer ticks
	var timerChan <-chan time.Time
	if testRepeatDuration > 0 {
		ticker := time.NewTicker(testRepeatDuration)
		timerChan = ticker.C

		// Add a bit of jitter, to prevent repeated storms of tests
		jitter := rand.Intn(common.MaxSynTestTimerJitter) // 0 - 10 seconds of jitter
		str.logger.Debug(fmt.Sprintf("sleeping for jitter: %dms", jitter))
		time.Sleep(time.Duration(jitter) * time.Millisecond)
	}

	// Create a channel on which we get other test runs
	var testRunChan chan proto.TestRun
	if len(dependantTestMap) > 0 {
		testRunChan = str.broadcaster.SubscribeToTestRuns(str.config.Name, common.DefaultSynTestSubChannelSize, str.logger)
		defer str.broadcaster.UnsubscribeFromTestRuns(testRunChan, str.logger)
	}

	for {
		str.logger.Debug("state of channels", "atStart", timerChan, "test", testRunChan)
		select {
		case <-timerChan: // Watch for ticker
			if str.isCtxCancelled(ctx) { // Check if ctx is cancelled before proceeding (this is to maintain priority of cancel signal if >1 channels are ready)
				return nil
			}
			err := str.testPlugin(ctx, initTimeout, testTimeout, finishTimeout)
			if err != nil {
				return err
			}
		case testRun := <-testRunChan: // Watch for other test runs
			// only run for the tests that the plugin cares about
			if dependantTestMap["*"] || dependantTestMap["*++"] || dependantTestMap[testRun.TestConfig.Name] {
				// buffer to store plugin logs
				if str.isCtxCancelled(ctx) { // Check if ctx is cancelled before proceeding (this is to maintain priority of cancel signal  if >1 channels are ready)
					return nil
				}
				err := str.testPlugin(ctx, initTimeout, testTimeout, finishTimeout)
				if err != nil {
					return err
				}
			}
		case <-ctx.Done(): // Watch for cancellation signal
			str.logger.Info("kill signal received")
			return nil
		}

		if timerChan == nil && testRunChan == nil {
			str.logger.Info("all trigger channels are nil")
			return nil
		}
	}
}

func (str *SynTestRoutine) isCtxCancelled(ctx context.Context) bool {
	select {
	case <-ctx.Done():
		str.logger.Info("kill signal received")
		return true
	default:
	}
	return false
}

// Does a test run with a timeout
// this is similar to running performTest() using runFuncWithTimeout, but this needs to return a proto.TestRun which is why runFuncWithTimeout is not used
func (str *SynTestRoutine) runTest(ctx context.Context, st common.SynTestPlugin, triggerInfo proto.Trigger, timeout time.Duration, pluginLogs *utils.Buffer) error {
	s := time.Now()
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	str.logger.Info("performing test", "test", str.config.Name, "trigger", triggerInfo.TriggerType, "timeout", timeout)
	t, testErr := str.performTest(ctx, st, triggerInfo)

	// if details map doesnt exist, initialise it
	if t.Details == nil {
		t.Details = map[string]string{}
	}

	// Add the error in the test run
	if testErr != nil {
		if testErr == context.DeadlineExceeded {
			testErr = errors.Wrap(testErr, "test hit timeout")
		}
		t.Details[common.ErrorKey] = testErr.Error()
	}

	// Wait for a bit for logs to be streamed
	if testErr != nil || t.TestResult.Marks < t.TestResult.MaxMarks {
		str.logger.Info("error: sleeping for logs", "time", "250ms")
		// Longer Sleep to allow the logs from panic and other issues to be salvaged
		time.Sleep(250 * time.Millisecond)
	} else {
		str.logger.Info("sleeping for logs", "time", str.logWaitTime)
		time.Sleep(str.logWaitTime)
	}
	logs, err := ioutil.ReadAll(pluginLogs) // Read the logs
	if err != nil {
		logs = []byte("<unable to fetch logs>")
	}

	// if the test errored, print the logs
	switch str.printPluginLogs {
	case LogAlways:
		str.printLogsFromPlugin(string(logs))
	case LogOnFail:
		if testErr != nil || t.TestResult.Marks < t.TestResult.MaxMarks {
			str.printLogsFromPlugin(string(logs))
		}
	}

	// Add the logs from the plugin
	t.Details[common.LogKey] = string(logs)
	t.AgentId = str.agentId
	str.broadcaster.PublishTestRun(t, str.logger)
	e := time.Now()
	str.logger.Info("handling test took", "time", e.Sub(s).String())
	return testErr
}

func (str *SynTestRoutine) finish(ctx context.Context, plugin common.SynTestPlugin, timeout time.Duration) {
	err := str.runFuncWithTimeout(ctx, timeout, "finish", func(errCh chan error) {
		defer str.panicHandler("finish")
		errCh <- plugin.Finish()
	})
	if err != nil {
		str.logger.Error("plugin error when finishing", "err", err)
		return
	}
}

func (str *SynTestRoutine) printLogsFromPlugin(logs string) {
	logger := hclog.New(&hclog.LoggerOptions{
		Name:        "",
		Level:       hclog.Warn,
		DisableTime: true,
		Color:       hclog.ColorOff,
	})
	prefix := "plugin." + str.computePluginIdHash() + ": " // we use a short hash of the plugin id so its identifiable in aggregated logging
	l := strings.ReplaceAll(logs, "\n", "\n"+prefix)
	logger.Warn(prefix + l)
}

func (str *SynTestRoutine) computePluginIdHash() string {
	pluginId := common.ComputePluginId(str.agentId, str.config.Name)
	fullHash := fmt.Sprintf("%08x", md5.Sum([]byte(pluginId)))
	return fullHash[:8]
}

// Runs a function (for type func(chan error)) with timeout
// Takes a context, a timeout, the function name (for logging purposes) and the function itself (which must have an error channel as a param)
// The error channel inside the function can be used to pass errors back or nil can be provided to indicate successful run
// The error passed to the error channel will be returned as an error for this function
// It will block until the provided function finishes executing
func (str *SynTestRoutine) runFuncWithTimeout(ctx context.Context, timeout time.Duration, fName string, function func(chan error)) (err error) {
	str.logger.Info("running "+fName, "timeout", timeout)
	errCh := make(chan error, 1)

	tCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	go func() {
		function(errCh)
		close(errCh)
	}()

	select {
	case err = <-errCh:
		if err != nil {
			str.logger.Error(fName + " returned error")
		} else {
			str.logger.Debug(fName + " successful")
		}
	case <-tCtx.Done():
		err = errors.New(tCtx.Err().Error())
	}
	return err
}

func (str *SynTestRoutine) panicHandler(funcName string) {
	if r := recover(); r != nil {
		str.logger.Debug("routine panicked", "func", funcName, "details", r)
	}
}

// Performs the test and returns a test result and error (if any), if the error is not nil then the testrun return will be a failed test run
func (str *SynTestRoutine) performTest(ctx context.Context, plugin common.SynTestPlugin, triggerInfo proto.Trigger) (proto.TestRun, error) {
	id := uuid.Generate().String()
	failedTestResult := common.FailedTestResult()
	res := proto.TestRun{
		TestConfig: &str.config,
		TestResult: &failedTestResult, // set a default failed result
		StartTime:  "",
		EndTime:    "",
		Id:         id,
		Trigger:    &triggerInfo,
	}
	s := time.Now()
	res.StartTime = s.Format(common.TimeFormat)

	type ReturnValues struct {
		TestResult proto.TestResult
		Err        error
	}

	returnCh := make(chan ReturnValues, 1)
	go func(returnCh chan ReturnValues) {
		testRes, err := plugin.PerformTest(triggerInfo)
		returnCh <- ReturnValues{
			TestResult: testRes,
			Err:        err,
		}
	}(returnCh)

	err := error(nil)
	select {
	case <-ctx.Done():
		err = ctx.Err()

	case returnValues := <-returnCh:
		res.TestResult = &returnValues.TestResult
		err = returnValues.Err
	}
	if err != nil {
		str.logger.Error("error performTest", err)
	}
	e := time.Now()
	res.EndTime = e.Format(common.TimeFormat)
	str.logger.Debug("performing test took", "time", e.Sub(s).String())
	return res, err
}

func (str *SynTestRoutine) testPlugin(ctx context.Context, initTimeout time.Duration, testTimeout time.Duration, finishTimeout time.Duration) error {
	// Connect with the Plugin
	pluginLogs := new(utils.Buffer)
	st, client, _, err := str.connectWithPlugin(str.config.PluginName, SynTestCmdMap[str.config.PluginName], pluginLogs)
	if err != nil {
		str.logger.Error("error connecting to plugin!", "err", err)
		return errors.Wrap(err, "error connecting to plugin")
	}
	defer client.Kill()

	// Initialise the plugin with timeout
	err = str.runFuncWithTimeout(ctx, initTimeout, "initialise", func(errCh chan error) {
		defer str.panicHandler("initialise")
		err := st.Initialise(str.config)
		errCh <- err
	})
	if err != nil {
		str.logger.Error("error initialising plugin", "err", err)
		return errors.Wrap(err, "error initialising plugin: --- LOGS ---\n"+pluginLogs.String())
	}
	if err != nil {
		str.logger.Error("error connecting to plugin!", "err", err)
		return errors.Wrap(err, "error connecting to plugin")
	}
	err = str.runTest(ctx, st, proto.Trigger{
		TriggerType: common.TriggerTypeTimer,
	}, testTimeout, pluginLogs)
	if err != nil {
		str.logger.Error("error run testing!", "err", err)
		return err
	}
	str.finish(ctx, st, finishTimeout)
	return nil
}

func (str *SynTestRoutine) connectWithPlugin(pluginName string, executableCmd []string, pluginLogs io.Writer) (common.SynTestPlugin, *plugin.Client, plugin.ClientProtocol, error) {
	str.logger.Info("connecting with plugin...")
	var command = exec.Command("")
	str.logger.Debug("command:", "cmd", executableCmd)

	if len(executableCmd) > 1 {
		command = exec.Command(executableCmd[0], executableCmd[1:]...)
	} else {
		command = exec.Command(executableCmd[0])
	}
	client := plugin.NewClient(&plugin.ClientConfig{
		HandshakeConfig:  common.DefaultTestPluginHandshakeConfig,
		Plugins:          SynTestNameMap,
		Cmd:              command,
		AllowedProtocols: []plugin.Protocol{plugin.ProtocolGRPC},
		Logger: hclog.New(&hclog.LoggerOptions{
			Level: hclog.Off,
		}),
		Stderr: pluginLogs,
	})

	// Connect via RPC
	rpcClient, err := client.Client()
	if err != nil {
		str.logger.Error("error call client.Client", "err", err)
		return nil, nil, nil, err
	}

	// Request the plugin
	raw, err := rpcClient.Dispense(pluginName)
	if err != nil {
		str.logger.Error("error Dispense client", "err", err)
		return nil, nil, nil, err
	}

	st := raw.(common.SynTestPlugin)
	return st, client, rpcClient, err
}
