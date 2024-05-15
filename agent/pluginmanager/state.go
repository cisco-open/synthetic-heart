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
	"errors"
	"github.com/cisco-open/synthetic-heart/common"
	"github.com/hashicorp/go-hclog"
	"sync"
	"time"
)

// Stores Plugin State (i.e. whether they are running, no. of restarts etc.)
type StateMap struct {
	logger    hclog.Logger
	c         common.AgentConfig
	stateLock *sync.Mutex
	state     State
}

type State struct {
	PluginStates map[string]common.PluginState `json:"plugins"`
}

func NewStateMap(logger hclog.Logger, agentConfig common.AgentConfig) StateMap {
	return StateMap{
		logger: logger.Named("stateManager"),
		state: State{
			PluginStates: map[string]common.PluginState{},
		},
		c:         agentConfig,
		stateLock: &sync.Mutex{},
	}
}

func (sm *StateMap) SetPluginState(id string, state common.PluginState) {
	sm.stateLock.Lock()
	defer sm.stateLock.Unlock()
	state.LastUpdated = time.Now()
	sm.state.PluginStates[id] = state
}

func (sm *StateMap) SetPluginStatus(id string, status common.RoutineStatus) {
	sm.stateLock.Lock()
	defer sm.stateLock.Unlock()
	state := sm.state.PluginStates[id]
	state.Status = status
	state.LastUpdated = time.Now()
	sm.state.PluginStates[id] = state
}

func (sm *StateMap) GetPluginState(id string) (common.PluginState, error) {
	sm.stateLock.Lock()
	defer sm.stateLock.Unlock()
	state, ok := sm.state.PluginStates[id]
	if !ok {
		return common.PluginState{}, errors.New("plugin not found")
	}
	return state, nil
}

func (sm *StateMap) DeletePluginState(id string) {
	sm.stateLock.Lock()
	defer sm.stateLock.Unlock()
	delete(sm.state.PluginStates, id)
}

// GetAgentStatus returns the state the agent is in including the status of all the plugins, as well as the agent config
func (sm *StateMap) GetAgentStatus() common.AgentStatus {
	status := common.AgentStatus{
		SynTests:    []string{},
		StatusTime:  time.Now().Format(common.TimeFormat),
		AgentConfig: sm.c,
	}
	sm.stateLock.Lock()
	defer sm.stateLock.Unlock()
	for k, _ := range sm.state.PluginStates {
		testName, _, _, _, _ := common.GetPluginIdComponents(k)
		status.SynTests = append(status.SynTests, testName)
	}

	return status
}

func (sm *StateMap) GetAllPluginState() State {
	sm.stateLock.Lock()
	defer sm.stateLock.Unlock()
	stateCopy := State{
		PluginStates: map[string]common.PluginState{},
	}
	for pluginId, pluginState := range sm.state.PluginStates {
		stateCopy.PluginStates[pluginId] = pluginState
	}
	return stateCopy
}
