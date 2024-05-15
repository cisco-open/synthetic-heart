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

package common

import (
	"context"
	"github.com/cisco-open/synthetic-heart/common/proto"
	"github.com/hashicorp/go-plugin"
	"google.golang.org/grpc"
	"log"
)

type SynTestPluginGRPCClient struct {
	client proto.SynTestPluginClient
}

func (t *SynTestPluginGRPCClient) Initialise(config proto.SynTestConfig) error {
	_, err := t.client.Initialise(context.Background(), &config)
	return err
}

func (t *SynTestPluginGRPCClient) PerformTest(trigger proto.Trigger) (proto.TestResult, error) {
	res, err := t.client.PerformTest(context.Background(), &trigger)
	if res != nil {
		return *res, err
	} else {
		return FailedTestResult(), err
	}
}

func (t *SynTestPluginGRPCClient) Finish() error {
	_, err := t.client.Finish(context.Background(), &proto.Empty{})
	return err
}

type SynTestPluginGRPCServer struct {
	Impl SynTestPlugin
	proto.UnimplementedSynTestPluginServer
}

func (s *SynTestPluginGRPCServer) Initialise(ctx context.Context, config *proto.SynTestConfig) (*proto.Empty, error) {
	err := s.Impl.Initialise(*config)
	return &proto.Empty{}, err
}

func (s *SynTestPluginGRPCServer) PerformTest(ctx context.Context, trigger *proto.Trigger) (*proto.TestResult, error) {
	log.Println("--- START OF TEST --- ")
	res, err := s.Impl.PerformTest(*trigger)
	log.Println("--- END OF TEST --- ")
	return &res, err
}

func (s *SynTestPluginGRPCServer) Finish(context.Context, *proto.Empty) (*proto.Empty, error) {
	err := s.Impl.Finish()
	return &proto.Empty{}, err
}

type SynTestGRPCPlugin struct {
	plugin.Plugin               // Implement the plugin.Plugin Interface even tho its a GRPC interface (necessary)
	Impl          SynTestPlugin // The real implementation is injected into this variable
}

func (p *SynTestGRPCPlugin) GRPCServer(broker *plugin.GRPCBroker, s *grpc.Server) error { // Used on plugin
	proto.RegisterSynTestPluginServer(s, &SynTestPluginGRPCServer{Impl: p.Impl})
	return nil
}

func (SynTestGRPCPlugin) GRPCClient(ctx context.Context, broker *plugin.GRPCBroker, c *grpc.ClientConn) (interface{}, error) { // Used on host
	return &SynTestPluginGRPCClient{client: proto.NewSynTestPluginClient(c)}, nil
}
