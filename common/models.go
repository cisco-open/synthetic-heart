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

import "time"

/*
 * Any Go structs shared between different components go here
 */

type PluginState struct {
	Status        RoutineStatus `json:"status" yaml:"status"`
	StatusMsg     string        `json:"statusMsg" yaml:"statusMsg"`
	LastMsg       string        `json:"lastMsg" yaml:"lastMsg"`
	Config        interface{}   `json:"config" yaml:"config"`
	Restarts      int           `json:"restarts" yaml:"restarts"`
	TotalRestarts int           `json:"totalRestarts" yaml:"totalRestarts"`
	RunningSince  time.Time     `json:"runningSince" yaml:"runningSince"`
	LastUpdated   time.Time     `json:"lastUpdated" yaml:"lastUpdated"`
}

type AgentStatus struct {
	SynTests   []string `json:"syntests"`
	StatusTime string   `json:"statusTime"`
}

type KafkaHeartBeat struct {
	Timestamp     string   `json:"timestamp"`
	ClusterName   string   `json:"clusterName"`
	ClusterDomain string   `json:"clusterDomain"`
	HostName      string   `json:"hostName"`
	Tags          []string `json:"tags"`
}

type PrometheusMetrics struct {
	Gauges []PrometheusGauge `yaml:"gauges"`
}

type PrometheusGauge struct {
	Name   string            `yaml:"name"`
	Help   string            `yaml:"help"`
	Value  float64           `yaml:"value"`
	Labels map[string]string `yaml:"labels"`
}

type ChronySource struct {
	Mode                    string
	State                   string
	Ip                      string
	Stratum                 int
	Poll                    int
	ReachRegister           int
	LastRx                  string
	LastSampleOffset        string
	LastSampleActualOffset  string
	LastSampleMarginOfError string
}
