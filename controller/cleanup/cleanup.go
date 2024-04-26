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

package cleanup

// package containing code to cleanup external storage (in case of discrepancies like stale agents)

import (
	"context"
	"fmt"
	"github.com/cisco-open/synthetic-heart/common"
	"github.com/cisco-open/synthetic-heart/common/storage"
	v1 "github.com/cisco-open/synthetic-heart/controller/api/v1"
	"github.com/hashicorp/go-hclog"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	"os"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"time"
)

func All(client client.Client, logger hclog.Logger) {
	ctx := context.Background()
	addr, ok := os.LookupEnv("SYNHEART_STORE_ADDR")
	if !ok {
		logger.Error("SYNHEART_STORE_ADDR env var not set")
		os.Exit(1)
	}
	store, err := storage.NewSynHeartStore(storage.SynHeartStoreConfig{
		Type:       "redis",
		BufferSize: 1000,
		Address:    addr,
	}, logger.Named("redis"))
	if err != nil {
		logger.Error("could not connect to synheart store")
		panic(fmt.Sprintf("could not connect to synheart store, err=%v", err.Error()))
	}
	defer store.Close()
	err = Agents(ctx, logger, store, client)
	if err != nil {
		logger.Warn("error cleaning up agents", "err", err)
	}

	err = SynTests(ctx, logger, store, client)
	if err != nil {
		logger.Warn("error cleaning up syntests", "err", err)
	}

	logger.Info("cleanup complete")

}

// cleans up any agents that do not corrsespond to a k8s node
func Agents(ctx context.Context, logger hclog.Logger, store storage.SynHeartStore, k8sClient client.Client) error {
	// Get all nodes
	var nodeList corev1.NodeList
	err := k8sClient.List(ctx, &nodeList)
	if err != nil {
		logger.Error("error listing nodes", "err", err)
	}
	// create a map for O(1) access
	nodeMap := map[string]bool{}
	for _, node := range nodeList.Items {
		nodeMap[node.GetName()] = true
	}

	// get all agents
	agents, err := store.FetchAllAgentStatus(ctx)
	if err != nil {
		return errors.Wrap(err, "error fetching list of all agents from redis")
	}

	// check if the agentId (i.e. node) actually exists, other wise delete the agent
	for agentId, _ := range agents {
		if !nodeMap[agentId] {
			logger.Info("deleting agent " + agentId)
			err := store.DeleteAgentStatus(ctx, agentId)
			if err != nil {
				logger.Warn("error deleting agent status, continuing", "agentId", agentId, "err", err)
			}
		}
	}
	return nil
}

// cleans up any old syntests (such as ones running on dead or non-existent agents)
func SynTests(ctx context.Context, logger hclog.Logger, store storage.SynHeartStore, k8sClient client.Client) error {
	// Get all syntest CRDs
	var synTestList v1.SyntheticTestList
	err := k8sClient.List(ctx, &synTestList)
	if err != nil {
		logger.Error("error listing synTests", "err", err)
	}
	// create a map for O(1) access
	synTestMap := map[string]v1.SyntheticTest{}
	for _, synTest := range synTestList.Items {
		synTestMap[synTest.Name] = synTest
	}
	// get all syntest configs from redis
	synTestsInRedis, err := store.FetchAllTestConfig(ctx)
	if err != nil {
		return errors.Wrap(err, "error fetching list of all syntests from redis")
	}

	for synTestName, _ := range synTestsInRedis {
		_, ok := synTestMap[synTestName]
		// check if the syntest crd actually exists, otherwise delete the syntest record from redis
		if !ok {
			logger.Info("deleting syntest " + synTestName)
			err := store.DeleteTestConfig(ctx, synTestName)
			if err != nil {
				logger.Warn("error deleting syntest, continuing", "syntest", synTestName, "err", err)
			}
		}
	}

	status, err := store.FetchAllTestRunStatus(ctx)
	if err != nil {
		return errors.Wrap(err, "error fetching statuses from redis")
	}

	// fetch the active agents
	activeAgents, err := FetchActiveAgents(ctx, store, logger)
	if err != nil {
		return errors.Wrap(err, "error fetching list of all agents from redis")
	}

	// cleanup any old test runs and health info also
	for pluginId, _ := range status {
		agent, testName, err := common.GetPluginIdComponents(pluginId)
		if err != nil {
			logger.Warn("unable to parse pluginId, continuing", "pluginId", pluginId, "err", err)
			continue
		}

		isStale := false
		runningTests, ok := activeAgents[agent]
		if !ok { // the agent doesnt exist (or is not active), so the test run data must be old
			isStale = true
		} else {
			isStale = true
			for _, t := range runningTests.SynTests { // check whether the agent is actually running the test
				if testName == t {
					isStale = false
				}
			}
		}

		if isStale {
			logger.Info("deleting plugin " + pluginId)
			err := store.DeleteAllTestRunInfo(ctx, pluginId)
			if err != nil {
				logger.Warn("unable to delete stale plugin data", "pluginId", pluginId, "err", err)
				continue
			}
		}
	}

	return nil
}

// fetches active agents from redis
func FetchActiveAgents(ctx context.Context, store storage.SynHeartStore, logger hclog.Logger) (map[string]common.AgentStatus, error) {
	// fetch all agents and their statuses
	agents, err := store.FetchAllAgentStatus(ctx)
	if err != nil {
		return agents, errors.Wrap(err, "error fetching all agents from store")
	}

	// check whether the agents have posted a recent status update
	for agentName, agent := range agents {
		statusTime, err := time.Parse(common.TimeFormat, agent.StatusTime)
		if err != nil {
			logger.Warn(fmt.Sprintf("couldn't parse last status time for agent: %v", agent))
			continue
		}
		statusLastUpdateAge := time.Now().Sub(statusTime)

		// get the agent status deadline from env
		agentStatusDeadline, ok := os.LookupEnv("AGENT_STATUS_DEADLINE")
		if !ok {
			logger.Error("no AGENT_STATUS_DEADLINE in env")
			os.Exit(1)
		}
		dur, err := time.ParseDuration(agentStatusDeadline)
		if err != nil {
			logger.Error("unable to parse AGENT_STATUS_DEADLINE duration: "+agentStatusDeadline, "err", err.Error())
			os.Exit(1)
		}

		// if the last update was too long ago, delete this agent from active agent map
		if statusLastUpdateAge.Seconds() >= dur.Seconds() {
			delete(agents, agentName)
		}
	}
	return agents, nil
}
