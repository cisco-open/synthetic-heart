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
	"context"
	"github.com/cisco-open/synthetic-heart/agent/utils"
	"github.com/cisco-open/synthetic-heart/common"
	"github.com/cisco-open/synthetic-heart/common/storage"
	"github.com/hashicorp/go-hclog"
	"github.com/pkg/errors"
	"sync"
	"time"
)

// ExtStorageHandler manages all communication with external storage (redis)
type ExtStorageHandler struct {
	agentId      string
	Store        storage.SynHeartStore
	config       common.StorageConfig
	logger       hclog.Logger
	filterLock   *sync.Mutex
	seenTestRuns map[string]string // cache of seen test runs map[testPluginId]testRunId
}

func NewExtStorageHandler(agentId string, config common.StorageConfig, logger hclog.Logger) (ExtStorageHandler, error) {
	store, err := storage.NewSynHeartStore(storage.SynHeartStoreConfig{
		Type:       config.Type,
		BufferSize: config.BufferSize,
		Address:    config.Address,
	}, logger)
	if err != nil {
		return ExtStorageHandler{}, err
	}
	return ExtStorageHandler{
		agentId:      agentId,
		Store:        store,
		config:       config,
		logger:       logger.Named("esh"),
		filterLock:   &sync.Mutex{},
		seenTestRuns: map[string]string{},
	}, nil

}

func (esh *ExtStorageHandler) Run(ctx context.Context, broadcaster *utils.Broadcaster, sm *StateMap) error {
	errorChan := make(chan error, 10)

	wg := sync.WaitGroup{}
	esmCtx, cancelAll := context.WithCancel(ctx)

	// sync and wait for all go routines to exit
	defer func() {
		cancelAll()
		esh.logger.Info("waiting for external storage go routines to finish")
		wg.Wait()
		esh.logger.Info("external storage helper exiting")
	}()

	// run plugin health exporter - exports plugin health to external storage
	wg.Add(1)
	go esh.runPluginHealthExporter(esmCtx, &wg, sm)

	// run test run exporter - exports test run to external storage
	wg.Add(1)
	go esh.runTestRunExporter(esmCtx, &wg, broadcaster)

	for {
		select {
		case err := <-errorChan:
			return errors.Wrap(err, "error running external storage manager")

		case <-ctx.Done():
			return nil
		}
	}
}

// Runs the plugin health exporter loop - periodically exports health
func (esh *ExtStorageHandler) runPluginHealthExporter(ctx context.Context, wg *sync.WaitGroup, sm *StateMap) {
	defer wg.Done()
	defer esh.logger.Trace("stopped health exporter")
	healthExportPeriod := time.NewTicker(esh.config.ExportRate)
	for {
		select {
		case <-ctx.Done():
			esh.logger.Info("stopping test run exporter")
			return
		case <-healthExportPeriod.C:
			esh.logger.Debug("exporting plugin health to external storage")
			pluginState := sm.GetAllPluginState()
			agentStatus := sm.GetAgentStatus()

			//Write agent status
			err := esh.Store.WriteAgentStatus(ctx, esh.agentId, agentStatus)
			if err != nil {
				esh.logger.Error("error exporting agent status", "err", err)
			}

			// Write status for all the synthetic test plugins
			for pluginId, state := range pluginState.PluginStates {
				err := esh.Store.WritePluginHealthStatus(ctx, pluginId, state)
				if err != nil {
					esh.logger.Error("error exporting syntest plugin state", "err", err, "pluginId", pluginId)
				}
			}
		}
	}
}

// Runs the test run exporter loop - exports test runs when they happen
func (esh *ExtStorageHandler) runTestRunExporter(ctx context.Context, wg *sync.WaitGroup, broadcaster *utils.Broadcaster) {
	defer wg.Done()
	defer esh.logger.Trace("stopped test run exporter")
	testRunChan := broadcaster.SubscribeToTestRuns("ext-storage-manager", esh.config.BufferSize, esh.logger)
	defer broadcaster.UnsubscribeFromTestRuns(testRunChan, esh.logger)
	for {
		select {
		case <-ctx.Done():
			esh.logger.Info("stopping test run exporter")
			return
		case testRun := <-testRunChan:
			// Check if the testRun originated from another agent, then dont export  - This shouldn't happen
			if testRun.AgentId != esh.agentId {
				esh.logger.Debug("not exporting test, as its from another agent", "testName", testRun.TestConfig.Name, "agentId", testRun.AgentId)
				continue
			}
			esh.logger.Debug("exporting test run to external storage", "testName", testRun.TestConfig.Name)
			err := esh.Store.WriteTestRun(ctx, common.ComputePluginId(testRun.TestConfig.Name, testRun.TestConfig.Namespace, testRun.AgentId), testRun)
			if err != nil {
				esh.logger.Error("error exporting test run", "err", err)
			}
		}
	}
}
