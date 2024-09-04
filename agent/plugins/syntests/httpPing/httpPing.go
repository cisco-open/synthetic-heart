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

package main

import (
	"context"
	"crypto/tls"
	"fmt"
	"io/ioutil"
	"log"
	"math"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/cisco-open/synthetic-heart/common"
	"github.com/cisco-open/synthetic-heart/common/proto"
	"github.com/cisco-open/synthetic-heart/common/utils"
	"github.com/hashicorp/go-plugin"
	"github.com/pkg/errors"
)

const PluginName = "httpPing"

type HttpPingTest struct {
	configs []HttpPingTestConfig
	timeout time.Duration
}

const ParallelWorkers = 5
const DefaultWaitBetweenRepeats = "5s"

type HttpPingTestConfig struct {
	Address           string `yaml:"address"`
	ExpectedCodeRegex string `yaml:"expectedCodeRegex"`
	MaxRetries        int    `yaml:"retries"`
	MaxTimeoutRetry   int    `yaml:"timeoutRetries"`
	RepeatWithoutFail int    `yaml:"repeatsWithoutFail"`
	WaitBetweenRepeat string `yaml:"waitBetweenRepeats"`
	timeout           time.Duration
}

func (t *HttpPingTest) Initialise(synTestConfig proto.SynTestConfig) error {
	configs := []HttpPingTestConfig{}
	err := common.ParseYMLConfig(synTestConfig.Config, &configs)

	if err != nil || len(configs) == 0 { // try parsing it as a single config instead of an array
		c := HttpPingTestConfig{}
		err = common.ParseYMLConfig(synTestConfig.Config, &c)
		if err != nil {
			return err
		}
		configs = append(configs, c)
	}

	testTimeout, err := time.ParseDuration(synTestConfig.Timeouts.Run)
	if err != nil {
		return err
	}
	// timeout for each url - based on number of urls, no. of workers
	t.timeout = (testTimeout / time.Duration(math.Max(float64(len(configs)/ParallelWorkers), 1))) - time.Second
	t.configs = configs
	for i := range t.configs {
		if (t.configs[i].MaxRetries > 0 || t.configs[i].MaxTimeoutRetry > 0) && t.configs[i].RepeatWithoutFail > 0 {
			log.Println("Error: retries/timeoutRetries and repeatsWithoutFail are mutually exclusive and cannot be used together.")
			return errors.New("retries/timeoutRetries and repeatsWithoutFail cannot be larger than 0 at the same time")
		}
		if len(t.configs[i].WaitBetweenRepeat) == 0 {
			t.configs[i].WaitBetweenRepeat = DefaultWaitBetweenRepeats
		}
	}
	return nil
}

func (t *HttpPingTest) PerformTest(_ proto.Trigger) (proto.TestResult, error) {
	log.Println("performing http ping tests...")
	log.Printf("timeout: %s ", t.timeout)
	if len(t.configs) <= 0 {
		return common.FailedTestResult(), errors.New("no endpoints to test")
	}

	// Create an empty test result struct
	testResult := proto.TestResult{
		Marks:    0,
		MaxMarks: uint64(len(t.configs)),
		Details:  map[string]string{},
	}

	// Create a worker pool to do the http ping tests in parallel
	wp := utils.NewWorkerPool(ParallelWorkers, len(t.configs), httpPingTest, false)
	wp.Start(context.Background())
	defer wp.Stop()
	testResult.Marks = 0

	// Add the urls as jobs
	for _, pingTest := range t.configs {
		pingTest.timeout = t.timeout
		wp.AddJob(pingTest)
	}

	promMetrics := common.PrometheusMetrics{Gauges: []common.PrometheusGauge{}}
	// Collect the results and logs from the http ping tests one-by-one
	for i := 0; i < len(t.configs); i++ {
		// Wait until the test is done and result is ready
		res := <-wp.ResultChan
		httpPingTestRes := res.ReturnValues.((map[string]int))
		additionalMarks := httpPingTestRes["marks"]
		elapsed_time := httpPingTestRes["elapsed_time"]
		promMetrics.Gauges = append(promMetrics.Gauges, createPrometheusGauge(elapsed_time))
		// If no errors when doing the test, increment marks
		if res.Error == nil {
			testResult.Marks += uint64(additionalMarks)
		}

		// Print the logs
		log.Println("\n----\n\n----\n" + strings.TrimSuffix(res.Logs, "\n"))
		log.Printf("marks: %d/%d (+%d)\n", testResult.Marks, testResult.MaxMarks, additionalMarks)
	}
	err := common.AddPrometheusMetricsToResults(promMetrics, testResult)
	if err != nil {
		log.Println("unable to add prometheus metrics")
		return testResult, err
	}
	return testResult, nil
}

func httpPingTest(_ context.Context, log *log.Logger, d interface{}) (interface{}, error) {
	result := map[string]int{"marks": 0, "elapsed_time": 0}
	address := d.(HttpPingTestConfig).Address
	expectedCodeRegex := d.(HttpPingTestConfig).ExpectedCodeRegex
	maxRetries := d.(HttpPingTestConfig).MaxRetries
	timeout := d.(HttpPingTestConfig).timeout
	maxTimeoutRetries := d.(HttpPingTestConfig).MaxTimeoutRetry
	repeatsWithoutFail := d.(HttpPingTestConfig).RepeatWithoutFail
	waitBetweenRepeats := d.(HttpPingTestConfig).WaitBetweenRepeat
	repeatWaitTime, parseErr := time.ParseDuration(waitBetweenRepeats)
	if parseErr != nil {
		return result, errors.Wrap(parseErr, "error parsing repeated success test interval")
	}

	log.Println("address: " + address)

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	tr := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}

	c := http.DefaultClient
	c.Transport = tr

	req, err := http.NewRequest("GET", address, nil)
	if err != nil {
		return result, err
	}
	req = req.WithContext(ctx)

	err = error(nil)
	var elapsed_time time.Duration
	match := false
	if repeatsWithoutFail > 0 {
		// when repeatsWithoutFail is larger than 0, zero-tolerance for ping failure
		for i := 1; i <= repeatsWithoutFail; i++ {
			log.Println(fmt.Sprintf("(%d/%d) repeat success testing...", i, repeatsWithoutFail))
			match, elapsed_time, err = runHttpPing(c, req, log, expectedCodeRegex)
			result["elapsed_time"] = int(elapsed_time.Milliseconds())
			if ctx.Err() != nil {
				log.Println(fmt.Sprintf("(%d/%d) error in http request context when repeat success ping, %v", i, repeatsWithoutFail, ctx.Err()))
				return result, ctx.Err()
			}
			if err != nil {
				log.Println(fmt.Sprintf("(%d/%d) failed http request when repeat success ping, %v", i, repeatsWithoutFail, err))
				return result, err
			}
			if !match {
				log.Println(fmt.Sprintf("(%d/%d) failed to match expected code when repeat success ping", i, repeatsWithoutFail))
				return result, nil
			}
			if i < repeatsWithoutFail {
				log.Println(fmt.Sprintf("sleeping %s before next repeatable success test", waitBetweenRepeats))
				time.Sleep(repeatWaitTime)
			}
		}
		log.Println(fmt.Sprintf("whole repeatable success test successfully after ping %d times", repeatsWithoutFail))
		result["marks"] = 1
	} else {
		for i := 0; i <= maxRetries && (ctx.Err() == nil || ctx.Err().Error() == context.DeadlineExceeded.Error()); i++ {
			// Allow retry when context deadline exceeded. Retry times not larger than maxRetries
			// maxRetries is max retry times when http request failed EXCEPT exceeded timeout
			// maxTimeoutRetries is max retry times ONLY when http request exceeded timeout
			// maxTimeoutRetries default value is 0. Recommended value is small number, like 1
			// Retry for timeout cases decline performance of synthetic test. Use maxTimeoutRetries for compromise
			if i > maxTimeoutRetries && ctx.Err() != nil && ctx.Err().Error() == context.DeadlineExceeded.Error() {
				break
			}
			if i > 0 {
				log.Println(fmt.Sprintf("(%d/%d) retrying...", i, maxRetries))
			}
			match, elapsed_time, err = runHttpPing(c, req, log, expectedCodeRegex)
			result["elapsed_time"] = int(elapsed_time.Milliseconds())
			if err == nil {
				break
			}
		}
		if err != nil {
			return result, err
		}
		if match {
			result["marks"] = 1
		}
	}
	return result, nil
}

func runHttpPing(c *http.Client, req *http.Request, log *log.Logger, expectedCodeRegex string) (bool, time.Duration, error) {
	start := time.Now()
	resp, err := c.Do(req)
	defer func() {
		if resp != nil && resp.Body != nil {
			log.Println("closing response body")
			resp.Body.Close()
		}
	}()
	if err == nil {
		elapsedTime := time.Since(start)
		log.Println("request latency: " + elapsedTime.String())
		log.Println("request returned code " + strconv.Itoa(resp.StatusCode))
		log.Println("details:")
		bodyBytes, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			return false, 0, err
		}
		bodyString := string(bodyBytes)
		log.Println(bodyString)
		match, err := regexp.MatchString(expectedCodeRegex, strconv.Itoa(resp.StatusCode))
		if err != nil {
			return false, elapsedTime, err
		}

		if match {
			log.Println("ping successful")
			return true, elapsedTime, nil
		} else {
			log.Println("ping failed")
			return false, elapsedTime, nil
		}
	}
	log.Println("err:", err)
	return false, 0, err
}

func (t *HttpPingTest) Finish() error {
	return nil
}

func createPrometheusGauge(latency int) common.PrometheusGauge {
	return common.PrometheusGauge{
		Name:  "http_ping_latency",
		Help:  "show httpPing request latency",
		Value: float64(latency),
		// Labels: map[string]string{"domain": domain},
	}
}

func main() {
	pluginImpl := &HttpPingTest{}

	plugin.Serve(&plugin.ServeConfig{
		HandshakeConfig: common.DefaultTestPluginHandshakeConfig,
		Plugins: map[string]plugin.Plugin{
			PluginName: &common.SynTestGRPCPlugin{Impl: pluginImpl},
		},
		GRPCServer: plugin.DefaultGRPCServer,
	})
}
