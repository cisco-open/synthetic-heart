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
	"github.com/hashicorp/go-plugin"
	"github.com/cisco-open/synthetic-heart/common"
	_ "github.com/cisco-open/synthetic-heart/common/proto"
)

type {{TestName}}Test struct {
	config {{TestName}}TestConfig
}

type {{TestName}}TestConfig struct {
	// --- Any config for your test goes here ---
}

func (t *{{TestName}}Test) Initialise(synTestConfig proto.SynTestConfig) error {
	// --- Any initialization logic goes here ---
	// Called when the plugin is started
	// Use this to parse config, use common.ParseYMLConfig() to parse yaml config
	return nil
}

func (t *{{TestName}}Test) PerformTest(trigger proto.Trigger) (proto.TestResult, error) {
	// Empty test result struct for you to populate - currently set to a fail test result
	testResult := proto.TestResult{Marks: 0, MaxMarks: 1, Details: map[string]string{}}

	// --- Test logic goes here ---
	// Called based on triggers that are set, information about what triggered the test is in trigger variable
	// If an error is returned, the agent will restart the plugin
	return testResult, nil
}

func (t *{{TestName}}Test) Finish() error {
	// --- Any finalization logic goes here
	// Called before the plugin is killed

	return nil
}

func main() {
	pluginImpl := &{{TestName}}Test{}

	plugin.Serve(&plugin.ServeConfig{
		HandshakeConfig: common.DefaultTestPluginHandshakeConfig,
		Plugins: map[string]plugin.Plugin{
			common.{{TestName}}TestName: &common.SynTestGRPCPlugin{Impl: pluginImpl},
		},
		GRPCServer: plugin.DefaultGRPCServer,
	})
}
