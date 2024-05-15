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

package storage

import (
	"context"
	"errors"
	"github.com/cisco-open/synthetic-heart/common"
	"github.com/cisco-open/synthetic-heart/common/proto"
	"github.com/hashicorp/go-hclog"
)

type SynHeartStoreConfig struct {
	Type       string `yaml:"type"`
	BufferSize int    `yaml:"bufferSize"`
	Address    string `yaml:"address"`
}

// Interface that a storage for Synthetic Heart must implement
type SynHeartStore interface {
	SubscribeToTestRunEvents(ctx context.Context, channelSize int, pluginId chan<- string) error
	WriteTestRun(ctx context.Context, pluginId string, testRun proto.TestRun) error
	FetchLatestTestRun(ctx context.Context, pluginId string) (proto.TestRun, error)
	FetchAllTestRunIds(ctx context.Context) (map[string]string, error)
	FetchAllTestRunStatus(ctx context.Context) (map[string]string, error)
	DeleteAllTestRunInfo(ctx context.Context, pluginId string) error
	UpdateTestRunStatus(ctx context.Context, pluginId string, status common.TestRunStatus) error

	WritePluginStatus(ctx context.Context, pluginId string, state common.PluginState) error
	FetchPluginStatus(ctx context.Context, pluginId string) (common.PluginState, error)

	SubscribeToConfigEvents(ctx context.Context, channelSize int, configChan chan<- string) error
	WriteTestConfig(ctx context.Context, version string, config proto.SynTestConfig, raw string) error
	DeleteTestConfig(ctx context.Context, configId string) error
	FetchTestConfig(ctx context.Context, configId string) (proto.SynTestConfig, error)
	FetchAllTestConfig(ctx context.Context) (map[string]string, error)

	FetchAllAgentStatus(ctx context.Context) (map[string]common.AgentStatus, error)
	WriteAgentStatus(ctx context.Context, agentId string, status common.AgentStatus) error
	DeleteAgentStatus(ctx context.Context, agentId string) error
	SubscribeToAgentEvents(ctx context.Context, channelSize int, configChan chan<- string) error
	NewAgentEvent(ctx context.Context, event string) error

	Close() error
	Ping(ctx context.Context) error
}

func NewSynHeartStore(config SynHeartStoreConfig, log hclog.Logger) (SynHeartStore, error) {
	switch config.Type {
	case "redis":
		store := NewRedisSynHeartStore(config, log)
		return &store, nil
	default:
		return nil, errors.New("unsupported store type " + config.Type)
	}

}
