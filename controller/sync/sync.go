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

package sync

// package containing code to sync external storage (in case of discrepancies like stale agents)

import (
	"context"
	"fmt"
	"github.com/cisco-open/synthetic-heart/common"
	"github.com/cisco-open/synthetic-heart/common/storage"
	v1 "github.com/cisco-open/synthetic-heart/controller/api/v1"
	"github.com/hashicorp/go-hclog"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/selection"
	"os"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"time"
)

func All(client client.Client, logger hclog.Logger) {
	logger.Info("syncing all")
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

	logger.Info("sync complete")

}

// Agents syncs agents that are deployed to the cluster
func Agents(ctx context.Context, logger hclog.Logger, store storage.SynHeartStore, k8sClient client.Client) error {
	logger.Info("syncing agents")
	// Get all agents pods using label selector
	var podList corev1.PodList
	r, err := labels.NewRequirement(common.K8sDiscoverLabel, selection.Equals, []string{common.K8sDiscoverLabelVal})
	if err != nil {
		return errors.Wrap(err, "error creating label selector requirement")
	}
	listOptions := client.ListOptions{
		LabelSelector: labels.NewSelector().Add(*r),
	}
	err = k8sClient.List(ctx, &podList, &listOptions)
	if err != nil {
		logger.Error("error listing nodes", "err", err)
	}

	// get all agents in redis
	agentsInRedis, err := store.FetchAllAgentStatus(ctx)
	if err != nil {
		return errors.Wrap(err, "error fetching list of all agents from redis")
	}

	// add any new agents to redis - so the rest api can show
	agentPodMap := map[string]bool{}
	for _, pod := range podList.Items {
		// add it to the map for O(1) access later
		agentId := common.ComputeAgentId(pod.Name, pod.Namespace)
		agentPodMap[agentId] = true
		if _, ok := agentsInRedis[agentId]; !ok { // agent doesnt exist on redis
			err = store.WriteAgentStatus(ctx, agentId, common.AgentStatus{}) // write an empty agent status
			if err != nil {
				logger.Warn("error registering new agent in redis, hopefully the agent itself will register, continuing...", "agentId", agentId, "err", err)
			}
		}
	}

	// check and clean up dead agents in redis
	for agentId, _ := range agentsInRedis {
		if !agentPodMap[agentId] {
			logger.Info("deleting old agent " + agentId)
			err := store.DeleteAgentStatus(ctx, agentId)
			if err != nil {
				logger.Warn("error deleting agent status, continuing...", "agentId", agentId, "err", err)
			}
		}
	}

	return nil
}

// SynTests sync all syntests CRDs to redis
func SynTests(ctx context.Context, logger hclog.Logger, store storage.SynHeartStore, k8sClient client.Client) error {
	logger.Info("syncing syntest")
	// Get all syntest CRDs
	var synTestList v1.SyntheticTestList
	err := k8sClient.List(ctx, &synTestList)
	if err != nil {
		logger.Error("error listing synTests", "err", err)
	}

	// create a map for O(1) access
	synTestMap := map[string]v1.SyntheticTest{}
	for _, synTest := range synTestList.Items {
		synTestMap[common.ComputeSynTestConfigId(synTest.Name, synTest.Namespace)] = synTest
	}

	// Get all syntest configs from redis
	synTestsInRedis, err := store.FetchAllTestConfig(ctx)
	if err != nil {
		return errors.Wrap(err, "error fetching list of all syntests from redis")
	}

	// check if the syntest crd actually exists, otherwise delete the syntest record from redis
	for synTestConfigId, _ := range synTestsInRedis {
		_, ok := synTestMap[synTestConfigId]
		if !ok {
			logger.Info("deleting old syntest from redis: " + synTestConfigId)
			err := store.DeleteTestConfig(ctx, synTestConfigId)
			if err != nil {
				logger.Warn("error deleting syntest, continuing", "syntest", synTestConfigId, "err", err)
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

	// sync any old test runs and health info also
	for pluginId, _ := range status {
		testName, _, podName, podNs, err := common.GetPluginIdComponents(pluginId)
		if err != nil {
			logger.Warn("unable to parse pluginId, continuing", "pluginId", pluginId, "err", err)
			continue
		}
		agentId := common.ComputeAgentId(podName, podNs)
		isStale := false
		runningTests, ok := activeAgents[agentId]
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
			logger.Info("deleting stale plugin and its data, id: " + pluginId)
			err := store.DeleteAllTestRunInfo(ctx, pluginId)
			if err != nil {
				logger.Warn("unable to delete stale plugin data", "pluginId", pluginId, "err", err)
				continue
			}
		}
	}

	return nil
}

// FetchActiveAgents fetches active agents from redis
func FetchActiveAgents(ctx context.Context, store storage.SynHeartStore, logger hclog.Logger) (map[string]common.AgentStatus, error) {
	logger.Info("fetching active agents from redis")
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
			logger.Info("agent not active, name: " + agentName)
			delete(agents, agentName)
		}
	}
	return agents, nil
}
