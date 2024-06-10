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
	"github.com/cisco-open/synthetic-heart/common"
	"github.com/cisco-open/synthetic-heart/common/proto"
	"github.com/cisco-open/synthetic-heart/common/storage"
	synheartv1 "github.com/cisco-open/synthetic-heart/controller/api/v1"
	"github.com/cisco-open/synthetic-heart/controller/sync"
	"github.com/go-logr/logr"
	"github.com/hashicorp/go-hclog"
	"github.com/pkg/errors"
	"gopkg.in/yaml.v3"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"math/rand/v2"
	"os"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
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
	logger := hclog.New(&hclog.LoggerOptions{
		Name:  fmt.Sprintf("reconcile [%s/%s]", request.Name, request.Namespace),
		Level: hclog.LevelFromString(os.Getenv("LOG_LEVEL")),
	})

	/*
		// Reconcile does the following
		1. Puts the config in CRD to storage
		2. Assigns an agent if requested (parameter in CRD)
		3. Updates the status in CRD if there are no agents available or any other issues
	*/

	store, err := ConnectToStorage(logger)
	if err != nil {
		return reconcile.Result{}, err
	}
	defer store.Close()
	logger.Info("==== reconciling ====", "name", request.Name, "namespace", request.Namespace)
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
			// Owned objects are automatically garbage collected. For additional sync logic use finalizers.
			// Return and don't requeue
			logger.Info(fmt.Sprintf("Object %v/%v not found! likely deleted, skipping reconciliation",
				request.NamespacedName.Name, request.NamespacedName.Namespace))

			// Delete test from redis
			logger.Info("deleting syntest", request.Name, request.Namespace)
			err := store.DeleteTestConfig(ctx, common.ComputeSynTestConfigId(request.Name, request.Namespace))
			if err != nil {
				logger.Info("warning: error deleting synthetic test", "err", err)
			}

			return reconcile.Result{}, nil
		}
		// Error reading the object - requeue the request.
		return reconcile.Result{}, err
	}

	configId := common.ComputeSynTestConfigId(instance.Name, instance.Namespace)

	// check if the test has the special key for node/pod assignment
	needsNodeAssignment := instance.Spec.Node == "$"
	needsPodAssignment := false
	if podNameVal, ok := instance.Spec.PodLabelSelector[common.SpecialKeyPodName]; ok && podNameVal == "$" {
		needsPodAssignment = true
	}
	if needsNodeAssignment && needsPodAssignment { // cant ask for both node and pod assignment
		var err = errors.New("error: cant ask for both node and pod assignment")
		logger.Error(err.Error(), "podLabelSelector", instance.Spec.PodLabelSelector, "nodeLabelSelector", instance.Spec.Node)

		err = r.updateTestConfigInRedis(ctx, instance, configId, "",
			instance.Spec.Node, instance.Spec.PodLabelSelector, store, logger)
		if err != nil {
			logger.Error("error updating test config in redis", "name", instance.Name, "err", err.Error())
			return reconcile.Result{}, errors.Wrap(err, "error updating test config in redis")
		}

		r.updateTestStatus(ctx, instance, configId, common.SyntestConfigStatus{
			Deployed: false,
			Message:  "error: config cant have '$' in both node and podLabel selector, use only one",
			Agent:    "",
		}, store, logger)
		return reconcile.Result{}, err
	}

	agent := ""
	node := instance.Spec.Node
	podLabelSelector := map[string]string{}
	for k, v := range instance.Spec.PodLabelSelector {
		podLabelSelector[k] = v
	}

	// if they need node or pod assignment, then remove the special key for node/pod selector
	if needsNodeAssignment || needsPodAssignment {
		logger.Info("assigning agent for syntest", "name", instance.Name, "node", node, "podLabelSelector", podLabelSelector)
		if needsNodeAssignment {
			node = "" // set the node to blank
		}
		if needsPodAssignment {
			delete(podLabelSelector, common.SpecialKeyPodName) // remove the special key for assignment
		}
	}

	// get all active agents
	activeAgents, err := r.getActiveAgents(ctx, store, logger)
	if err != nil {
		logger.Error("error getting active agents, Requeue after 30s", "name", instance.Name, "err", err.Error())
		return reconcile.Result{
			Requeue:      true,
			RequeueAfter: 30 * time.Second,
		}, nil
	}

	// get agents that match the pod/node selctor and can run the test
	validAgents, err := r.getValidAgentsForTest(activeAgents, instance, node, podLabelSelector, logger)
	if err != nil {
		logger.Error("error getting valid agents, Requeue after 30s", "name", instance.Name, "err", err.Error())
		return reconcile.Result{
			Requeue:      true,
			RequeueAfter: 30 * time.Second,
		}, nil
	}

	// if no valid agents then requeue after 3 minutes
	if len(validAgents) <= 0 {
		var err = errors.New("error: no valid agents for syntest")
		logger.Error(err.Error(), "name", instance.Name)
		// update the test config in redis
		err = r.updateTestConfigInRedis(ctx, instance, configId, agent, node, podLabelSelector, store, logger)
		if err != nil {
			logger.Error("error updating test config in redis", "name", instance.Name, "err", err.Error())
			return reconcile.Result{}, errors.Wrap(err, "error updating test config in redis")
		}
		r.updateTestStatus(ctx, instance, configId, common.SyntestConfigStatus{
			Deployed: false,
			Message:  "error: no valid agents found to run the synthetic test on, retry after 1 minute",
			Agent:    "",
		}, store, logger)
		logger.Warn("requesting requeue after 1 minute")
		return reconcile.Result{
			Requeue:      true,
			RequeueAfter: 1 * time.Minute,
		}, nil
	}

	// assign the agent
	if needsNodeAssignment || needsPodAssignment {
		// check if the test is already running on a valid agent, if so dont change it
		_, ok := validAgents[instance.Status.Agent]
		if !ok {
			agent, err = SelectRandomAgent(validAgents)
			if err != nil {
				return reconcile.Result{}, err
			}
			logger.Info("assigning new agent", "name", instance.Name, "agent", agent)
		} else {
			agent = instance.Status.Agent
			logger.Info("keeping existing agent", "name", instance.Name, "agent", agent)
		}
		podLabelSelector[common.SpecialKeyAgentId] = agent
	} else {
		agent = "multiple"
	}

	err = r.updateTestConfigInRedis(ctx, instance, configId, agent, node, podLabelSelector, store, logger)
	if err != nil {
		logger.Error("error updating test config in redis", "name", instance.Name, "err", err.Error())
		return reconcile.Result{}, errors.Wrap(err, "error updating test config in redis")
	}
	// update status
	r.updateTestStatus(ctx, instance, configId, common.SyntestConfigStatus{
		Deployed: true,
		Message:  "deployed to agent",
		Agent:    agent,
	}, store, logger)

	return ctrl.Result{}, nil
}

func (r *SyntheticTestReconciler) updateTestConfigInRedis(ctx context.Context, instance *synheartv1.SyntheticTest, configId string,
	newAgent string, node string, podLabelSelector map[string]string, store storage.SynHeartStore, logger hclog.Logger) error {

	// fetch the config from redis
	configInRedis, err := store.FetchTestConfig(ctx, configId)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			logger.Info("config not found in redis, creating new", "name", instance.Name)
		} else {
			errors.Wrap(err, "error fetching test config from redis")
		}
	}

	timeouts := proto.Timeouts{}
	if instance.Spec.Timeouts != nil {
		timeouts = proto.Timeouts{
			Init:   instance.Spec.Timeouts.Init,
			Run:    instance.Spec.Timeouts.Run,
			Finish: instance.Spec.Timeouts.Finish,
		}
	}
	newTestConfig := proto.SynTestConfig{
		Name:                instance.Name,
		Version:             "", // we assign this later
		Labels:              instance.ObjectMeta.Labels,
		PluginName:          instance.Spec.Plugin,
		DisplayName:         instance.Spec.DisplayName,
		Description:         instance.Spec.Description,
		Importance:          instance.Spec.Importance,
		Repeat:              instance.Spec.Repeat,
		NodeSelector:        node,
		PodLabelSelector:    podLabelSelector,
		Namespace:           instance.Namespace,
		DependsOn:           instance.Spec.DependsOn,
		Timeouts:            &timeouts,
		PluginRestartPolicy: instance.Spec.PluginRestartPolicy,
		LogWaitTime:         instance.Spec.LogWaitTime,
		Config:              instance.Spec.Config,
	}

	// check if the version in redis is the same as CRD
	configHash := ComputeHash(fmt.Sprintf("%v", newTestConfig))
	onLatestVersion := configInRedis.Version == configHash

	// return if the synthetic test is on the latest version and theres no change in the agent its supposed to run on
	if onLatestVersion && instance.Status.Agent == newAgent {
		logger.Info("synthetic test is already on latest version and no changes in agent", "version", configHash, "agent", newAgent)
		return nil
	}

	// we need to update the version in redis

	// update the version
	newTestConfig.Version = configHash

	// marshal the CRD as the "raw config"
	rawConfig, err := yaml.Marshal(instance.Spec)
	if err != nil {
		return errors.Wrap(err, "error marshalling spec yaml")
	}

	// write to redis
	logger.Info("updating test config in redis", "name", instance.Name, "version", configHash, "newAgent", newAgent)
	err = store.WriteTestConfig(ctx, newTestConfig, string(rawConfig))
	if err != nil {
		return errors.Wrap(err, "error writing test config to redis")
	}

	return nil
}

func (r *SyntheticTestReconciler) getActiveAgents(ctx context.Context, store storage.SynHeartStore, logger hclog.Logger) (map[string]common.AgentStatus, error) {
	activeAgents, err := sync.FetchActiveAgents(ctx, store, logger)
	if err != nil {
		return activeAgents, errors.Wrap(err, "error fetching active agents")
	}
	return activeAgents, nil
}

func (r *SyntheticTestReconciler) getValidAgentsForTest(activeAgents map[string]common.AgentStatus, instance *synheartv1.SyntheticTest,
	node string, podLabelSelector map[string]string, logger hclog.Logger) (map[string]bool, error) {
	validAgents := map[string]bool{}

	for agentId, agentStatus := range activeAgents {
		// check if the agent is valid for the syntest
		ok, err := common.IsAgentValidForSynTest(agentStatus.AgentConfig, agentId, instance.Name, instance.Namespace, node, podLabelSelector, instance.Labels, logger)
		if err != nil {
			return validAgents, errors.Wrap(err, "error checking agent selector")
		}
		if ok {
			validAgents[agentId] = true
		}
	}
	return validAgents, nil
}

func (r *SyntheticTestReconciler) updateTestStatus(ctx context.Context, instance *synheartv1.SyntheticTest,
	configId string, status common.SyntestConfigStatus, store storage.SynHeartStore, logger hclog.Logger) {
	logger.Info("updating status", "name", instance.Name)

	// update the timestamp
	status.Timestamp = time.Now().Format(common.TimeFormat)

	// update the status in redis
	err := store.WriteTestConfigStatus(ctx, configId, status)
	if err != nil {
		logger.Info("warning: unable to update status in redis", "err", err)
	}

	// update the status in the CRD
	instance.Status.Deployed = status.Deployed
	instance.Status.Agent = status.Agent
	instance.Status.Message = status.Message
	err = r.Client.Status().Update(ctx, instance)
	if err != nil {
		logger.Info("warning: unable to update status in CRD", "err", err)
	}
}

func ComputeHash(in string) string {
	hmd5 := md5.Sum([]byte(in))
	return fmt.Sprintf("%x", hmd5)
}

func SelectRandomAgent(validAgents map[string]bool) (string, error) {
	index := 0
	if len(validAgents) > 1 {
		index = rand.IntN(len(validAgents) - 1)
	}
	i := 0
	for a, _ := range validAgents {
		if i == index {
			return a, nil
		}
		i++
	}
	return "", errors.New("error selecting random agent, index out of range")
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

	// run sync every 5 minutes
	go func() {
		log := logger.Named("sync")
		ticker := time.NewTicker(5 * time.Minute)
		defer ticker.Stop()
		time.Sleep(5 * time.Second)    // give time for the controller to setup
		sync.All(mgr.GetClient(), log) // run once at start
		log.Info("starting sync loop")
		for {
			<-ticker.C
			log.Info("periodic sync ...")
			sync.All(mgr.GetClient(), log)

			// send a reconcile event after sync
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
				log.Error("couldn't subscribe for agent changes, check redis connection", "err", err)
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
