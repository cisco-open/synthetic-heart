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
	"github.com/cisco-open/synthetic-heart/common"
	"github.com/hashicorp/go-plugin"
)

const (
	BinPath       = "./plugins/"
	SyntestPrefix = "test-"
)

func init() {
	// Register SynTest plugins  [ADD TEST PLUGINS HERE]
	// RegisterSynTestPlugin(<name of plugin>, <command to run the plugin (as array)>)
	RegisterSynTestPlugin(common.DnsTestName, []string{BinPath + SyntestPrefix + common.DnsTestName})
	RegisterSynTestPlugin(common.SelfTestRxTestName, []string{BinPath + SyntestPrefix + common.SelfTestRxTestName})
	RegisterSynTestPlugin(common.HttpPingTestName, []string{BinPath + SyntestPrefix + common.HttpPingTestName})
	RegisterSynTestPlugin(common.PingTestName, []string{BinPath + SyntestPrefix + common.PingTestName})
	RegisterSynTestPlugin(common.NetDialTestName, []string{BinPath + SyntestPrefix + common.NetDialTestName})
	RegisterSynTestPlugin(common.CurlTestName, []string{BinPath + SyntestPrefix + common.CurlTestName})
}

var SynTestNameMap = map[string]plugin.Plugin{}
var SynTestCmdMap = map[string][]string{}

func RegisterSynTestPlugin(pluginName string, cmd []string) {
	SynTestNameMap[pluginName] = &common.SynTestGRPCPlugin{}
	SynTestCmdMap[pluginName] = cmd
}
