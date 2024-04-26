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
	"fmt"
	"github.com/cisco-open/synthetic-heart/agent/utils"
	"github.com/cisco-open/synthetic-heart/common"
	"github.com/cisco-open/synthetic-heart/common/proto"
	"github.com/hashicorp/go-hclog"
	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/prometheus/client_golang/prometheus/push"
	"gopkg.in/yaml.v3"
	"net/http"
	"regexp"
	"sync"
	"time"
)

var invalidMetricNameRegex = regexp.MustCompile(`[^a-zA-Z_][^a-zA-Z0-9_]*`) // All invalid chars, from: https://prometheus.io/docs/practices/naming/#metric-names

type PrometheusExporter struct {
	config      PrometheusConfig
	broadcaster *utils.Broadcaster
	srv         *http.Server
	gauges      map[string]*prometheus.GaugeVec
	pusher      *push.Pusher
	logger      hclog.Logger
}

type PrometheusConfig struct {
	ServerAddress     string `yaml:"address"`
	Push              bool   `yaml:"push"`
	PrometheusPushUrl string `yaml:"pushUrl"`
}

const (
	MarksGauge    = "syntheticheart_marks_total"
	MaxMarksGauge = "syntheticheart_max_marks_total"
	TimeGauge     = "syntheticheart_runtime_ns"
	CustomGauge   = "syntheticheart_%s" // Gauge name
)

func NewPrometheusExporter(logger hclog.Logger, config PrometheusConfig, agentId string, debugMode bool) PrometheusExporter {
	p := PrometheusExporter{}
	p.config = config
	p.logger = logger
	if !p.config.Push {
		mux := http.NewServeMux()
		mux.Handle("/metrics", promhttp.Handler())
		if debugMode {
			mux.Handle("/debug/", http.DefaultServeMux)
		}
		srv := &http.Server{Addr: p.config.ServerAddress, Handler: mux}
		p.srv = srv
	} else {
		p.pusher = push.New(p.config.PrometheusPushUrl, agentId).Gatherer(prometheus.DefaultGatherer)
	}
	p.gauges = map[string]*prometheus.GaugeVec{}
	return p
}

func (p *PrometheusExporter) Run(ctx context.Context, broadcaster *utils.Broadcaster, configChange chan struct{}) {
	resChan := broadcaster.SubscribeToTestRuns("prometheus", common.DefaultChannelSize, p.logger)
	wg := sync.WaitGroup{}

	if !p.config.Push {
		// start the prometheus client server
		wg.Add(1)
		go func() { p.startPrometheusClient(); wg.Done() }()
	}
	for {
		select {
		case res := <-resChan:
			err := p.ExportTestRunMetrics(res)
			if err != nil {
				p.logger.Error("error exporting test run metrics", "err", err)
			}

		case <-configChange:
			p.logger.Info("config changed, cleaning up prometheus")
			p.Cleanup()
		case <-ctx.Done():
			// stop the client server and wait for it to stop
			err := p.stopPrometheusClient()
			if err != nil {
				p.logger.Error("error exporting test run metrics", "err", err)
			}
			if !p.config.Push {
				p.logger.Info("waiting prometheus client server to finish")
				wg.Wait()
			}
			p.logger.Info("prometheus exporter exiting")
			return
		}
	}
}

func (p *PrometheusExporter) Cleanup() {
	for _, gauge := range p.gauges {
		gauge.Reset()
	}
}

func (p *PrometheusExporter) ExportTestRunMetrics(res proto.TestRun) error {
	p.logger.Info("interpreting results d", "test", res.TestConfig.Name)
	// Add the default metrics
	err := p.addDefaultMetrics(res)
	if err != nil {
		return errors.Wrap(err, "error adding default metrics")
	}

	// Adds custom metrics passed by the tests (if there are any)
	if promResults, ok := res.TestResult.Details[common.PrometheusKey]; ok {
		err := p.addCustomMetrics(promResults, res)
		if err != nil {
			return errors.Wrap(err, "error adding custom metrics")
		}
	}

	if p.config.Push {
		err := p.pusher.Push()
		if err != nil {
			return errors.Wrap(err, "error pushing metrics to push server")
		}
	}
	return nil
}

func (p *PrometheusExporter) startPrometheusClient() {
	p.logger.Info("starting prom client server...")
	if err := p.srv.ListenAndServe(); err != http.ErrServerClosed {
		p.logger.Error("error when starting prometheus client", "err", err)
	}
}

func (p *PrometheusExporter) stopPrometheusClient() error {
	p.logger.Info("closing client prometheus server...")
	err := p.srv.Close()
	return err
}

func (p *PrometheusExporter) addDefaultMetrics(testRun proto.TestRun) error {
	// Add the marks of the test as a prometheus Gauge
	p.setOrCreateGauge(MarksGauge,
		"The marks obtained in the test",
		float64(testRun.TestResult.Marks),
		getMetadataLabels(testRun),
		testRun)

	// Add the max marks of the test as a prometheus Gauge
	p.setOrCreateGauge(MaxMarksGauge,
		"The max marks in the test",
		float64(testRun.TestResult.MaxMarks),
		getMetadataLabels(testRun),
		testRun)

	// Add the runtime of the test as a prometheus Gauge
	runtimeGaugeName := TimeGauge
	testStartTime, err := time.Parse(common.TimeFormat, testRun.StartTime)
	if err != nil {
		return err
	}
	testEndTime, err := time.Parse(common.TimeFormat, testRun.EndTime)
	if err != nil {
		return err
	}
	runtime := testEndTime.Sub(testStartTime)
	p.setOrCreateGauge(runtimeGaugeName,
		"The runtime of the test in nano seconds",
		float64(runtime.Nanoseconds()),
		getMetadataLabels(testRun),
		testRun)

	return nil
}

// Parses the custom metrics passed by the test result
func (p *PrometheusExporter) addCustomMetrics(promMetricsStr string, res proto.TestRun) error {
	p.logger.Debug("processing prometheus specific results...")
	promMetrics := common.PrometheusMetrics{}
	err := yaml.Unmarshal([]byte(promMetricsStr), &promMetrics)
	if err != nil {
		return err
	}
	for _, gauge := range promMetrics.Gauges {
		gaugeName := cleanMetricName(fmt.Sprintf(CustomGauge, gauge.Name))
		p.logger.Debug("adding " + gaugeName)
		// Inject metadata labels into the custom gauge
		for k, v := range getMetadataLabels(res) {
			gauge.Labels[k] = v
		}
		p.setOrCreateGauge(gaugeName,
			gauge.Help,
			gauge.Value,
			gauge.Labels,
			res)
	}
	return nil
}

func (p *PrometheusExporter) setOrCreateGauge(name string, help string, value float64, labels map[string]string, res proto.TestRun) {
	if _, ok := p.gauges[name]; !ok {
		var labelKeys []string
		for k := range labels {
			labelKeys = append(labelKeys, k)
		}

		p.gauges[name] = promauto.NewGaugeVec(prometheus.GaugeOpts{
			Name: name,
			Help: help,
		}, labelKeys)
	}

	g, err := p.gauges[name].GetMetricWith(labels)
	if err != nil {
		p.logger.Error("error getting metric with labels", "err", err)
	}
	g.Set(value)
}

func cleanMetricName(dirty string) string {
	return invalidMetricNameRegex.ReplaceAllString(dirty, "_")
}

func getMetadataLabels(rh proto.TestRun) map[string]string {
	return map[string]string{
		"test_name":    rh.TestConfig.Name,
		"test_agent":   rh.TestConfig.Etc["agentId"],
		"test_cluster": rh.TestConfig.Etc["clusterName"],
		"test_domain":  rh.TestConfig.Etc["clusterDomain"],
		"test_node":    rh.TestConfig.Etc["nodeName"]}
}