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

package common

import (
	"crypto/md5"
	"fmt"
	"github.com/cisco-open/synthetic-heart/common/proto"
	"github.com/pkg/errors"
	"gopkg.in/yaml.v2"
	"log"
	"net"
	"os"
	"strings"
)

const ActiveClusterStatus = 1

// Parses a yaml config
func ParseYMLConfig(configStr string, o interface{}) error {
	err := yaml.Unmarshal([]byte(configStr), o)
	log.Println(fmt.Sprintf("config: %v", o))
	return err
}

// Gets a failed test result
func FailedTestResult() proto.TestResult {
	return proto.TestResult{
		Marks:    0,
		MaxMarks: 1,
		Details:  map[string]string{},
	}
}

// Computes the tag for heartbeat send to LMA (used by LMA tests)
func GetKafkaHeartBeatTag(clusterName string, clusterDomain string) string {
	clusterDomainHash := fmt.Sprintf("%08x", md5.Sum([]byte(clusterName+clusterDomain)))
	return fmt.Sprintf("synthetic_heart_kafka_hb_%s_%s", clusterName, clusterDomainHash[:8])
}

// Adds a prometheus metric to a test result
func AddPrometheusMetricsToResults(promMetric PrometheusMetrics, testResult proto.TestResult) error {
	if testResult.Details == nil {
		testResult.Details = map[string]string{}
	}
	// Add prometheus metrics to the test results - this will be picked up the prometheus result handler
	promYaml, err := yaml.Marshal(promMetric)
	if err != nil {
		return errors.Wrap(err, "error converting prometheus metric to yaml")
	} else {
		testResult.Details[PrometheusKey] = string(promYaml)
	}
	return nil
}

// Computes test run status from the test run results
func GetTestRunStatus(testRun proto.TestRun) TestRunStatus {
	testResultStatus := Unknown
	if testRun.Id != "" {
		passRatio := float64(testRun.TestResult.Marks) / float64(testRun.TestResult.MaxMarks)
		if passRatio == 1 {
			testResultStatus = Passing
		} else if passRatio >= 0.5 && testRun.TestConfig.Importance != ImportanceCritical {
			testResultStatus = Warning
		} else {
			testResultStatus = Failing
		}
	}
	return testResultStatus
}

// Computes plugin id: Plugin Id is just a string representing the agentId (i.e. node) & the test name
func ComputePluginId(agentId string, testName string) string {
	return agentId + "/" + testName
}

// Splits the plugin Id into its agent Id and test name
func GetPluginIdComponents(pluginId string) (agentId string, testName string, err error) {
	comp := strings.Split(pluginId, "/")
	if len(comp) < 2 {
		return "", "", errors.New("invalid id, not enough components")
	}
	return comp[0], comp[1], nil
}

// Get all active cluster names in target cloud by query cluster status DNS SRV record
func GetAllActiveClusters(domain string) ([]string, error) {
	var allClusters []string
	_, srvRecords, err := net.LookupSRV("", "", domain)
	if err != nil {
		log.Printf("DNS SRV look up %s error, %s", domain, err)
		return nil, err
	}
	for _, srvRecord := range srvRecords {
		if srvRecord.Weight == ActiveClusterStatus {
			allClusters = append(allClusters, strings.Split(srvRecord.Target, ".")[1])
		}
	}
	log.Printf("DNS SRV look up all active clusters: %v", allClusters)
	return allClusters, nil
}

// Simple helper struct to output logs in the test result details
type TestDetailsLogger struct {
	Res       proto.TestResult
	stdLogger *log.Logger
}

func (l *TestDetailsLogger) Log(msg string) {
	l.Res.Details[LogKey] += msg + "\n"
	if l.stdLogger == nil {
		l.stdLogger = log.New(os.Stderr, "", log.LstdFlags)
	}
	l.stdLogger.Println(msg)
}
