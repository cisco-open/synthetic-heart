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
	"fmt"
	"github.com/cisco-open/synthetic-heart/common/proto"
	"github.com/hashicorp/go-hclog"
	"github.com/pkg/errors"
	"gopkg.in/yaml.v2"
	"log"
	"os"
	"path/filepath"
	"strings"
)

const ActiveClusterStatus = 1

// ParseYMLConfig Parses and prints a yaml config
func ParseYMLConfig(configStr string, o interface{}) error {
	err := yaml.Unmarshal([]byte(configStr), o)
	log.Println(fmt.Sprintf("config: %v", o))
	return err
}

// FailedTestResult Gets an empty failed test result
func FailedTestResult() proto.TestResult {
	return proto.TestResult{
		Marks:    0,
		MaxMarks: 1,
		Details:  map[string]string{},
	}
}

// AddPrometheusMetricsToResults Adds a prometheus metric to a test result
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

// ComputePluginId Computes plugin id, which represents a unique syntest plugin in the cluster
func ComputePluginId(testName string, namespace string, agentId string) string {
	return testName + "/" + namespace + "/" + agentId
}

// ComputeSynTestConfigId Computes config id, which is a unique identifier for a syntest config using name and namespace
func ComputeSynTestConfigId(testName string, testNamespace string) string {
	return testName + "/" + testNamespace
}

// GetPluginIdComponents Splits the plugin Id into all its different compoenents
func GetPluginIdComponents(pluginId string) (testName, testNs, podName, podNs string, err error) {
	comp := strings.Split(pluginId, "/")
	if len(comp) < 4 {
		return "", "", "", "", errors.New("invalid id, not enough components")
	}
	return comp[0], comp[1], comp[2], comp[3], nil
}

// ComputeAgentId Computes agent id: it's just a string representing the pod name & namespace
func ComputeAgentId(podName string, namespace string) string {
	return podName + "/" + namespace
}

// IsAgentValidForSynTest checks if the agent matches the selectors in the SynTest
func IsAgentValidForSynTest(agentConfig AgentConfig, agentId, testName, testNs, testNodeSelector string,
	testPodLabelSelector, testLabels map[string]string, logger hclog.Logger) (bool, error) {

	logger.Debug("checking agent selector for syntest", "testName", testName, "testNs", testNs,
		"agentId", agentId, "nodeSelector", testNodeSelector, "podLabelSelector", testPodLabelSelector)

	// if watchNamespaceSet is not empty, then check if the namespace is in the set
	if len(agentConfig.MatchNamespaceSet) > 0 {
		if ok := agentConfig.MatchNamespaceSet[testNs]; !ok {
			logger.Debug("syntest not in a 'watched' namespace, ignoring syntest...", "test", testName, "testNs", testNs, "watchedNs", agentConfig.MatchNamespaceSet)
			return false, nil
		}
	}

	// if nodeSelector is not empty, then check if the node selector matches the node name
	if testNodeSelector != "" {
		matchesNode, err := filepath.Match(testNodeSelector, agentConfig.RunTimeInfo.NodeName)
		if err != nil {
			return false, errors.Wrap(err, "error matching nodeSelector")
		}
		if !matchesNode {
			logger.Debug("syntest nodeSelector didn't match, ignoring syntest...", "selector", testNodeSelector, "node", agentConfig.RunTimeInfo.NodeName)
			return false, nil
		}
	}

	// check if the syntest has labels that the agent expects, otherwise dont run this test
	if len(agentConfig.MatchTestLabels) > 0 {
		for k, v := range agentConfig.MatchTestLabels {
			if testLabels[k] != v {
				logger.Debug("syntest labels didnt match the ones that agent expects ", "agentExpectsLabels", agentConfig.MatchTestLabels, "testLabels", agentConfig.RunTimeInfo.NodeName)
				return false, nil
			}
		}
	}

	// if podLabelSelector is not empty, then check if the selector matches the pod labels for the agent
	if len(testPodLabelSelector) > 0 {
		for k, v := range testPodLabelSelector {
			// check for special labels (like agentId, agentNs, podName) match
			if k == SpecialKeyAgentId && v == agentId {
				continue
			}
			if k == SpecialKeyAgentNs {
				matchesAgentNs, err := filepath.Match(v, agentConfig.RunTimeInfo.AgentNamespace)
				if err != nil {
					return false, errors.Wrap(err, "error matching "+SpecialKeyAgentNs+" in podLabelSelector")
				}
				if matchesAgentNs {
					continue
				}
			}
			if k == SpecialKeyPodName {
				matchesPodName, err := filepath.Match(v, agentConfig.RunTimeInfo.PodName)
				if err != nil {
					return false, errors.Wrap(err, "error matching "+SpecialKeyPodName+" in podLabelSelector")
				}
				if matchesPodName {
					continue
				}
			}
			// check if the labels match
			if val, ok := agentConfig.RunTimeInfo.PodLabels[k]; ok {
				matchesPodLabel, err := filepath.Match(v, val)
				if err != nil {
					return false, errors.Wrap(err, "error matching podLabelSelector")
				}
				if !matchesPodLabel {
					logger.Debug("syntest podLabelSelector value not did not match, ignoring syntest...", "key", k, "selectorVal", v, "podLabelVal", val)
					return false, nil
				}
			} else {
				logger.Debug("syntest podLabelSelector key not found in pod, ignoring syntest...", "selector", testPodLabelSelector, "podLabels", agentConfig.RunTimeInfo.PodLabels)
				return false, nil
			}
		}
	}

	// everything matches
	return true, nil
}

// TestDetailsLogger is a simple helper struct to output logs in the test result details
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
