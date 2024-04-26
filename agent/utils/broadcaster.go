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

package utils

import (
	"github.com/cisco-open/synthetic-heart/common"
	"github.com/cisco-open/synthetic-heart/common/proto"
	"github.com/hashicorp/go-hclog"
)

// Broadcaster is a async pub/sub mechanism for go routines
// Used for broadcasting test results between different test routines
type Broadcaster struct {
	testRunSubCh   chan Listener
	testRunUnsubCh chan chan proto.TestRun
	testRunPubCh   chan proto.TestRun
	stopCh         chan struct{}
	logger         hclog.Logger
}

// Struct to hold metadata of the listener, useful for debugging
type Listener struct {
	ResCh       chan proto.TestRun
	Name        string
	ChannelSize int
}

func NewBroadcaster(log hclog.Logger) Broadcaster {
	return Broadcaster{
		logger:         log.Named("broadcaster"),
		testRunSubCh:   make(chan Listener, 1),
		testRunUnsubCh: make(chan chan proto.TestRun, 1),
		testRunPubCh:   make(chan proto.TestRun, common.BroadcasterPublishChannelSize),
		stopCh:         make(chan struct{}),
	}
}
func (b *Broadcaster) PublishTestRun(testRun proto.TestRun, logger hclog.Logger) {
	logger.Debug("publishing test run " + testRun.Id)
	b.testRunPubCh <- testRun
}

func (b *Broadcaster) SubscribeToTestRuns(listenerName string, channelSize int, logger hclog.Logger) chan proto.TestRun {
	logger.Debug("subscribing to broadcaster")
	resCh := make(chan proto.TestRun, channelSize) // queue of test run results if the listener routine is unresponsive
	b.testRunSubCh <- Listener{
		ResCh:       resCh,
		Name:        listenerName,
		ChannelSize: channelSize,
	}
	return resCh
}

func (b *Broadcaster) UnsubscribeFromTestRuns(rCh chan proto.TestRun, logger hclog.Logger) {
	logger.Debug("un-subscribing from broadcaster")
	b.testRunUnsubCh <- rCh
}

func (b *Broadcaster) Stop() {
	b.logger.Debug("stopping broadcaster...")
	close(b.stopCh)
}

func (b *Broadcaster) Start() {
	b.logger.Debug("starting broadcaster...")
	testRunSubs := map[chan proto.TestRun]Listener{}
	for {
		select {
		case <-b.stopCh:
			b.logger.Debug("shutting down")
			return

		case listener := <-b.testRunSubCh:
			b.logger.Debug("test run sub")
			testRunSubs[listener.ResCh] = listener

		case ch := <-b.testRunUnsubCh:
			b.logger.Debug("test run unsub")
			delete(testRunSubs, ch)

		case res := <-b.testRunPubCh:
			b.logger.Debug("new test run event")
			for resCh, listener := range testRunSubs {
				// Check if the testRun passes the provided filters
				select {
				case resCh <- res:
				default:
					b.logger.Warn("listener: not ready to accept more test runs, dropping", "listener", listener,
						"testName", res.TestConfig.Name)
				}
			}
		}
	}
}
