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
	"github.com/pkg/errors"
	"path/filepath"
	"strings"
)

const (
	SyntestPrefix = "test-"
)

// SynTestNameMap is a map of plugin names to go-plugin objs
var SynTestNameMap = map[string]plugin.Plugin{}

// SynTestCmdMap is a map of plugin names to plugin commands
var SynTestCmdMap = map[string][]string{}

// RegisterSynTestPlugin registers a plugin with the plugin manager
func RegisterSynTestPlugin(pluginName string, cmd []string) {
	SynTestNameMap[pluginName] = &common.SynTestGRPCPlugin{}
	SynTestCmdMap[pluginName] = cmd
}

// DiscoverPlugins returns a list of all plugins discovered in the plugin directory
func DiscoverPlugins(config common.PluginDiscoveryConfig) (map[string][]string, error) {
	plugins := map[string][]string{}

	files, err := filepath.Glob(config.Path)
	if err != nil {
		return nil, errors.Wrap(err, "error discovering plugins from path "+config.Path)
	}
	for _, filePath := range files {
		cmd := []string{}

		// Check if the file starts with the syntest prefix, all files must start with test-
		components := strings.Split(filePath, "/")
		fileName := components[len(components)-1]
		if !strings.HasPrefix(fileName, SyntestPrefix) {
			continue
		}

		if config.Cmd == "" { // no cmd specified, use the file itself as a binary
			if !strings.HasPrefix(filePath, "./") {
				cmd = append(cmd, "./"+filePath)
			}
		} else {
			cmd = append(cmd, config.Cmd)
			cmd = append(cmd, filePath)
		}

		// use the file name as the plugin name so test-abc will be "abc" plugin
		pluginName := strings.TrimPrefix(fileName, SyntestPrefix)
		plugins[pluginName] = cmd
	}
	return plugins, nil
}
