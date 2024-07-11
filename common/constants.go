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
	"time"

	"github.com/hashicorp/go-plugin"
	"k8s.io/apimachinery/pkg/util/wait"
)

// Handshake config for Hashicorp plugins to talk to syntest plugins
var DefaultTestPluginHandshakeConfig = plugin.HandshakeConfig{
	ProtocolVersion:  1,
	MagicCookieKey:   "SYNTEST_PLUGIN",
	MagicCookieValue: "synthetic-heart",
}

var DefaultBackoff = wait.Backoff{
	Steps:    20,
	Duration: 10 * time.Millisecond,
	Factor:   3.0,
	Jitter:   0.1,
	Cap:      15 * time.Second,
}

// Time constants
const (
	TimeFormat                    = "2006-01-02T15:04:05.999999999Z07:00"
	BroadcasterPublishChannelSize = 1000
	DefaultSynTestSubChannelSize  = 1000
	DefaultChannelSize            = 1000
	MaxSynTestTimerJitter         = 10000 // milliseconds
	MaxConfigTimerJitter          = 5000  // milliseconds
	DefaultInitTimeout            = 10 * time.Second
	DefaultRunTimeout             = 10 * time.Second
	DefaultFinishTimeout          = 10 * time.Second
	DefaultLogWaitTime            = 15 * time.Millisecond
	DefaultRestartPolicy          = RestartAlways
)

// Trigger Type Values
const (
	TriggerTypeTimer = "timer"
	TriggerTypeTest  = "test"
)

// Importance Values
const (
	ImportanceCritical = "critical"
	ImportanceHigh     = "high"
	ImportanceMedium   = "medium"
	ImportanceLow      = "low"
)

// Special Keys in Details of TestDetailsMap
const (
	ErrorKey      = "_error"      // special key for error details
	LogKey        = "_log"        // special key for logs
	PrometheusKey = "_prometheus" // special key for prometheus metrics
)

// PluginRestartPolicy Values
type PluginRestartPolicy string

const (
	RestartAlways  PluginRestartPolicy = "always"
	RestartNever   PluginRestartPolicy = "never"
	RestartOnError PluginRestartPolicy = "onError"
)

// Synthetic Heart Metric Names
const (
	DroppedTestResults = "dropped_test_results" // How many test results the broadcaster has dropped because the listener wasn't ready
)

type PrintPluginLogOption string

const (
	LogOnFail PrintPluginLogOption = "onFail"
	LogNever  PrintPluginLogOption = "never"
	LogAlways PrintPluginLogOption = "always"
)

type RoutineStatus string

const (
	NotRunning    RoutineStatus = "notRunning"
	Running       RoutineStatus = "running"
	Error         RoutineStatus = "error"
	Restarting    RoutineStatus = "restarting"
	StatusUnknown RoutineStatus = "unknown"
)

// Other constants
const (
	K8sDiscoverLabel    string = "synheart.infra.webex.com/discover"
	K8sDiscoverLabelVal string = "true"
	SpecialKeyNodeName  string = "$nodeName"
	SpecialKeyAgentId   string = "$agentId"
	SpecialKeyPodName   string = "$podName"
	SpecialKeyAgentNs   string = "$agentNamespace"
)
