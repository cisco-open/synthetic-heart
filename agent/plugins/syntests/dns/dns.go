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
	"errors"
	"fmt"
	"github.com/cisco-open/synthetic-heart/common"
	"github.com/cisco-open/synthetic-heart/common/proto"
	"github.com/cisco-open/synthetic-heart/common/utils"
	"github.com/hashicorp/go-plugin"
	"log"
	"net"
	"strings"
)

/*
 * Test to check if domains are resolvable
 */
type DNSTest struct {
	config DNSTestConfig
}

type DNSTestConfig struct {
	Domains []string `yaml:"domains"`
	Workers int      `yaml:"workers"`
	Repeats int      `yaml:"repeats"`
}

func (t *DNSTest) Initialise(synTestConfig proto.SynTestConfig) error {
	t.config = DNSTestConfig{}
	log.Println("parsing config")
	err := common.ParseYMLConfig(synTestConfig.Config, &t.config)
	// Set default workers to 3
	if t.config.Workers <= 0 {
		t.config.Workers = 3
	}

	// Set default repeats to 1
	if t.config.Repeats <= 0 {
		t.config.Repeats = 1
	}
	return err
}

func (t *DNSTest) PerformTest(_ proto.Trigger) (proto.TestResult, error) {
	log.Println("performing dns tests...")
	if len(t.config.Domains) <= 0 {
		return common.FailedTestResult(), errors.New("no domains to test")
	}

	// Create an empty test result struct
	testResult := proto.TestResult{
		Marks:    0,
		MaxMarks: uint64(len(t.config.Domains) * t.config.Repeats),
		Details:  map[string]string{},
	}
	promMetrics := common.PrometheusMetrics{Gauges: []common.PrometheusGauge{}}

	// Create a worker pool to do the dns tests in parallel
	wp := utils.NewWorkerPool(t.config.Workers, len(t.config.Domains), dnsTest, false)
	wp.Start(context.Background())
	defer wp.Stop()
	testResult.Marks = 0

	// Add the domains as jobs
	for _, domain := range t.config.Domains {
		wp.AddJob(DnsTestJob{domain, t.config.Repeats})
	}

	// Collect the results and logs from the dns tests one-by-one
	for i := 0; i < len(t.config.Domains); i++ {
		// Wait until the test is done and result is ready
		res := <-wp.ResultChan

		successCount := res.ReturnValues.(int)
		// If no errors when doing the test, increment marks
		if res.Error == nil {
			testResult.Marks += uint64(successCount)
		}

		// Print the logs
		log.Println("---\n" + strings.TrimSuffix(res.Logs, "\n"))
		log.Printf("total marks: %d/%d \n", testResult.Marks, testResult.MaxMarks)

		// Add prometheus metric
		promMetrics.Gauges = append(promMetrics.Gauges,
			createPrometheusGauge(res.Job.(DnsTestJob).Domain, successCount, t.config.Repeats))
	}

	err := common.AddPrometheusMetricsToResults(promMetrics, testResult)
	if err != nil {
		log.Println("unable to add prometheus metrics")
		return testResult, err
	}
	return testResult, nil
}

func createPrometheusGauge(domain string, successCount int, maxRetries int) common.PrometheusGauge {
	return common.PrometheusGauge{
		Name:   "dns_repeat_count",
		Help:   fmt.Sprintf("Number of Successful resolutions (out of %d)", maxRetries),
		Value:  float64(successCount),
		Labels: map[string]string{"domain": domain},
	}
}

type DnsTestJob struct {
	Domain  string
	Repeats int
}

func dnsTest(_ context.Context, log *log.Logger, d interface{}) (interface{}, error) {
	domain := d.(DnsTestJob).Domain
	repeats := d.(DnsTestJob).Repeats
	log.Println("sending dns request to " + domain)
	ips := []net.IP{}
	err := error(nil)
	marks := 0
	for i := 0; i < repeats; i++ {
		ips, err = net.LookupIP(domain)
		if err != nil {
			log.Printf("[%d/%d] err: %s", i+1, repeats, err.Error())
		} else {
			marks++
			log.Printf("[%d/%d] got IPs: %v", i+1, repeats, ips)
		}

	}
	return marks, nil
}

func (t *DNSTest) Finish() error { return nil }

func main() {
	pluginImpl := &DNSTest{}

	plugin.Serve(&plugin.ServeConfig{
		HandshakeConfig: common.DefaultTestPluginHandshakeConfig,
		Plugins: map[string]plugin.Plugin{
			common.DnsTestName: &common.SynTestGRPCPlugin{Impl: pluginImpl},
		},
		GRPCServer: plugin.DefaultGRPCServer,
	})
}
