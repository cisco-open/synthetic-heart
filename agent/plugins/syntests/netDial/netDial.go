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
	"log"
	"net"
	"strings"
	"time"

	"github.com/cisco-open/synthetic-heart/common"
	"github.com/cisco-open/synthetic-heart/common/proto"
	"github.com/cisco-open/synthetic-heart/common/utils"
	"github.com/hashicorp/go-plugin"
	"github.com/pkg/errors"
)

type NetDialTest struct {
	config  NetDialTestConfig
	timeout time.Duration
}

type NetDialTestConfig struct {
	Addresses []Address
	Workers   int
}

type Address struct {
	Network string `yaml:"net"`
	Address string `yaml:"addr"`
	Timeout int    `yaml:"timeout"`
}

func (t *NetDialTest) Initialise(synTestConfig proto.SynTestConfig) error {
	netDialConfig := NetDialTestConfig{}
	err := common.ParseYMLConfig(synTestConfig.Config, &netDialConfig)
	if err != nil {
		return errors.Wrap(err, "error parsing config")
	}
	t.config = netDialConfig
	// Set default workers to 3
	if t.config.Workers <= 0 {
		t.config.Workers = 3
	}
	return nil
}

func (t *NetDialTest) PerformTest(_ proto.Trigger) (proto.TestResult, error) {
	// Empty test result struct for you to populate - currently set to a fail test result
	testResult := proto.TestResult{Marks: 0, MaxMarks: 1, Details: map[string]string{}}
	if len(t.config.Addresses) <= 0 {
		testResult.Marks = 1
		log.Println("nothing to test")
		return testResult, nil
	}
	testResult.MaxMarks = uint64(len(t.config.Addresses))
	promMetrics := common.PrometheusMetrics{Gauges: []common.PrometheusGauge{}}

	// Create a worker pool to do the connection tests in parallel
	wp := utils.NewWorkerPool(t.config.Workers, len(t.config.Addresses), netDialTest, false)
	wp.Start(context.Background())
	defer wp.Stop()

	// Add the addresses as jobs for workers to work on
	for _, addr := range t.config.Addresses {
		wp.AddJob(addr)
	}

	// Collect the result and logs from the tests one-by-one
	for i := 0; i < len(t.config.Addresses); i++ {
		// Wait until the test is done and result is ready
		res := <-wp.ResultChan
		ok := 0
		if res.Error == nil {
			testResult.Marks++
			ok = 1
		}

		// Print the logs
		log.Println("---\n" + strings.TrimSuffix(res.Logs, "\n"))
		log.Printf("total marks: %d/%d \n", testResult.Marks, testResult.MaxMarks)

		// Add prometheus metric for test successes
		promMetrics.Gauges = append(promMetrics.Gauges,
			common.PrometheusGauge{
				Name:  "net_dial",
				Help:  "Whether Net dial was successful",
				Value: float64(ok),
				Labels: map[string]string{
					"net":  res.Job.(Address).Network,
					"addr": res.Job.(Address).Address,
				},
			})

		// Add prometheus metric for connection times
		d := res.ReturnValues.(time.Duration)
		promMetrics.Gauges = append(promMetrics.Gauges,
			common.PrometheusGauge{
				Name:  "net_dial_duration_ns",
				Help:  "Duration of the net.Dial() call",
				Value: float64(d.Nanoseconds()),
				Labels: map[string]string{
					"net":  res.Job.(Address).Network,
					"addr": res.Job.(Address).Address,
				},
			})
	}

	err := common.AddPrometheusMetricsToResults(promMetrics, testResult)
	if err != nil {
		log.Println("unable to add prometheus metrics")
		return testResult, err
	}

	return testResult, nil
}

func netDialTest(_ context.Context, log *log.Logger, j interface{}) (interface{}, error) {
	job := j.(Address)
	// Set a default timeout
	if job.Timeout == 0 {
		job.Timeout = 3
	}
	log.Println(job)
	log.Println("dialing on " + job.Address + "...")
	start := time.Now()
	conn, err := net.DialTimeout(job.Network, job.Address, time.Duration(job.Timeout)*time.Second)
	if err != nil {
		log.Println("could not connect", err)
		return time.Duration(0), errors.Wrap(err, "could not connect to "+job.Address)
	}
	duration := time.Since(start)
	log.Printf("net.Dial() took %dms\n", duration.Milliseconds())

	defer conn.Close()
	log.Println("port is open")
	return duration, nil
}

func (t *NetDialTest) Finish() error {
	return nil
}

func main() {
	pluginImpl := &NetDialTest{}

	plugin.Serve(&plugin.ServeConfig{
		HandshakeConfig: common.DefaultTestPluginHandshakeConfig,
		Plugins: map[string]plugin.Plugin{
			common.NetDialTestName: &common.SynTestGRPCPlugin{Impl: pluginImpl},
		},
		GRPCServer: plugin.DefaultGRPCServer,
	})
}
