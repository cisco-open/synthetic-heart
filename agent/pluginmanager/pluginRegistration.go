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
	"fmt"
	"github.com/cisco-open/synthetic-heart/common"
	"github.com/hashicorp/go-plugin"
	"github.com/pkg/errors"
	"os"
	"strings"
)

const (
	BinPath       = "./plugins/"
	SyntestPrefix = "test-"
)

// SynTestNameMap is a map of plugin names to plugin objects
var SynTestNameMap = map[string]plugin.Plugin{}

// SynTestCmdMap is a map of plugin names to plugin commands
var SynTestCmdMap = map[string][]string{}

// RegisterSynTestPlugin registers a plugin with the plugin manager
func RegisterSynTestPlugin(pluginName string) error {
	// Check if binary exists
	if _, err := os.Stat(BinPath + SyntestPrefix + pluginName); os.IsNotExist(err) {
		return errors.New(fmt.Sprintf("Plugin binary not found: %s", BinPath+SyntestPrefix+pluginName))
	}
	SynTestNameMap[pluginName] = &common.SynTestGRPCPlugin{}
	SynTestCmdMap[pluginName] = []string{BinPath + SyntestPrefix + pluginName}
	return nil
}

// DiscoverPlugins returns a list of all plugins discovered in the plugin directory
func DiscoverPlugins() ([]string, error) {
	pluginNames := []string{}
	files, err := os.ReadDir(BinPath)
	if err != nil {
		return nil, errors.Wrap(err, "error reading plugin directory")
	}
	for _, file := range files {
		if file.IsDir() {
			continue
		}
		if strings.HasPrefix(file.Name(), SyntestPrefix) {
			pluginNames = append(pluginNames, strings.TrimPrefix(file.Name(), SyntestPrefix))
		}
	}
	return pluginNames, nil
}
