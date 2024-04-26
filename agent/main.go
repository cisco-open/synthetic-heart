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
	"context"
	"github.com/cisco-open/synthetic-heart/agent/pluginmanager"
	"github.com/hashicorp/go-hclog"
	_ "net/http/pprof"
	"os"
	"os/signal"
	"sync/atomic"
	"syscall"
)

type SynHeart struct{}

const (
	FileName = "syntheticheart-config"
	FileType = "yaml"
)

const DefaultConfigFilePath string = "/etc/config/syntheticheart-config.yaml"

// The main function manages the config and starts the plugin manager
// It also watches for config changes and kill signal from system
func main() {
	logger := hclog.New(&hclog.LoggerOptions{
		Name:  "main",
		Level: hclog.LevelFromString(os.Getenv("LOG_LEVEL")),
		Color: hclog.ForceColor,

		IncludeLocation: true,
	})

	// Create a safe restart flag (which can safely be set by other go routines)
	var restartSync atomic.Value
	restartSync.Store(true)
	for restartSync.Load().(bool) {
		s := SynHeart{}
		restartSync.Store(false)
		ctx, cancel := context.WithCancel(context.Background())

		// Setup system kill signal watcher - need for Ctrl-C as well as SIGTERM sent by K8s
		sigChan := make(chan os.Signal, 2)
		signal.Notify(sigChan, os.Kill, os.Interrupt, syscall.SIGTERM)
		go func() {
			<-sigChan
			logger.Warn("kill signal from system")
			restartSync.Store(false)
			cancel()
		}()

		configPath := DefaultConfigFilePath
		if len(os.Args) > 1 {
			configPath = os.Args[1]
		}

		// Run the agent
		err := s.Start(ctx, configPath, logger)
		if err != nil {
			logger.Error("error starting pm", "err", err)
			os.Exit(1)
		}
	}

	logger.Info("exiting synthetic heart, good bye!")
}

func (s *SynHeart) Start(ctx context.Context, configPath string, logger hclog.Logger) error {
	// Run the plugin manager
	logger.Debug("starting plugin manager")
	pm, err := pluginmanager.NewPluginManager(configPath)
	if err != nil {
		logger.Error("error creating plugin manager", "err", err)
		return err
	}
	err = pm.Start(ctx) // Blocks until ctx.cancel is called
	if err != nil {
		logger.Error("error starting plugin manager", "err", err)
		return err
	}
	return nil
}
