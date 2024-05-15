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
	"github.com/cisco-open/synthetic-heart/common"
	"github.com/cisco-open/synthetic-heart/common/proto"
	"github.com/cisco-open/synthetic-heart/common/storage"
	"github.com/hashicorp/go-hclog"
	"github.com/hashicorp/go-plugin"
	"github.com/pkg/errors"
	"log"
	"time"
)

const PluginName = "selfTestRx"

type SelfTestRx struct {
	Config SelfTestConfig
}

type SelfTestConfig struct {
	DeadManSwitchDeadline time.Duration               `yaml:"deadManSwitchDeadline"`
	StorageConfig         storage.SynHeartStoreConfig `yaml:"storage"`
}

func (s *SelfTestRx) Initialise(config proto.SynTestConfig) error {
	c := SelfTestConfig{}
	err := common.ParseYMLConfig(config.Config, &c)
	if err != nil {
		return errors.Wrap(err, "error parsing config")
	}
	s.Config = c
	return nil
}

func (s *SelfTestRx) PerformTest(_ proto.Trigger) (proto.TestResult, error) {
	store, err := storage.NewSynHeartStore(s.Config.StorageConfig, hclog.New(&hclog.LoggerOptions{
		Name:  "store",
		Level: hclog.Warn,
	}))

	if err != nil {
		return common.FailedTestResult(), errors.Wrap(err, "error connecting to external storage")
	}
	defer store.Close()

	agentStatus, err := store.FetchAllAgentStatus(context.Background())
	if err != nil {
		return common.FailedTestResult(), errors.Wrap(err, "error fetching agent status")
	}

	var marks = uint64(0)
	var maxMarks = uint64(len(agentStatus))

	for agentId, status := range agentStatus {
		t, err := time.Parse(common.TimeFormat, status.StatusTime)
		if err != nil {
			log.Println(agentId + ":")
			log.Println("\tcannot parse timestamp: " + status.StatusTime)
			continue
		}
		lastUpdateAge := time.Now().Sub(t)

		if lastUpdateAge.Seconds() < s.Config.DeadManSwitchDeadline.Seconds() {
			marks += 1
		} else {
			log.Println(agentId + ":")
			log.Println("\tlast update: " + lastUpdateAge.Round(time.Minute).String() + " ago (too old)")
		}
	}

	return proto.TestResult{
		Marks:    marks,
		MaxMarks: maxMarks,
		Details:  map[string]string{},
	}, nil
}

func (s *SelfTestRx) Finish() error {
	return nil
}

func main() {
	pluginImpl := &SelfTestRx{}

	plugin.Serve(&plugin.ServeConfig{
		HandshakeConfig: common.DefaultTestPluginHandshakeConfig,
		Plugins: map[string]plugin.Plugin{
			PluginName: &common.SynTestGRPCPlugin{Impl: pluginImpl},
		},
		GRPCServer: plugin.DefaultGRPCServer,
	})
}
