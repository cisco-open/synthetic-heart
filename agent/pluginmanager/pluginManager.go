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
	"github.com/cisco-open/synthetic-heart/agent/utils"
	"github.com/cisco-open/synthetic-heart/common"
	"github.com/cisco-open/synthetic-heart/common/proto"
	"github.com/hashicorp/go-hclog"
	goPlugin "github.com/hashicorp/go-plugin"
	"gopkg.in/yaml.v3"

	"github.com/pkg/errors"
	"io/ioutil"
	"math"
	"math/rand"
	"os"
	"path/filepath"
	"sync"
	"syscall"
	"time"
)

// PluginManager manages all the different plugins
// It manages the lifecycle and the communication between them
type PluginManager struct {
	AgentId        string
	logger         hclog.Logger
	wg             sync.WaitGroup
	config         Config
	broadcaster    utils.Broadcaster
	sm             StateMap
	esh            ExtStorageHandler
	SyntheticTests map[string]SyntheticTest
}

type PrintPluginLogOption string

const (
	LogOnFail PrintPluginLogOption = "onFail"
	LogNever  PrintPluginLogOption = "never"
	LogAlways PrintPluginLogOption = "always"
)

type Config struct {
	NodeName         string               `yaml:"nodeName"`
	SyncFrequency    time.Duration        `yaml:"syncFrequency"`
	GracePeriod      time.Duration        `yaml:"gracePeriod"`
	PrometheusConfig PrometheusConfig     `yaml:"prometheus"`
	StoreConfig      StorageConfig        `yaml:"storage"`
	PrintPluginLogs  PrintPluginLogOption `yaml:"printPluginLogs"`
	Etc              map[string]string    `yaml:"etc"`
	DebugMode        bool                 `yaml:"debugMode"`
}

type RunnablePlugin interface {
	Run(ctx context.Context) error
}

type SyntheticTest struct {
	config  proto.SynTestConfig
	version string
	cancel  context.CancelFunc
	wg      *sync.WaitGroup
}

// Creates a new plugin manager
func NewPluginManager(configPath string) (*PluginManager, error) {
	pm := PluginManager{
		SyntheticTests: map[string]SyntheticTest{},
	}
	pm.logger = hclog.New(&hclog.LoggerOptions{
		Name:            "pm",
		Level:           hclog.LevelFromString(os.Getenv("LOG_LEVEL")),
		Color:           hclog.ForceColor,
		IncludeLocation: true,
	})

	err := pm.parsePluginManagerConfig(configPath)
	if err != nil {
		pm.logger.Error("error parsing config", "err", err)
		return nil, err
	}

	pm.sm = NewStateMap(pm.logger)
	pm.broadcaster = utils.NewBroadcaster(pm.logger)
	pm.AgentId = pm.config.NodeName
	pm.logger.Info("Agent Id: " + pm.AgentId)
	rand.Seed(time.Now().UTC().UnixNano()) // seed the rng

	esh, err := NewExtStorageHandler(pm.AgentId, pm.config.StoreConfig, pm.logger)
	if err != nil {
		return nil, errors.Wrap(err, "error creating storage client")
	}
	pm.esh = esh

	pm.logger.Info("pm config", "val", pm.config)

	return &pm, nil
}

func (pm *PluginManager) parsePluginManagerConfig(configPath string) error {
	conf := Config{}
	config, err := ioutil.ReadFile(configPath)
	if err != nil {
		pm.logger.Error("error reading config file", "file", configPath)
		return errors.Wrap(err, "error reading config file")
	}
	err = yaml.Unmarshal(config, &conf)
	if err != nil {
		pm.logger.Error("error unmarshalling config yaml", "file", configPath)
		return errors.Wrap(err, "error unmarshalling config yaml")
	}
	pm.config = conf

	// Get the node name
	pm.config.NodeName = os.Getenv("NODE_NAME") // Get node name from environmental variables
	if pm.config.NodeName == "" {
		return errors.New("NODE_NAME missing from env")
	}

	if pm.config.GracePeriod <= 0 {
		return errors.New("gracePeriod must be a positive value")
	}

	if pm.config.SyncFrequency <= 0 {
		return errors.New("syncFrequency must be a positive value")
	}

	// set default for print plugin log option
	if pm.config.PrintPluginLogs != LogOnFail && pm.config.PrintPluginLogs != LogAlways && pm.config.PrintPluginLogs != LogNever {
		pm.config.PrintPluginLogs = LogNever
	}

	pm.logger.Info("running with config:")
	pm.printConfig()
	return nil
}

func (pm *PluginManager) printConfig() {
	// Print the config
	bs, err := yaml.Marshal(pm.config)
	if err != nil {
		bs = []byte("error marshalling config as yaml: " + err.Error())
	}
	pm.logger.Info("\n" + string(bs))
}

func (pm *PluginManager) Start(ctx context.Context) error {
	// Cleanup plugins
	defer goPlugin.CleanupClients()

	// Create a wait group so we know which routines are running
	bwg := sync.WaitGroup{}          // wait group for broadcaster
	eshwg := sync.WaitGroup{}        // wait group for external storage helper
	prometheuswg := sync.WaitGroup{} // wait group for prometheus exporter

	// Run the Broadcaster
	bwg.Add(1)
	go func() {
		pm.broadcaster.Start()
		bwg.Done()
	}()

	pm.logger.Info("starting ext-storage helper")
	eshContext, cancelExtStore := context.WithCancel(ctx)
	eshwg.Add(1)
	go func(ctx context.Context) {
		defer eshwg.Done()
		err := pm.esh.Run(ctx, &pm.broadcaster, &pm.sm)
		if err != nil && !errors.Is(ctx.Err(), context.Canceled) && !errors.Is(ctx.Err(), context.DeadlineExceeded) {
			pm.logger.Error("error running ext-store", "err", err)
			pm.Exit(errors.Wrap(err, "cannot connect to storage"))
		}
	}(eshContext)

	// subscribe for new syntest configs
	pm.logger.Info("subscribing to config changes from ext-storage")
	configChan := make(chan string, 2)
	go func(ctx context.Context) {
		err := pm.esh.Store.SubscribeToConfigEvents(ctx, 1000, configChan)
		if err != nil && !errors.Is(ctx.Err(), context.Canceled) && !errors.Is(ctx.Err(), context.DeadlineExceeded) {
			pm.logger.Error("error watching for configuration change", "err", err)
			pm.Exit(errors.Wrap(err, "error watching for configuration change"))
		}
	}(ctx)

	// start the prometheus server
	promConfigChange := make(chan struct{}, 2)
	cancelPrometheus := pm.StartPrometheus(ctx, &prometheuswg, promConfigChange)

	ticker := time.NewTicker(pm.config.SyncFrequency)
	pm.logger.Trace("sending empty msg to force sync, timer also set", "frequency", pm.config.SyncFrequency)

	// send a signal to all agents and controller that a new agent is joining
	_ = pm.esh.Store.NewAgentEvent(ctx, "new agent: "+pm.AgentId)

	// send a config signal, to force sync at the start
	configChan <- "init"

configWatch:
	for {
		pm.logger.Info("listening for syntest configs...")
		select {
		case signal := <-configChan:
			pm.logger.Trace("sync triggered by redis signal", "signal", signal)

			// sleep a random time to prevent storms
			time.Sleep(time.Duration(rand.Intn(common.MaxConfigTimerJitter)) * time.Millisecond)
			configChanged, err := pm.SyncConfig(ctx)
			if err != nil {
				pm.logger.Error("cannot sync configs, no point continuing")
				pm.Exit(errors.Wrap(err, "error syncing config"))
			}
			if configChanged {
				promConfigChange <- struct{}{} // notify prometheus that config has changed
			}
		case <-ticker.C:
			pm.logger.Trace("sync triggered by timer")
			pm.logger.Debug("checking redis connection")
			err := pm.esh.Store.Ping(ctx)
			if err != nil {
				pm.logger.Error("cannot ping storage sucessfully")
				pm.Exit(errors.Wrap(err, "error syncing config"))
			}

			pm.logger.Debug("syncing configs")
			configChanged, err := pm.SyncConfig(ctx)
			if err != nil {
				pm.logger.Error("cannot sync configs, no point continuing")
				pm.Exit(errors.Wrap(err, "error syncing config"))
			}
			if configChanged {
				promConfigChange <- struct{}{} // notify prometheus that config has changed
			}
		case <-ctx.Done():
			break configWatch
		}
	}

	// Wait for syntests to finish
	for k, _ := range pm.SyntheticTests {
		pm.logger.Info("waiting for plugin to finish", "plugin", pm.SyntheticTests[k].config.Name)
		pm.SyntheticTests[k].wg.Wait()
	}
	pm.logger.Info("all syntest routines finished...")

	pm.logger.Warn("allowing time for agent to export all test results...", "gracePeriod", pm.config.GracePeriod)
	time.Sleep(pm.config.GracePeriod)

	// Wait for prometheus to finish
	cancelPrometheus()
	pm.logger.Info("waiting for prometheus to finish...")
	prometheuswg.Wait()

	pm.logger.Info("cleaning up external storage")
	pm.cleanupAndUnregister()

	cancelExtStore()
	pm.logger.Info("waiting for ext-storage-manger to finish...")
	eshwg.Wait()

	pm.broadcaster.Stop()
	pm.logger.Info("waiting for broadcaster finish...")
	bwg.Wait()
	return nil
}

// Starts prometheus server, returns a cancel function
func (pm *PluginManager) StartPrometheus(ctx context.Context, wg *sync.WaitGroup, configChange chan struct{}) context.CancelFunc {
	prometheusContext, cancelPrometheus := context.WithCancel(ctx)
	wg.Add(1)
	go func(ctx context.Context) {
		if pm.config.PrometheusConfig.ServerAddress != "" {
			prom := NewPrometheusExporter(pm.logger.Named("prometheus"), pm.config.PrometheusConfig, pm.AgentId, pm.config.DebugMode)
			prom.Run(ctx, &pm.broadcaster, configChange)
		}
		wg.Done()
	}(prometheusContext)
	return cancelPrometheus
}

func (pm *PluginManager) Exit(err error) {
	pm.logger.Error("FATAL Error", "err", err.Error())
	syscall.Kill(syscall.Getpid(), syscall.SIGINT)
}

func (pm *PluginManager) cleanupAndUnregister() {
	// Cleanup all synthetic test plugin data
	for k, _ := range pm.SyntheticTests {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		pm.StopAndDeleteSynTest(ctx, k)
		cancel()
	}

	// Cleanup all agent info
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	err := pm.esh.Store.DeleteAgentStatus(ctx, pm.AgentId)
	if err != nil {
		pm.logger.Warn("error deleting agent info external storage", "err", err)
	}

	// send a event to everyone that this agent is quitting
	err = pm.esh.Store.NewAgentEvent(ctx, "exiting agent: "+pm.AgentId)
	if err != nil {
		pm.logger.Warn("error deleting agent info external storage", "err", err)
	}
	cancel()
}

func (pm *PluginManager) SyncConfig(ctx context.Context) (bool, error) {
	pm.logger.Info("syncing syntest configs...")
	configChanged, err := pm.SyncSyntestPluginConfigs(ctx)
	if err != nil {
		return configChanged, errors.Wrap(err, "error syncing syntest configs")
	}
	pm.logger.Info("finished syncing syntest configs")
	return configChanged, nil
}

// checks external storage for new syntest config or change in existing ones and then start/stops appropriate plugins
func (pm *PluginManager) SyncSyntestPluginConfigs(ctx context.Context) (bool, error) {
	configChanged := false
	latestSynTestConfigs, err := pm.esh.Store.FetchAllTestConfig(ctx)
	if err != nil {
		return configChanged, err
	}
	// iterate over the running syntests, and check if they still exist
	for name, _ := range pm.SyntheticTests {
		_, ok := latestSynTestConfigs[name]
		if !ok {
			pm.logger.Info("syntest deleted", "test", name)
			pm.StopAndDeleteSynTest(ctx, name)
			configChanged = true
		}
	}

	// iterate over latest syntest configs, and check if the version we are running is the latest
	for testName, latestVersion := range latestSynTestConfigs {
		st, ok := pm.SyntheticTests[testName]
		// if test exists and we are running on latest version, then continue to next test
		if ok && st.version == latestVersion {
			continue
		}
		synTestConfig, err := pm.esh.Store.FetchTestConfig(ctx, testName)
		if err != nil {
			pm.logger.Warn("error getting latest config", "name", testName, "err", err)
			continue
		}
		if ok { // test is running but version changed - so we stop and delete it for now
			pm.logger.Info("syntest changed", "test", testName, "old", st.version, "new", latestVersion)
			pm.StopAndDeleteSynTest(ctx, testName)
			configChanged = true
		}

		pm.logger.Trace("checking if test matches agent selector", "test", testName)
		// check if it matches the agentSelector, otherwise dont run
		if ok, err := filepath.Match(synTestConfig.AgentSelector, pm.AgentId); err == nil && ok {
			tCtx, cancel := context.WithCancel(ctx)
			pm.SyntheticTests[testName] = SyntheticTest{
				config:  synTestConfig,
				version: latestVersion,
				cancel:  cancel,
				wg:      &sync.WaitGroup{},
			}
			pm.logger.Info("(re)starting syntest", "test", testName)
			pm.StartTestRoutine(tCtx, pm.SyntheticTests[testName])
			configChanged = true
		} else {
			pm.logger.Debug("not running test as it didn't match agent selector",
				"name", testName, "selector", synTestConfig.AgentSelector)
		}

	}
	return configChanged, nil
}

func (pm *PluginManager) StopAndDeleteSynTest(ctx context.Context, testName string) {
	pm.logger.Debug("stopping and deleting", "test", testName)
	pm.SyntheticTests[testName].cancel()
	(pm.SyntheticTests[testName].wg).Wait() // wait until the test stops
	// delete old data
	delete(pm.SyntheticTests, testName)
	pluginId := common.ComputePluginId(pm.AgentId, testName)
	pm.sm.DeletePluginState(pluginId)
	err := pm.esh.Store.DeleteAllTestRunInfo(ctx, pluginId)
	if err != nil {
		pm.logger.Warn("error deleting syntest data from ext-storage", "name", testName, "err", err)
	}
}

// Starts the synthetic test go routine (that manages the plugin)
func (pm *PluginManager) StartTestRoutine(ctx context.Context, s SyntheticTest) {
	pm.logger.Debug("starting test routine", "name", s.config.Name, "plugin", s.config.PluginName)

	// Add run time configs
	s.config.Etc = map[string]string{}
	for k, v := range pm.config.Etc {
		s.config.Etc[k] = v
	}
	s.config.Etc["nodeName"] = pm.config.NodeName
	s.config.Etc["agentId"] = pm.AgentId

	// Create an empty struct for plugin state
	synTestState := common.PluginState{}
	synTestState.Status = common.StatusUnknown
	synTestState.Config = s.config
	synTestState.Restarts = -1
	synTestState.TotalRestarts = -1

	pluginId := common.ComputePluginId(pm.AgentId, s.config.Name)

	if testPlugin, ok := SynTestNameMap[s.config.PluginName]; ok {
		// Create the test routine
		t := SynTestRoutine{
			agentId:         pm.AgentId,
			config:          s.config,
			plugin:          testPlugin,
			broadcaster:     &pm.broadcaster,
			storageHandler:  &pm.esh,
			printPluginLogs: pm.config.PrintPluginLogs,
		}

		// Add the go routine to the wait group
		s.wg.Add(1)

		// Set the state for the plugin
		pm.sm.SetPluginState(pluginId, synTestState)

		// Parse restart policy, or set default
		restartPolicy := common.PluginRestartPolicy(s.config.PluginRestartPolicy)
		if restartPolicy != common.RestartAlways && restartPolicy != common.RestartNever && restartPolicy != common.RestartOnError {
			pm.logger.Warn("restartPolicy not supported, using default", "restartPolicy", s.config.PluginRestartPolicy, "default", common.DefaultRestartPolicy)
			restartPolicy = common.DefaultRestartPolicy
		}

		// Start the go routine with the params
		go func(ctx context.Context, id string, pluginName string, restartPolicy common.PluginRestartPolicy, routine SynTestRoutine, sm StateMap) {
			defer s.wg.Done()
			StartPlugin(ctx, id, pluginName, &routine, restartPolicy, sm)
		}(ctx, pluginId, t.config.PluginName, restartPolicy, t, pm.sm)
	} else {
		// Set error state for the plugin
		synTestState.Status = common.Error
		synTestState.StatusMsg = "couldn't find plugin '" + s.config.PluginName + "' in the name map"
		pm.sm.SetPluginState(pluginId, synTestState)
		pm.logger.Error("couldn't find syntest plugin in the name map", "plugin", s.config.PluginName, "name", s.config.Name)
	}
}

// Starts a plugin and manages the lifecycle (i.e. syntest)
func StartPlugin(ctx context.Context, pluginId string, pluginName string, plugin RunnablePlugin, restartPolicy common.PluginRestartPolicy, sm StateMap) {
	logger := hclog.New(&hclog.LoggerOptions{
		Name:            "pm.pluginStarter",
		Level:           hclog.LevelFromString(os.Getenv("LOG_LEVEL")),
		Color:           hclog.ForceColor,
		IncludeLocation: true,
	})

	if restartPolicy == "" { // set default for restartPolicy
		restartPolicy = common.RestartAlways
	}

	for ctx.Err() == nil { // For loop for restart, checks if context was cancelled
		// Fetch the state of the plugin
		s, smErr := sm.GetPluginState(pluginId)
		if smErr != nil {
			logger.Error("cannot fetch plugin state!", "pluginName", pluginName, "pluginId", pluginId, "err", smErr)
			break
		}

		// Set to running state
		s.Status = common.Running
		s.Restarts++
		s.TotalRestarts++
		s.LastMsg = s.StatusMsg
		s.StatusMsg = ""
		s.RunningSince = time.Now()
		sm.SetPluginState(pluginId, s)

		routineCtx, cancel := context.WithCancel(ctx)

		err := plugin.Run(routineCtx) // Runs the Plugin - blocking call
		logger.Warn("routine returned", "pluginName", pluginName, "pluginId", pluginId, "err", err)
		cancel() // stop any routines started by the Run command

		if err != nil { // Check if it returned an error
			logger.Error("plugin run returned error: ", "plugin", pluginName, "err", err)
			s.LastMsg = s.StatusMsg
			s.StatusMsg = err.Error()
			if restartPolicy == common.RestartNever {
				s.Status = common.Error
				sm.SetPluginState(pluginId, s)
				break // dont restart
			} else {
				s.Status = common.RestartBackOff
				sm.SetPluginState(pluginId, s)
			}
		} else { // Plugin exited with no error
			s.LastMsg = s.StatusMsg
			s.StatusMsg = "plugin exited with no error"
			if restartPolicy == common.RestartNever || restartPolicy == common.RestartOnError {
				s.Status = common.NotRunning
				sm.SetPluginState(pluginId, s)
				break // dont restart
			} else {
				s.Status = common.RestartBackOff
				sm.SetPluginState(pluginId, s)
			}
		}

		// if the code got to here, that means the plugin needs to be restarted
		// If the plugin succesfully ran for over 10 minutes, then reset the number of restarts
		if time.Now().Sub(s.RunningSince) > 10*time.Minute {
			s.Restarts = 0
			sm.SetPluginState(pluginId, s)
		}

		// Calculate the next backOff time
		backOffTime := time.Duration(10*math.Pow(2, math.Max(float64(s.Restarts), 0))) * time.Second
		if backOffTime > 5*time.Minute { // Max backoff time is 5 minutes
			backOffTime = 5 * time.Minute
		}

		// Making sure that the backoff time is a positive number, otherwise it can result in panics
		if backOffTime <= 0 {
			backOffTime = 1 * time.Second
		}

		// Wait before retrying
		ticker := time.NewTicker(backOffTime)
		logger.Info("waiting before restart", "dur", backOffTime.String())
		select {
		case <-ctx.Done():
			logger.Info("context cancelled, exiting...")
			break
		case <-ticker.C:
			break
		}
		ticker.Stop()
	}
}
