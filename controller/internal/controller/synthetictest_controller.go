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

package controller

import (
	"context"
	"crypto/md5"
	"fmt"
	"github.com/cisco-open/synthetic-heart/common/proto"
	"github.com/cisco-open/synthetic-heart/common/storage"
	synheartv1 "github.com/cisco-open/synthetic-heart/controller/api/v1"
	"github.com/cisco-open/synthetic-heart/controller/cleanup"
	"github.com/go-logr/logr"
	"github.com/hashicorp/go-hclog"
	"github.com/pkg/errors"
	"gopkg.in/yaml.v3"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"math/rand"
	"os"
	"path/filepath"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
	"strings"
	"time"
)

// SyntheticTestReconciler reconciles a SyntheticTest object
type SyntheticTestReconciler struct {
	client.Client
	Log    logr.Logger
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=synheart.infra.webex.com,resources=synthetictests,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=synheart.infra.webex.com,resources=synthetictests/status,verbs=get;update;patch

func (r *SyntheticTestReconciler) Reconcile(ctx context.Context, request ctrl.Request) (ctrl.Result, error) {
	reqLogger := r.Log.WithValues("synthetictest", request.NamespacedName)

	logger := hclog.New(&hclog.LoggerOptions{
		Name:  "reconcile",
		Level: hclog.LevelFromString(os.Getenv("LOG_LEVEL")),
	})
	store, err := ConnectToStorage(logger)
	if err != nil {
		return reconcile.Result{}, err
	}
	defer store.Close()

	instance := &synheartv1.SyntheticTest{
		TypeMeta:   metav1.TypeMeta{},
		ObjectMeta: metav1.ObjectMeta{},
		Spec:       synheartv1.SyntheticTestSpec{},
		Status:     synheartv1.SyntheticTestStatus{},
	}

	// Fetch the instance of SyntheticTest
	err = r.Client.Get(ctx, request.NamespacedName, instance)
	if err != nil {
		if k8serrors.IsNotFound(err) {
			// Request object not found, could have been deleted after reconcile request.
			// Owned objects are automatically garbage collected. For additional cleanup logic use finalizers.
			// Return and don't requeue
			reqLogger.Info(fmt.Sprintf("Object %v/%v not found! likely deleted, skipping reconciliation",
				request.NamespacedName.Name, request.NamespacedName.Namespace))

			// Delete test from redis
			reqLogger.Info("deleting syntest " + request.NamespacedName.Name)
			err := store.DeleteTestConfig(ctx, request.NamespacedName.Name)
			if err != nil {
				reqLogger.Info("warning: error deleting synthetic test", "err", err)
			}

			return reconcile.Result{}, nil
		}
		// Error reading the object - requeue the request.
		return reconcile.Result{}, err
	}

	// cleanup any old agents
	err = cleanup.Agents(ctx, logger, store, r.Client)
	if err != nil {
		reqLogger.Info("warning: error cleaning up agents", "err", err)
	}

	// Fetch the version currently in redis
	configVersionMap, err := store.FetchAllTestConfig(ctx)
	if err != nil {
		return reconcile.Result{}, errors.Wrap(err, "error fetching synthetic test config versions")
	}

	// get all active agents
	activeAgents, err := cleanup.FetchActiveAgents(ctx, store, logger)
	if err != nil || len(activeAgents) == 0 {
		if len(activeAgents) == 0 {
			reqLogger.Info("error: no agents are currently running")
		}
		return reconcile.Result{}, errors.Wrap(err, "error fetching active agents")
	}

	// assign a valid agent
	agent := ""
	if strings.Contains(instance.Spec.Node, "$") {
		// get all valid agents
		validAgents := map[string]bool{}
		for agentId, _ := range activeAgents {
			agentSelectorGlob := strings.ReplaceAll(instance.Spec.Node, "$", "*")
			if ok, err := filepath.Match(agentSelectorGlob, agentId); err == nil && ok {
				validAgents[agentId] = true
			}
		}
		if len(validAgents) <= 0 {
			reqLogger.Info("no valid agents for syntest", "name", instance.Name)
			return reconcile.Result{}, errors.New("no valid agents for syntest")
		}

		// check if the test is running on an active agent
		_, ok := validAgents[instance.Status.Agent]
		if !ok {
			agent = SelectRandomAgent(validAgents)
		} else {
			agent = instance.Status.Agent
		}
	} else {
		agent = instance.Spec.Node
	}

	// cleanup any old tests
	err = cleanup.SynTests(ctx, logger, store, r.Client)
	if err != nil {
		reqLogger.Info("warning: error cleaning up synthetic tests", "err", err)
	}

	timeouts := proto.Timeouts{}
	if instance.Spec.Timeouts != nil {
		timeouts = proto.Timeouts{
			Init:   instance.Spec.Timeouts.Init,
			Run:    instance.Spec.Timeouts.Run,
			Finish: instance.Spec.Timeouts.Finish,
		}
	}
	testConfig := proto.SynTestConfig{
		Name:                instance.Name,
		PluginName:          instance.Spec.Plugin,
		DisplayName:         instance.Spec.DisplayName,
		Description:         instance.Spec.Description,
		Importance:          instance.Spec.Importance,
		Repeat:              instance.Spec.Repeat,
		AgentSelector:       agent,
		Namespace:           instance.Namespace,
		DependsOn:           instance.Spec.DependsOn,
		Timeouts:            &timeouts,
		PluginRestartPolicy: instance.Spec.PluginRestartPolicy,
		LogWaitTime:         instance.Spec.LogWaitTime,
		Config:              instance.Spec.Config,
	}

	// check if the version in redis is the same as CRD
	configHash := ComputeHash(fmt.Sprintf("%v", testConfig))
	onLatestVersion := configVersionMap[instance.Name] == configHash

	// return if the synthetic test is on the latest version and theres no change in the agent its supposed to run on
	if onLatestVersion && agent == instance.Spec.Node {
		return reconcile.Result{}, nil
	}

	rawConfig, err := yaml.Marshal(instance.Spec)
	if err != nil {
		return reconcile.Result{}, errors.Wrap(err, "error marshalling spec yaml")
	}

	reqLogger.Info("updating syntest "+instance.Name, "version", configHash)
	err = store.WriteTestConfig(ctx, configHash, testConfig, string(rawConfig))
	if err != nil {
		return reconcile.Result{}, err
	}

	// update status
	if !instance.Status.Deployed || instance.Status.Agent != agent {
		reqLogger.Info("updating status", "name", instance.Name, "deployed", true, "agent", agent)
		instance.Status.Deployed = true
		instance.Status.Agent = agent
		err = r.Client.Status().Update(ctx, instance)
		if err != nil {
			reqLogger.Info("warning: unable to update status", "err", err)
		}
	}

	return ctrl.Result{}, nil
}

func ComputeHash(in string) string {
	hmd5 := md5.Sum([]byte(in))
	return fmt.Sprintf("%x\n", hmd5)
}

func SelectRandomAgent(validAgents map[string]bool) string {
	s1 := rand.NewSource(time.Now().UnixNano())
	r1 := rand.New(s1)
	index := r1.Intn(len(validAgents) - 1)
	agentIds := make([]string, 0, len(validAgents))
	for a, _ := range validAgents {
		agentIds = append(agentIds, a)
	}
	return agentIds[index]
}

func ConnectToStorage(logger hclog.Logger) (storage.SynHeartStore, error) {
	addr, ok := os.LookupEnv("SYNHEART_STORE_ADDR")
	if !ok {
		logger.Error("SYNHEART_STORE_ADDR env var not set")
	}
	store, err := storage.NewSynHeartStore(storage.SynHeartStoreConfig{
		Type:       "redis",
		BufferSize: 1000,
		Address:    addr,
	}, logger.Named("redis"))
	if err != nil {
		return store, errors.Wrap(err, "error creating synheart store (redis) client")
	}
	return store, nil

}

// Returns an array of reconcile requests for Synthetic Tests
func (r *SyntheticTestReconciler) ReconcileForExternalEvents(context context.Context, c client.Client) []reconcile.Request {
	requests := []reconcile.Request{}
	var synTestList synheartv1.SyntheticTestList
	err := c.List(context, &synTestList)
	if err != nil {
		return []reconcile.Request{}
	}
	for _, synTest := range synTestList.Items {
		requests = append(requests, reconcile.Request{
			NamespacedName: types.NamespacedName{
				Name:      synTest.Name,
				Namespace: synTest.Namespace,
			},
		})
	}
	return requests
}

func (r *SyntheticTestReconciler) SetupWithManager(mgr ctrl.Manager) error {

	// channel to receive redis events, so we can reconcile
	eventChan := make(chan event.GenericEvent, 100)

	logger := hclog.New(&hclog.LoggerOptions{
		Name:  "loop",
		Level: hclog.LevelFromString(os.Getenv("LOG_LEVEL")),
	})

	// run cleanup every 15 minutes
	go func() {
		log := logger.Named("cleanup")
		ticker := time.NewTicker(15 * time.Minute)
		defer ticker.Stop()
		time.Sleep(5 * time.Second)       // give time for the controller to setup
		cleanup.All(mgr.GetClient(), log) // run once at start
		log.Info("starting clean up loop")
		for {
			<-ticker.C
			log.Info("periodic clean up...")
			cleanup.All(mgr.GetClient(), log)

			// send a reconcile event after cleanup
			eventChan <- event.GenericEvent{
				Object: nil,
			}
		}
	}()

	// subscribe to redis channel for agent registration and un-registration events
	go func() {
		log := logger.Named("agent-watch")
		store, err := ConnectToStorage(log)
		if err != nil {
			log.Error("couldn't connect to storage", "err", err)
			os.Exit(1)
		}
		defer store.Close()
		agentChan := make(chan string, 3)
		go func() {
			err = store.SubscribeToAgentEvents(context.Background(), 1000, agentChan)
			if err != nil {
				log.Error("couldn't subscribe for agent changes", "err", err)
				os.Exit(1)
			}
		}()
		for {
			log.Info("watching for agent signals...")
			signal := <-agentChan
			log.Info("signal from redis: " + signal)
			eventChan <- event.GenericEvent{
				Object: nil,
			}
		}
	}()

	return ctrl.NewControllerManagedBy(mgr).
		For(&synheartv1.SyntheticTest{}).
		WatchesRawSource(&source.Channel{
			Source:         eventChan,
			DestBufferSize: 5,
		}, handler.EnqueueRequestsFromMapFunc(
			func(ctx context.Context, o client.Object) []reconcile.Request {
				return r.ReconcileForExternalEvents(ctx, mgr.GetClient())
			}),
		).Complete(r)
}
