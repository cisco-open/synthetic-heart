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
	Status         RoutineStatus `json:"status" yaml:"status"`
	StatusMsg      string        `json:"statusMsg" yaml:"statusMsg"`
	Config         interface{}   `json:"config" yaml:"config"`
	Restarts       int           `json:"restarts" yaml:"restarts"`
	RestartBackOff string        `json:"restartBackOff" yaml:"restartBackOff"`
	TotalRestarts  int           `json:"totalRestarts" yaml:"totalRestarts"`
	RunningSince   time.Time     `json:"runningSince" yaml:"runningSince"`
	LastUpdated    time.Time     `json:"lastUpdated" yaml:"lastUpdated"`
}

type AgentStatus struct {
	SynTests    []string    `json:"syntests"`
	StatusTime  string      `json:"statusTime"`
	AgentConfig AgentConfig `json:"agentConfig"`
}

type AgentConfig struct {
	MatchTestNamespaces []string                `yaml:"matchTestNamespaces" json:"matchTestNamespaces"`
	MatchTestLabels     map[string]string       `yaml:"matchTestLabels" json:"matchTestLabels"`
	LabelFileLocation   string                  `yaml:"labelFileLocation" json:"labelFileLocation"`
	SyncFrequency       time.Duration           `yaml:"syncFrequency" json:"syncFrequency"`
	GracePeriod         time.Duration           `yaml:"gracePeriod" json:"gracePeriod"`
	PrometheusConfig    PrometheusConfig        `yaml:"prometheus" json:"prometheusConfig"`
	StoreConfig         StorageConfig           `yaml:"storage" json:"storeConfig"`
	PrintPluginLogs     PrintPluginLogOption    `yaml:"printPluginLogs" json:"printPluginLogs"`
	EnabledPlugins      []PluginDiscoveryConfig `yaml:"enabledPlugins" json:"enabledPlugins"`
	DebugMode           bool                    `yaml:"debugMode" json:"debugMode"`

	// Populated at run time
	DiscoveredPlugins map[string][]string `json:"discoveredPlugins"`
	RunTimeInfo       AgentInfo           `json:"runTimeInfo"`
	MatchNamespaceSet map[string]bool     `json:"matchNamespaceSet"` // so we can check if a namespace is being watched in O(1)
}

type PluginDiscoveryConfig struct {
	Path string
	Cmd  string
}

type AgentInfo struct {
	NodeName       string            `json:"nodeName"`       // derived from Downward api
	PodName        string            `json:"podName"`        // derived from Downward api
	PodLabels      map[string]string `json:"podLabels"`      // derived from Downward api
	AgentNamespace string            `json:"agentNamespace"` // derived from Downward api
}

type SyntestConfigSummary struct {
	ConfigId    string `json:"configId"`
	Version     string `json:"version"`
	DisplayName string `json:"displayName"`
	Description string `json:"description"`
	Namespace   string `json:"namespace"`
	Plugin      string `json:"plugin"`
	Repeat      string `json:"repeat"`
}

type SyntestConfigStatus struct {
	Deployed  bool   `json:"deployed"`
	Message   string `json:"message"`
	Agent     string `json:"agent"`
	Timestamp string `json:"timestamp"`
}

type StorageConfig struct {
	Type       string        `yaml:"type"`
	BufferSize int           `yaml:"bufferSize"`
	Address    string        `yaml:"address"`
	ExportRate time.Duration `yaml:"exportRate"`
}

type PrometheusConfig struct {
	ServerAddress     string            `yaml:"address"`
	Push              bool              `yaml:"push"`
	PrometheusPushUrl string            `yaml:"pushUrl"`
	Labels            map[string]string `yaml:"labels"`
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
