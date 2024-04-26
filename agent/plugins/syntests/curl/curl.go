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
	"github.com/cisco-open/synthetic-heart/common"
	"github.com/cisco-open/synthetic-heart/common/proto"
	"github.com/hashicorp/go-plugin"
	"github.com/pkg/errors"
	"log"
	"os/exec"
	"strconv"
	"strings"
)

type CurlTest struct {
	config CurlTestConfig
}

type CurlTestConfig struct {
	Url           string         `yaml:"url"`
	OutputOptions []OutputOption `yaml:"outputOptions"`
}

type OutputOption struct {
	Name             string `yaml:"name"`
	PrometheusMetric bool   `yaml:"metric"`
	PrometheusLabel  bool   `yaml:"label"`
}

func (t *CurlTest) Initialise(synTestConfig proto.SynTestConfig) error {
	t.config = CurlTestConfig{}
	log.Println("parsing config")
	err := common.ParseYMLConfig(synTestConfig.Config, &t.config)
	return err
}

func (t *CurlTest) PerformTest(_ proto.Trigger) (proto.TestResult, error) {
	testResult := proto.TestResult{Marks: 0, MaxMarks: 1, Details: map[string]string{}}

	outputOptionsStr := "'"
	for _, outParam := range t.config.OutputOptions {
		outputOptionsStr += "%{" + outParam.Name + "} "
	}
	outputOptionsStr += "'"

	cmd := exec.Command("curl", "-v", "--output", "/dev/null", "--silent", "--write-out", outputOptionsStr, t.config.Url)

	log.Println(cmd.Args)
	out, err := cmd.CombinedOutput()
	if err != nil {
		log.Println("error executing curl")
		log.Println(string(out))
		log.Println(err.Error())
		return proto.TestResult{}, errors.Wrap(err, "error executing curl")
	}
	output := string(out)
	log.Println(output)

	outputLines := strings.Split(output, "\n")
	relevantLine := outputLines[len(outputLines)-1]

	// remove the quotes from the result
	relevantLine = relevantLine[1 : len(relevantLine)-1]

	log.Println("Parsing line: " + relevantLine)
	// parse the metric values out
	values := strings.Split(relevantLine, " ")

	if len(values) < len(t.config.OutputOptions) {
		return testResult, errors.New("the number of output returned by curl do not match the query")
	}

	outputVals := map[string]string{}
	// Convert array to map, so it can be referred easily
	for i, outputOption := range t.config.OutputOptions {
		outputVals[outputOption.Name] = values[i]
	}

	return t.addPrometheusMetrics(outputVals, testResult)

}

func (t *CurlTest) addPrometheusMetrics(outputVals map[string]string, testResult proto.TestResult) (proto.TestResult, error) {
	promMetrics := common.PrometheusMetrics{
		Gauges: []common.PrometheusGauge{},
	}

	// Add the metrics
	for _, outputOption := range t.config.OutputOptions {
		if !outputOption.PrometheusMetric {
			continue
		}
		metricName := "curl_" + outputOption.Name
		log.Println("adding metric " + outputOption.Name)
		v, err := strconv.ParseFloat(outputVals[outputOption.Name], 64)
		if err != nil {
			return testResult, errors.Wrap(err, "error converting metric to float")
		}
		gauge := common.PrometheusGauge{
			Name:   metricName,
			Help:   "Curl metric for " + outputOption.Name,
			Value:  v,
			Labels: nil,
		}
		promMetrics.Gauges = append(promMetrics.Gauges, gauge)
	}

	// Add the labels
	for _, outputOption := range t.config.OutputOptions {
		if !outputOption.PrometheusLabel {
			continue
		}
		var gauges []common.PrometheusGauge
		for _, gauge := range promMetrics.Gauges {
			if gauge.Labels == nil {
				gauge.Labels = map[string]string{}
			}
			gauge.Labels[outputOption.Name] = outputVals[outputOption.Name]
			gauge.Labels["url"] = t.config.Url // Adding the URL manually
			gauges = append(gauges, gauge)
		}
		promMetrics.Gauges = gauges
	}

	err := common.AddPrometheusMetricsToResults(promMetrics, testResult)
	if err != nil {
		log.Println(err)
		log.Println("error adding prometheus metric, continuing anyway...")
	}

	// If no errors or issues, the test passes
	testResult.Marks = 1
	testResult.MaxMarks = 1
	return testResult, nil
}

func (t *CurlTest) Finish() error {
	return nil
}

func main() {
	pluginImpl := &CurlTest{}

	plugin.Serve(&plugin.ServeConfig{
		HandshakeConfig: common.DefaultTestPluginHandshakeConfig,
		Plugins: map[string]plugin.Plugin{
			common.CurlTestName: &common.SynTestGRPCPlugin{Impl: pluginImpl},
		},
		GRPCServer: plugin.DefaultGRPCServer,
	})
}
