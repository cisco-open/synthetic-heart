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
	"fmt"
	"github.com/cisco-open/synthetic-heart/common"
	"github.com/cisco-open/synthetic-heart/common/proto"
	"github.com/go-ping/ping"
	"github.com/hashicorp/go-plugin"
	"github.com/pkg/errors"
	"log"
	"time"
)

const PluginName = "ping"

type PingTest struct {
	config   PingTestConfig
	timeout  time.Duration
	interval time.Duration
}

type PingTestConfig struct {
	Domain     string
	Pings      int
	Interval   string
	Privileged bool
}

func (t *PingTest) Initialise(synTestConfig proto.SynTestConfig) error {
	t.config = PingTestConfig{}
	err := common.ParseYMLConfig(synTestConfig.Config, &t.config)
	if err != nil {
		return errors.Wrap(err, "error parsing config")
	}
	if t.config.Pings <= 0 {
		return errors.Wrap(err, "error in config, pings must be > 0")
	}
	t.interval, err = time.ParseDuration(t.config.Interval)
	if err != nil {
		return errors.Wrap(err, "error parsing interval")
	}
	t.timeout, err = time.ParseDuration(synTestConfig.Timeouts.Run)
	if err != nil { // should never happen
		return errors.Wrap(err, "error parsing timeout in the test config, how did this get through agent?")
	}
	return nil
}

func (t *PingTest) PerformTest(trigger proto.Trigger) (proto.TestResult, error) {
	testResult := proto.TestResult{Marks: 0, MaxMarks: uint64(t.config.Pings), Details: map[string]string{}}

	pinger, err := ping.NewPinger(t.config.Domain)
	if err != nil {
		return testResult, errors.Wrap(err, "error creating the pinger")
	}
	log.Println("created pinger")

	if t.config.Privileged {
		pinger.SetPrivileged(true)
	}
	pinger.Timeout = t.timeout - 1*time.Second
	pinger.Count = t.config.Pings
	pinger.Interval = t.interval
	pinger.OnRecv = func(pkt *ping.Packet) {
		log.Println(fmt.Sprintf("%d bytes from %s: icmp_seq=%d time=%v", pkt.Nbytes, pkt.IPAddr, pkt.Seq, pkt.Rtt))
	}
	pinger.OnFinish = func(stats *ping.Statistics) {
		log.Println(fmt.Sprintf("--- %s ping statistics ---", stats.Addr))
		log.Println(fmt.Sprintf("%d packets transmitted, %d packets received, %v%% packet loss",
			stats.PacketsSent, stats.PacketsRecv, stats.PacketLoss))
		log.Println(fmt.Sprintf("round-trip min/avg/max/stddev = %v/%v/%v/%v",
			stats.MinRtt, stats.AvgRtt, stats.MaxRtt, stats.StdDevRtt))
		testResult.Marks = uint64(stats.PacketsRecv)
	}

	log.Println(fmt.Sprintf("PING %s (%s):\n", pinger.Addr(), pinger.IPAddr()))
	pinger.Run()
	return testResult, nil
}

func (t *PingTest) Finish() error {
	return nil
}

func main() {
	pluginImpl := &PingTest{}

	plugin.Serve(&plugin.ServeConfig{
		HandshakeConfig: common.DefaultTestPluginHandshakeConfig,
		Plugins: map[string]plugin.Plugin{
			PluginName: &common.SynTestGRPCPlugin{Impl: pluginImpl},
		},
		GRPCServer: plugin.DefaultGRPCServer,
	})
}
