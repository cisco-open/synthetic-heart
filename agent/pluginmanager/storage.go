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
	"fmt"
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
	agentId       string
	Store         storage.SynHeartStore
	config        StorageConfig
	logger        hclog.Logger
	testsToImport map[string]int // filter for which tests to import from other agents (by default none)
	filterLock    *sync.Mutex
	seenTestRuns  map[string]string // cache of seen test runs map[testPluginId]testRunId
}

type StorageConfig struct {
	Type       string        `yaml:"type"`
	BufferSize int           `yaml:"bufferSize"`
	Address    string        `yaml:"address"`
	ExportRate time.Duration `yaml:"exportRate"`
	PollRate   time.Duration `yaml:"pollRate"`
}

func NewExtStorageHandler(agentId string, config StorageConfig, logger hclog.Logger) (ExtStorageHandler, error) {
	store, err := storage.NewSynHeartStore(storage.SynHeartStoreConfig{
		Type:       config.Type,
		BufferSize: config.BufferSize,
		Address:    config.Address,
	}, logger)
	if err != nil {
		return ExtStorageHandler{}, err
	}
	return ExtStorageHandler{
		agentId:       agentId,
		Store:         store,
		config:        config,
		logger:        logger.Named("esh"),
		testsToImport: map[string]int{},
		filterLock:    &sync.Mutex{},
		seenTestRuns:  map[string]string{},
	}, nil

}

func (esh *ExtStorageHandler) Run(ctx context.Context, broadcaster *utils.Broadcaster, sm *StateMap) error {
	extTestRunChan := make(chan string, esh.config.BufferSize)
	errorChan := make(chan error, 10)

	wg := sync.WaitGroup{}
	esmCtx, cancelAll := context.WithCancel(ctx)

	// cleanup and wait for all go routines to exit
	defer func() {
		cancelAll()
		esh.logger.Info("waiting for external storage go routines to finish")
		wg.Wait()
		esh.logger.Info("external storage helper exiting")
	}()

	// subscribe to external test runs (in other agents)
	wg.Add(1)
	go esh.listenForExtTestRuns(esmCtx, &wg, extTestRunChan)

	// run plugin health exporter - exports plugin health to external storage
	wg.Add(1)
	go esh.runPluginHealthExporter(esmCtx, &wg, sm)

	// run test run exporter - exports test run to external storage
	wg.Add(1)
	go esh.runTestRunExporter(esmCtx, &wg, broadcaster)

	// resync ticker
	reSyncPeriod := time.NewTicker(esh.config.PollRate)

	for {
		select {
		case err := <-errorChan:
			return errors.Wrap(err, "error running external storage manager")

		case synTestPluginId := <-extTestRunChan:
			err := esh.importTestRun(ctx, synTestPluginId, broadcaster)
			if err != nil {
				esh.logger.Warn(fmt.Sprintf("error importing test run %s  , continuing...", synTestPluginId))
			}

		case <-reSyncPeriod.C:
			err := esh.ReSyncTestRun(ctx, broadcaster)
			if err != nil {
				esh.logger.Warn("error resyncing testruns", "err", err)
			}
		case <-ctx.Done():
			return nil
		}
	}
}

func (esh *ExtStorageHandler) RegisterTestToImport(testName string) {
	esh.filterLock.Lock()
	esh.testsToImport[testName] += 1
	esh.filterLock.Unlock()
}

func (esh *ExtStorageHandler) UnregisterTestToImport(testName string) {
	esh.filterLock.Lock()
	delete(esh.testsToImport, testName)
	esh.filterLock.Unlock()
}

func (esh *ExtStorageHandler) ReSyncTestRun(ctx context.Context, b *utils.Broadcaster) error {
	testNameVersion, err := esh.Store.FetchAllTestRunIds(ctx)
	if err != nil {
		return err
	}
	for synTestPluginId, latestTestRunId := range testNameVersion {
		cachedTestRunId, ok := esh.seenTestRuns[synTestPluginId]
		if !ok || latestTestRunId != cachedTestRunId {
			err := esh.importTestRun(ctx, synTestPluginId, b)
			if err != nil {
				return errors.Wrap(err, "error importing test run")
			}
		}
	}
	return nil
}

func (esh *ExtStorageHandler) importTestRun(ctx context.Context, synTestPluginId string, broadcaster *utils.Broadcaster) error {
	agentId, testName, err := common.GetPluginIdComponents(synTestPluginId)
	if err != nil {
		return err
	}

	// Only import test runs that are from other agents
	if agentId == esh.agentId {
		esh.logger.Trace("not importing test run as its from the local agent", "pluginId", synTestPluginId)
		return nil
	}

	// check if we should import this test
	esh.filterLock.Lock()
	shouldImport := esh.testsToImport[testName] != 0 || esh.testsToImport["*"] != 0
	esh.filterLock.Unlock()
	if !shouldImport {
		esh.logger.Trace("not importing test run as its not relevant", "pluginId", synTestPluginId)
		return nil
	}

	testRun, err := esh.Store.FetchLatestTestRun(ctx, synTestPluginId)
	if err != nil {
		return err
	}

	// check if we have already imported this test run
	if esh.seenTestRuns[testRun.TestConfig.Name] != testRun.Id {
		esh.seenTestRuns[testRun.TestConfig.Name] = testRun.Id // add it to cache as a seen test run
		esh.logger.Debug("importing test run", "pluginId", synTestPluginId)
		broadcaster.PublishTestRun(testRun, esh.logger)
	} else {
		esh.logger.Trace("not importing test run as its already imported", "pluginId", synTestPluginId)
	}

	return nil
}

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
				err := esh.Store.WritePluginStatus(ctx, pluginId, state)
				if err != nil {
					esh.logger.Error("error exporting syntest plugin state", "err", err, "pluginId", pluginId)
				}
			}
		}
	}
}

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
			// Check if the testRun originated from another agent, then dont export
			if testRun.AgentId != esh.agentId {
				esh.logger.Debug("not exporting test, as its from another agent", "testName", testRun.TestConfig.Name, "agentId", testRun.AgentId)
				continue
			}
			esh.logger.Debug("exporting test run to external storage", "testName", testRun.TestConfig.Name)
			err := esh.Store.WriteTestRun(ctx, common.ComputePluginId(esh.agentId, testRun.TestConfig.Name), testRun)
			if err != nil {
				esh.logger.Error("error exporting test run", "err", err)
			}
		}
	}
}

func (esh *ExtStorageHandler) listenForExtTestRuns(ctx context.Context, wg *sync.WaitGroup, pluginIdChan chan string) {
	defer wg.Done()
	defer esh.logger.Trace("stopped listening for test runs")
	for {
		select {
		case <-ctx.Done():
			return
		default:
			err := esh.Store.SubscribeToTestRunEvents(ctx, esh.config.BufferSize, pluginIdChan)
			if err != nil {
				esh.logger.Error("error subscribing to external test runs", "err", err)
			}
			time.Sleep(1 * time.Second)
		}
	}
}
