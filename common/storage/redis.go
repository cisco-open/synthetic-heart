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

package storage

import (
	"context"
	"encoding/json"
	goerrors "errors"
	"fmt"
	"github.com/cisco-open/synthetic-heart/common"
	"github.com/cisco-open/synthetic-heart/common/proto"
	"github.com/hashicorp/go-hclog"
	"github.com/pkg/errors"
	"github.com/redis/go-redis/v9"
	"google.golang.org/protobuf/encoding/protojson"
	"k8s.io/client-go/util/retry"
	"time"
)

type RedisSynHeartStore struct {
	client                *redis.Client
	logger                hclog.Logger
	protoJsonMarshaller   protojson.MarshalOptions
	protoJsonUnMarshaller protojson.UnmarshalOptions
}

var ErrNotFound = errors.New("not found")

const (
	SynTestsBase           = "syntest-plugins"
	AllTestRunStatus       = SynTestsBase + "/all/testRunStatus" // ui needs this
	AllPluginStatus        = SynTestsBase + "/all/pluginStatus"
	PluginLatestHealthFmt  = SynTestsBase + "/%s/latestHealth"
	PluginLastUnhealthyFmt = SynTestsBase + "/%s/lastUnhealthy"
	TestRunLatestFmt       = SynTestsBase + "/%s/latestRun"
	TestRunLastFailedFmt   = SynTestsBase + "/%s/lastFailedRun"

	ConfigBase             = "configs"
	ConfigSynTestsSummary  = ConfigBase + "/syntests/summary"
	ConfigSynTestJsonFmt   = ConfigBase + "/syntest/%s/json"
	ConfigSynTestRawFmt    = ConfigBase + "/syntest/%s/raw"
	ConfigSynTestStatusFmt = ConfigBase + "/syntest/%s/status"

	AgentsAll = "agents/all"

	SynTestChannel = "syntests"
	ConfigChannel  = "config"
	AgentChannel   = "agent"
)

func NewRedisSynHeartStore(config SynHeartStoreConfig, log hclog.Logger) RedisSynHeartStore {
	r := RedisSynHeartStore{}
	r.client = redis.NewClient(&redis.Options{
		Addr:     config.Address,
		Password: "", // no password set
		DB:       0,  // use default DB
	})
	r.logger = log.Named("redis")
	r.protoJsonMarshaller = protojson.MarshalOptions{
		EmitUnpopulated: true,
	}
	r.protoJsonUnMarshaller = protojson.UnmarshalOptions{}
	return r
}

func (r *RedisSynHeartStore) Close() error {
	return r.client.Close()
}

func (r *RedisSynHeartStore) Ping(ctx context.Context) error {
	return r.client.Ping(ctx).Err()
}

func (r *RedisSynHeartStore) WriteTestRun(ctx context.Context, pluginId string, testRun proto.TestRun) error {
	r.logger.Info("publishing test result to redis")
	bytes, err := r.protoJsonMarshaller.Marshal(&testRun)
	if err != nil {
		err = errors.Wrap(err, "error marshalling test run")
		return err
	}

	testRunKey := fmt.Sprintf(TestRunLatestFmt, pluginId)
	err = r.SetR(ctx, testRunKey, string(bytes), 0)
	if err != nil {
		return errors.Wrap(err, "error writing test run")
	}

	passRatio := float64(testRun.TestResult.Marks) / float64(testRun.TestResult.MaxMarks)
	err = r.UpdateTestRunStatus(ctx, pluginId, passRatio)
	if err != nil {
		return err
	}

	// write last failed test run if the test run failed -- so if it passes, next time we have some way of knowing what failed
	if passRatio < 1 {
		lastFailedTestRunKey := fmt.Sprintf(TestRunLastFailedFmt, pluginId)
		err = r.SetR(ctx, lastFailedTestRunKey, string(bytes), 0)
		if err != nil {
			return errors.Wrap(err, "error writing test run")
		}
	}

	// This is to let subscribers know there is a new test run
	err = r.PublishR(ctx, SynTestChannel, "new run: "+pluginId)
	if err != nil {
		return errors.Wrap(err, "error publishing test run to channel")
	}
	r.logger.Info("successfully published test run to channel: " + SynTestChannel)
	return nil
}

func (r *RedisSynHeartStore) UpdateTestRunStatus(ctx context.Context, pluginId string, passRatio float64) error {
	err := r.HSetR(ctx, AllTestRunStatus, pluginId, fmt.Sprintf("%.5f", passRatio))
	if err != nil {
		return errors.Wrap(err, "error writing test run status")
	}
	return nil
}

func (r *RedisSynHeartStore) FetchAllTestConfigSummary(ctx context.Context) (map[string]common.SyntestConfigSummary, error) {
	synTestConfigSummaries, err := r.HGetAllR(ctx, ConfigSynTestsSummary)
	if err != nil {
		return map[string]common.SyntestConfigSummary{}, errors.Wrap(err, "error fetching all test config summaries")
	}
	summaries := map[string]common.SyntestConfigSummary{}
	for configId, jsonSummary := range synTestConfigSummaries {
		summary := common.SyntestConfigSummary{}
		err := json.Unmarshal([]byte(jsonSummary), &summary)
		if err != nil {
			return map[string]common.SyntestConfigSummary{}, errors.Wrap(err, "error unmarshalling config summary")
		}
		summaries[configId] = summary
	}
	return summaries, nil
}

func (r *RedisSynHeartStore) FetchAllTestRunStatus(ctx context.Context) (map[string]string, error) {
	synTestRunStatuses, err := r.HGetAllR(ctx, AllTestRunStatus)
	if err != nil {
		return map[string]string{}, err
	}
	return synTestRunStatuses, nil
}

func (r *RedisSynHeartStore) SubscribeToTestRunEvents(ctx context.Context, channelSize int, testChan chan<- string) error {
	pubsub := r.client.Subscribe(ctx, SynTestChannel)
	// Wait for confirmation that subscription is created before publishing anything.
	_, err := pubsub.Receive(ctx)
	if err != nil {
		return errors.Wrap(err, "error subscribing to channel "+SynTestChannel)
	}
	r.logger.Info("successfully subscribed to channel: " + SynTestChannel)
	for {
		select {
		case <-ctx.Done():
			r.logger.Info("kill signal received, stopping test run subscription")
			return nil
		case msg := <-pubsub.Channel(redis.WithChannelSize(channelSize)):
			testChan <- msg.Payload
		}
	}
}

func (r *RedisSynHeartStore) FetchTestConfig(ctx context.Context, testConfigId string) (proto.SynTestConfig, error) {
	msg, err := r.GetR(ctx, fmt.Sprintf(ConfigSynTestJsonFmt, testConfigId))
	if errors.Is(err, redis.Nil) {
		return proto.SynTestConfig{}, ErrNotFound
	} else if err != nil {
		return proto.SynTestConfig{}, errors.Wrap(err, "couldn't fetch latest config for:"+testConfigId)
	}
	config := proto.SynTestConfig{}
	err = r.protoJsonUnMarshaller.Unmarshal([]byte(msg), &config)
	if err != nil {
		return proto.SynTestConfig{}, errors.Wrap(err, "error un-marshalling config from redis")
	}

	return config, nil
}

func (r *RedisSynHeartStore) FetchLatestTestRun(ctx context.Context, pluginId string) (proto.TestRun, error) {
	msg, err := r.GetR(ctx, fmt.Sprintf(TestRunLatestFmt, pluginId))
	if errors.Is(err, redis.Nil) {
		return proto.TestRun{}, ErrNotFound
	} else if err != nil {
		return proto.TestRun{}, errors.Wrap(err, "couldn't fetch latest test run for:"+pluginId)
	}
	testRun := proto.TestRun{}
	err = r.protoJsonUnMarshaller.Unmarshal([]byte(msg), &testRun)
	if err != nil {
		return proto.TestRun{}, errors.Wrap(err, "error un-marshalling syntest from redis")
	}

	return testRun, nil
}

func (r *RedisSynHeartStore) FetchLastFailedTestRun(ctx context.Context, pluginId string) (proto.TestRun, error) {
	msg, err := r.GetR(ctx, fmt.Sprintf(TestRunLastFailedFmt, pluginId))
	if errors.Is(err, redis.Nil) {
		return proto.TestRun{}, ErrNotFound
	} else if err != nil {
		return proto.TestRun{}, errors.Wrap(err, "couldn't fetch last failed test run for:"+pluginId)
	}
	testRun := proto.TestRun{}
	err = r.protoJsonUnMarshaller.Unmarshal([]byte(msg), &testRun)
	if err != nil {
		return proto.TestRun{}, errors.Wrap(err, "error un-marshalling syntest from redis")
	}

	return testRun, nil
}

func (r *RedisSynHeartStore) DeleteAllTestRunInfo(ctx context.Context, pluginId string) error {
	err := r.HDelR(ctx, AllTestRunStatus, pluginId)
	if err != nil {
		r.logger.Warn(errors.Wrap(err, "couldn't delete testrun status from hset run for:"+pluginId).Error())
	}
	err = r.HDelR(ctx, AllPluginStatus, pluginId)
	if err != nil {
		r.logger.Warn(errors.Wrap(err, "couldn't delete plugin status from hset run for:"+pluginId).Error())
	}

	err = r.DelR(ctx, fmt.Sprintf(TestRunLatestFmt, pluginId))
	if err != nil {
		r.logger.Warn(errors.Wrap(err, "couldn't delete latest test run for:"+pluginId).Error())
	}
	err = r.DelR(ctx, fmt.Sprintf(TestRunLastFailedFmt, pluginId))
	if err != nil {
		r.logger.Warn(errors.Wrap(err, "couldn't delete last failed test run for:"+pluginId).Error())
	}

	err = r.DelR(ctx, fmt.Sprintf(PluginLatestHealthFmt, pluginId))
	if err != nil {
		r.logger.Warn(errors.Wrap(err, "couldn't delete plugin health status for:"+pluginId).Error())
	}
	err = r.DelR(ctx, fmt.Sprintf(PluginLastUnhealthyFmt, pluginId))
	if err != nil {
		r.logger.Warn(errors.Wrap(err, "couldn't delete plugin last unhealthy status health data for:"+pluginId).Error())
	}
	return nil
}

func (r *RedisSynHeartStore) SubscribeToConfigEvents(ctx context.Context, channelSize int, pluginName chan<- string) error {
	pubsub := r.client.Subscribe(ctx, ConfigChannel)
	// Wait for confirmation that subscription is created before publishing anything.
	_, err := pubsub.Receive(ctx)
	if err != nil {
		return errors.Wrap(err, "error subscribing to channel "+ConfigChannel)
	}
	r.logger.Info("successfully subscribed to channel: " + ConfigChannel)
	for {
		select {
		case <-ctx.Done():
			r.logger.Info("kill signal received, stopping config subscription")
			return nil
		case msg := <-pubsub.Channel(redis.WithChannelSize(channelSize)):
			pluginName <- msg.Payload
		}
	}
}

func (r *RedisSynHeartStore) WriteTestConfig(ctx context.Context, config proto.SynTestConfig, raw string) error {
	configId := common.ComputeSynTestConfigId(config.Name, config.Namespace)
	err := r.SetR(ctx, fmt.Sprintf(ConfigSynTestRawFmt, configId), raw, 0)
	if err != nil {
		return errors.Wrap(err, "error writing config"+", testName="+configId)
	}

	// write empty status
	err = r.WriteTestConfigStatus(ctx, configId, common.SyntestConfigStatus{
		Deployed: false,
		Message:  "",
		Agent:    "",
	})

	// update the summary
	b, err := r.protoJsonMarshaller.Marshal(&config)
	if err != nil {
		return err
	}
	err = r.SetR(ctx, fmt.Sprintf(ConfigSynTestJsonFmt, configId), string(b), 0)
	if err != nil {
		return errors.Wrap(err, "error writing config"+", testName="+configId)
	}
	summary := common.SyntestConfigSummary{
		ConfigId:    configId,
		Version:     config.Version,
		DisplayName: config.DisplayName,
		Description: config.Description,
		Namespace:   config.Namespace,
		Repeat:      config.Repeat,
		Plugin:      config.PluginName,
	}
	summaryJson, err := json.Marshal(summary)
	if err != nil {
		return errors.Wrap(err, "error marshalling config summary")
	}
	err = r.HSetR(ctx, ConfigSynTestsSummary, configId, string(summaryJson))
	if err != nil {
		return errors.Wrap(err, "error writing config summary to hashmap"+", testName="+configId)
	}
	err = r.PublishR(ctx, ConfigChannel, "update "+configId)
	if err != nil {
		return errors.Wrap(err, "error publishing to config channel"+", testName="+configId)
	}
	return nil
}

func (r *RedisSynHeartStore) DeleteTestConfig(ctx context.Context, configId string) error {
	err := r.DelR(ctx, fmt.Sprintf(ConfigSynTestRawFmt, configId))
	if err != nil {
		return errors.Wrap(err, "error deleting syntest raw config in ext-storage"+", testName="+configId)
	}
	err = r.DelR(ctx, fmt.Sprintf(ConfigSynTestJsonFmt, configId))
	if err != nil {
		return errors.Wrap(err, "error deleting syntest config in ext-storage"+", testName="+configId)
	}
	err = r.DelR(ctx, fmt.Sprintf(ConfigSynTestStatusFmt, configId))
	if err != nil {
		return errors.Wrap(err, "error deleting syntest config status in ext-storage"+", testName="+configId)
	}

	err = r.HDelR(ctx, ConfigSynTestsSummary, configId)
	if err != nil {
		return errors.Wrap(err, "error deleting syntest from 'summary' set in ext-storage"+", testName="+configId)
	}
	err = r.PublishR(ctx, ConfigChannel, "deleting "+configId)
	if err != nil {
		return errors.Wrap(err, "error publishing delete signal to config channel")
	}
	return nil
}

func (r *RedisSynHeartStore) WriteTestConfigStatus(ctx context.Context, configId string, status common.SyntestConfigStatus) error {
	statusJson, err := json.Marshal(status)
	if err != nil {
		return errors.Wrap(err, "error marshalling config status")
	}

	err = r.SetR(ctx, fmt.Sprintf(ConfigSynTestStatusFmt, configId), string(statusJson), 0)
	if err != nil {
		return errors.Wrap(err, "error writing config"+", testName="+configId)
	}
	return nil
}

func (r *RedisSynHeartStore) FetchTestConfigStatus(ctx context.Context, configId string) (common.SyntestConfigStatus, error) {
	statusRaw, err := r.GetR(ctx, fmt.Sprintf(ConfigSynTestStatusFmt, configId))
	if errors.Is(err, redis.Nil) {
		return common.SyntestConfigStatus{}, ErrNotFound
	} else if err != nil {
		return common.SyntestConfigStatus{}, errors.Wrap(err, "error fetching config status")
	}
	status := common.SyntestConfigStatus{}
	err = json.Unmarshal([]byte(statusRaw), &status)
	if err != nil {
		return common.SyntestConfigStatus{}, errors.Wrap(err, "error un-marshalling config status from redis")
	}
	return status, nil
}

func (r *RedisSynHeartStore) WritePluginHealthStatus(ctx context.Context, pluginId string, pluginState common.PluginState) error {
	healthKey := fmt.Sprintf(PluginLatestHealthFmt, pluginId)

	b, err := json.Marshal(pluginState)
	if err != nil {
		return errors.Wrap(err, "error marshalling plugin state json")
	}

	err = r.SetR(ctx, healthKey, string(b), 0)
	if err != nil {
		return errors.Wrap(err, "error writing health status to redis, plugin: "+pluginId)
	}

	if pluginState.Status == common.Error {
		badHealthKey := fmt.Sprintf(PluginLastUnhealthyFmt, pluginId)
		err = r.SetR(ctx, badHealthKey, string(b), 0)
		if err != nil {
			return errors.Wrap(err, "error writing last bad health status to redis, plugin: "+pluginId)
		}
	}

	err = r.UpdatePluginStatus(ctx, pluginId, string(pluginState.Status))
	if err != nil {
		return errors.Wrap(err, "error updating plugins status, plugin: "+pluginId)
	}

	return nil
}

func (r *RedisSynHeartStore) UpdatePluginStatus(ctx context.Context, pluginId string, status string) error {
	err := r.HSetR(ctx, AllPluginStatus, pluginId, status)
	if err != nil {
		return errors.Wrap(err, "error writing plugin status")
	}
	return nil
}

func (r *RedisSynHeartStore) FetchPluginHealthStatus(ctx context.Context, pluginId string) (common.PluginState, error) {
	healthKey := fmt.Sprintf(PluginLatestHealthFmt, pluginId)
	val, err := r.GetR(ctx, healthKey)
	if errors.Is(err, redis.Nil) {
		return common.PluginState{}, ErrNotFound
	} else if err != nil {
		return common.PluginState{}, errors.Wrap(err, "error reading health status frp, redis, plugin: "+pluginId)
	}
	state := common.PluginState{}
	err = json.Unmarshal([]byte(val), &state)
	if err != nil {
		return common.PluginState{}, errors.Wrap(err, "error unmarshalling plugin state json")
	}
	return state, nil
}

func (r *RedisSynHeartStore) FetchPluginLastUnhealthyStatus(ctx context.Context, pluginId string) (common.PluginState, error) {
	healthKey := fmt.Sprintf(PluginLastUnhealthyFmt, pluginId)
	val, err := r.GetR(ctx, healthKey)
	if errors.Is(err, redis.Nil) {
		return common.PluginState{}, ErrNotFound
	} else if err != nil {
		return common.PluginState{}, errors.Wrap(err, "error reading last unhealthy status frp, redis, plugin: "+pluginId)
	}
	state := common.PluginState{}
	err = json.Unmarshal([]byte(val), &state)
	if err != nil {
		return common.PluginState{}, errors.Wrap(err, "error unmarshalling plugin state json")
	}
	return state, nil
}

func (r *RedisSynHeartStore) FetchAllPluginStatus(ctx context.Context) (map[string]string, error) {
	pluginStatuses, err := r.HGetAllR(ctx, AllPluginStatus)
	if err != nil {
		return map[string]string{}, err
	}
	return pluginStatuses, nil
}

func (r *RedisSynHeartStore) FetchAllAgentStatus(ctx context.Context) (map[string]common.AgentStatus, error) {
	agents, err := r.HGetAllR(ctx, AgentsAll)
	if err != nil {
		return map[string]common.AgentStatus{}, err
	}

	allAgentStatus := map[string]common.AgentStatus{}

	for agentId, val := range agents {
		status := common.AgentStatus{}
		err := json.Unmarshal([]byte(val), &status)
		if err != nil {
			return map[string]common.AgentStatus{}, errors.Wrap(err, "error getting agent status")
		}
		allAgentStatus[agentId] = status
	}

	return allAgentStatus, nil
}

func (r *RedisSynHeartStore) WriteAgentStatus(ctx context.Context, agentId string, status common.AgentStatus) error {
	b, err := json.Marshal(status)
	if err != nil {
		return errors.Wrap(err, "error marshalling agent status")
	}
	err = r.HSetR(ctx, AgentsAll, agentId, string(b))
	if err != nil {
		return errors.Wrap(err, "error writing agent status to redis")
	}
	return nil
}

func (r *RedisSynHeartStore) DeleteAgentStatus(ctx context.Context, agentId string) error {
	err := r.HDelR(ctx, AgentsAll, agentId)
	if err != nil {
		return errors.Wrap(err, "error deleting agent from redis")
	}
	return nil
}

func (r *RedisSynHeartStore) SubscribeToAgentEvents(ctx context.Context, channelSize int, agentChan chan<- string) error {
	pubsub := r.client.Subscribe(ctx, AgentChannel)
	// Wait for confirmation that subscription is created before publishing anything.
	_, err := pubsub.Receive(ctx)
	if err != nil {
		return errors.Wrap(err, "error subscribing to channel "+AgentChannel)
	}
	r.logger.Info("successfully subscribed to channel: " + AgentChannel)
	for {
		select {
		case <-ctx.Done():
			r.logger.Info("kill signal received, stopping test run subscription")
			return nil
		case msg := <-pubsub.Channel(redis.WithChannelSize(channelSize)):
			agentChan <- msg.Payload
		}
	}
}

func (r *RedisSynHeartStore) NewAgentEvent(ctx context.Context, event string) error {
	err := r.PublishR(ctx, AgentChannel, event)
	if err != nil {
		return errors.Wrap(err, "error publishing 'agent' signal to config channel")
	}
	return nil
}

func (r *RedisSynHeartStore) GetR(ctx context.Context, key string) (string, error) {
	r.logger.Trace("redis cmd", "cmd", "get", "key", key)
	var val *string
	err := retry.OnError(common.DefaultBackoff, func(err error) bool {
		_, isRedisError := err.(redis.Error)
		isRedisNilError := errors.Is(err, redis.Nil)
		isCtxError := errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled)
		return err != nil && !isRedisError && !isCtxError && !isRedisNilError // retry if all these are true
	}, func() error {
		res, err := r.client.Get(ctx, key).Result()
		if err != nil {
			r.logger.Error("redis error, trying again...", "cmd", "get", "err", err)
			return err
		} else {
			val = &res
			return nil
		}
	})
	if val == nil { // sanity check so we dont dereference a nil pointer
		tmp := ""
		val = &tmp
	}
	return *val, err
}

// Publishes value to a channel
func (r *RedisSynHeartStore) PublishR(ctx context.Context, channel string, msg string) error {
	r.logger.Trace("redis cmd", "cmd", "publish", "channel", channel, "msg", msg)
	return retry.OnError(common.DefaultBackoff, func(err error) bool {
		_, isRedisError := err.(redis.Error)
		isCtxError := goerrors.Is(err, context.DeadlineExceeded) || goerrors.Is(err, context.Canceled)
		return err != nil && !isRedisError && !isCtxError
	}, func() error {
		err := r.client.Publish(ctx, channel, msg).Err()
		if err != nil {
			r.logger.Error("redis error, trying again...", "cmd", "publish", "err", err)
		}
		return err
	})
}

// Writes value to a key
func (r *RedisSynHeartStore) SetR(ctx context.Context, key string, val interface{}, expiration time.Duration) error {
	r.logger.Trace("redis cmd", "cmd", "set", "key", key, "val", val)
	return retry.OnError(common.DefaultBackoff, func(err error) bool {
		_, isRedisError := err.(redis.Error)
		isCtxError := goerrors.Is(err, context.DeadlineExceeded) || goerrors.Is(err, context.Canceled)
		return err != nil && !isRedisError && !isCtxError
	}, func() error {
		err := r.client.Set(ctx, key, val, expiration).Err()
		if err != nil {
			r.logger.Error("redis error, trying again...", "cmd", "set", "err", err)
		}
		return err
	})
}

// Deletes key and val
func (r *RedisSynHeartStore) DelR(ctx context.Context, key string) error {
	r.logger.Trace("redis cmd", "cmd", "del", "key", key)
	return retry.OnError(common.DefaultBackoff, func(err error) bool {
		_, isRedisError := err.(redis.Error)
		isCtxError := goerrors.Is(err, context.DeadlineExceeded) || goerrors.Is(err, context.Canceled)
		return err != nil && !isRedisError && !isCtxError
	}, func() error {
		err := r.client.Del(ctx, key).Err()
		if err != nil {
			r.logger.Error("redis error, trying again...", "cmd", "del", "err", err)
		}
		return err
	})
}

// Writes a hashset value to a key
func (r *RedisSynHeartStore) HSetR(ctx context.Context, key string, field string, val string) error {
	r.logger.Trace("redis cmd", "cmd", "hset", "key", key, "field", field, "val", val)
	return retry.OnError(common.DefaultBackoff, func(err error) bool {
		_, isRedisError := err.(redis.Error)
		isCtxError := goerrors.Is(err, context.DeadlineExceeded) || goerrors.Is(err, context.Canceled)
		return err != nil && !isRedisError && !isCtxError
	}, func() error {
		err := r.client.HSet(ctx, key, field, val).Err()
		if err != nil {
			r.logger.Error("redis error, trying again...", "cmd", "hset", "err", err)
		}
		return err
	})
}

// Deletes a hashset value to a key
func (r *RedisSynHeartStore) HDelR(ctx context.Context, key string, field string) error {
	r.logger.Trace("redis cmd", "cmd", "hdel", "key", key, "field", field)
	return retry.OnError(common.DefaultBackoff, func(err error) bool {
		_, isRedisError := err.(redis.Error)
		isCtxError := goerrors.Is(err, context.DeadlineExceeded) || goerrors.Is(err, context.Canceled)
		return err != nil && !isRedisError && !isCtxError
	}, func() error {
		err := r.client.HDel(ctx, key, field).Err()
		if err != nil {
			r.logger.Error("redis error, trying again...", "cmd", "hdel", "err", err)
		}
		return err
	})
}

// Fetches all fields and value from a hashset
func (r *RedisSynHeartStore) HGetAllR(ctx context.Context, key string) (map[string]string, error) {
	r.logger.Trace("redis cmd", "cmd", "hgetall", "key", key)
	var val *map[string]string
	err := retry.OnError(common.DefaultBackoff, func(err error) bool {
		_, isRedisError := err.(redis.Error)
		isCtxError := goerrors.Is(err, context.DeadlineExceeded) || goerrors.Is(err, context.Canceled)
		return err != nil && !isRedisError && !isCtxError
	}, func() error {
		res, err := r.client.HGetAll(ctx, key).Result()
		if err != nil {
			r.logger.Error("redis error, trying again...", "cmd", "hgetall", "err", err)
			return err
		} else {
			val = &res
			return nil
		}
	})
	if val == nil { // sanity check so we dont dereference a nil pointer
		tmp := map[string]string{}
		val = &tmp
	}
	return *val, err
}
