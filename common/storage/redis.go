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
	"k8s.io/client-go/util/retry"
	"strconv"
	"time"
)

type RedisSynHeartStore struct {
	client *redis.Client
	logger hclog.Logger
}

const (
	SynTestsBase              = "syntests"
	SynTestAllTestRunStatus   = SynTestsBase + "/all/status"
	SynTestAllTestRunLatestId = SynTestsBase + "/all/latestTestRuns"
	SynTestHealthFmt          = SynTestsBase + "/%s/health"
	SynTestLatestTestRunFmt   = SynTestsBase + "/%s/latest"

	ConfigBase                   = "configs"
	ConfigSynTestsAll            = ConfigBase + "/syntests/all"
	ConfigSynTestsAllDisplayName = ConfigBase + "/syntests/all/displayName" // needed by UI
	ConfigSynTestsAllNamespace   = ConfigBase + "/syntests/all/namespace"   // needed by UI
	ConfigSynTestJsonFmt         = ConfigBase + "/syntests/%s/json"
	ConfigSynTestRawFmt          = ConfigBase + "/syntests/%s/raw"

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
	bytes, err := json.Marshal(testRun)
	if err != nil {
		err = errors.Wrap(err, "error marshalling test run")
		return err
	}

	testRunKey := fmt.Sprintf(SynTestLatestTestRunFmt, pluginId)
	err = r.SetR(ctx, testRunKey, string(bytes), 0)
	if err != nil {
		return errors.Wrap(err, "error writing test run")
	}

	err = r.UpdateTestRunStatus(ctx, pluginId, common.GetTestRunStatus(testRun))
	if err != nil {
		return err
	}

	err = r.HSetR(ctx, SynTestAllTestRunLatestId, pluginId, testRun.Id)
	if err != nil {
		return errors.Wrap(err, "error writing latest test run Id")
	}

	// This is to let subscribers know there is a new test run
	err = r.PublishR(ctx, SynTestChannel, "new run: "+pluginId)
	if err != nil {
		return errors.Wrap(err, "error publishing test run to channel")
	}
	r.logger.Info("successfully published test run to channel: " + SynTestChannel)
	return nil
}

func (r *RedisSynHeartStore) UpdateTestRunStatus(ctx context.Context, pluginId string, status common.TestRunStatus) error {
	err := r.HSetR(ctx, SynTestAllTestRunStatus, pluginId, strconv.Itoa(int(status)))
	if err != nil {
		return errors.Wrap(err, "error writing test run status")
	}
	return nil
}

func (r *RedisSynHeartStore) FetchAllTestConfig(ctx context.Context) (map[string]string, error) {
	synTestConfigVersions, err := r.HGetAllR(ctx, ConfigSynTestsAll)
	if err != nil {
		return map[string]string{}, err
	}
	return synTestConfigVersions, nil
}

func (r *RedisSynHeartStore) FetchAllTestRunIds(ctx context.Context) (map[string]string, error) {
	synTestRunIds, err := r.HGetAllR(ctx, SynTestAllTestRunLatestId)
	if err != nil {
		return map[string]string{}, err
	}
	return synTestRunIds, nil
}

func (r *RedisSynHeartStore) FetchAllTestRunStatus(ctx context.Context) (map[string]string, error) {
	synTestRunIds, err := r.HGetAllR(ctx, SynTestAllTestRunStatus)
	if err != nil {
		return map[string]string{}, err
	}
	return synTestRunIds, nil
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

func (r *RedisSynHeartStore) FetchTestConfig(ctx context.Context, pluginName string) (proto.SynTestConfig, error) {
	msg, err := r.GetR(ctx, fmt.Sprintf(ConfigSynTestJsonFmt, pluginName))
	if err != nil {
		return proto.SynTestConfig{}, errors.Wrap(err, "couldn't fetch latest config for:"+pluginName)
	}
	config := proto.SynTestConfig{}
	err = json.Unmarshal([]byte(msg), &config)
	if err != nil {
		return proto.SynTestConfig{}, errors.Wrap(err, "error un-marshalling config from redis")
	}

	return config, nil
}

func (r *RedisSynHeartStore) FetchLatestTestRun(ctx context.Context, pluginId string) (proto.TestRun, error) {
	msg, err := r.GetR(ctx, fmt.Sprintf(SynTestLatestTestRunFmt, pluginId))
	if err != nil {
		return proto.TestRun{}, errors.Wrap(err, "couldn't fetch latest test run for:"+pluginId)
	}
	testRun := proto.TestRun{}
	err = json.Unmarshal([]byte(msg), &testRun)
	if err != nil {
		return proto.TestRun{}, errors.Wrap(err, "error un-marshalling syntest from redis")
	}

	return testRun, nil
}

func (r *RedisSynHeartStore) DeleteAllTestRunInfo(ctx context.Context, pluginId string) error {
	err := r.HDelR(ctx, SynTestAllTestRunLatestId, pluginId)
	if err != nil {
		return errors.Wrap(err, "couldn't delete latest test run from hset for:"+pluginId)
	}

	err = r.HDelR(ctx, SynTestAllTestRunStatus, pluginId)
	if err != nil {
		return errors.Wrap(err, "couldn't delete status from hset run for:"+pluginId)
	}

	err = r.DelR(ctx, fmt.Sprintf(SynTestLatestTestRunFmt, pluginId))
	if err != nil {
		return errors.Wrap(err, "couldn't delete latest test run for:"+pluginId)
	}
	err = r.DelR(ctx, fmt.Sprintf(SynTestHealthFmt, pluginId))
	if err != nil {
		return errors.Wrap(err, "couldn't delete plugin health data for:"+pluginId)
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

func (r *RedisSynHeartStore) WriteTestConfig(ctx context.Context, version string, config proto.SynTestConfig, raw string) error {
	err := r.SetR(ctx, fmt.Sprintf(ConfigSynTestRawFmt, config.Name), raw, 0)
	if err != nil {
		return errors.Wrap(err, "error writing config"+", testName="+config.Name)
	}
	b, err := json.Marshal(config)
	if err != nil {
		return err
	}
	err = r.SetR(ctx, fmt.Sprintf(ConfigSynTestJsonFmt, config.Name), string(b), 0)
	if err != nil {
		return errors.Wrap(err, "error writing config"+", testName="+config.Name)
	}
	err = r.HSetR(ctx, ConfigSynTestsAll, config.Name, version)
	if err != nil {
		return errors.Wrap(err, "error writing config version to hashmap"+", testName="+config.Name)
	}
	err = r.HSetR(ctx, ConfigSynTestsAllDisplayName, config.Name, config.DisplayName) // Needed by UI
	if err != nil {
		return errors.Wrap(err, "error writing config display name to hashmap"+", testName="+config.Name)
	}
	err = r.HSetR(ctx, ConfigSynTestsAllNamespace, config.Name, config.Namespace) // Needed by UI
	if err != nil {
		return errors.Wrap(err, "error writing config namespace to hashmap"+", testName="+config.Name)
	}
	err = r.PublishR(ctx, ConfigChannel, "update "+config.Name)
	if err != nil {
		return errors.Wrap(err, "error publishing to config channel"+", testName="+config.Name)
	}
	return nil
}

func (r *RedisSynHeartStore) DeleteTestConfig(ctx context.Context, name string) error {
	err := r.DelR(ctx, fmt.Sprintf(ConfigSynTestRawFmt, name))
	if err != nil {
		return errors.Wrap(err, "error deleting syntest raw config in ext-storage"+", testName="+name)
	}
	err = r.DelR(ctx, fmt.Sprintf(ConfigSynTestJsonFmt, name))
	if err != nil {
		return errors.Wrap(err, "error deleting syntest config in ext-storage"+", testName="+name)
	}
	err = r.HDelR(ctx, ConfigSynTestsAll, name)
	if err != nil {
		return errors.Wrap(err, "error deleting syntest from 'all' set in ext-storage"+", testName="+name)
	}
	err = r.HDelR(ctx, ConfigSynTestsAllDisplayName, name) // Needed by UI
	if err != nil {
		return errors.Wrap(err, "error deleting syntest from 'allDisplayNames' set in ext-storage"+", testName="+name)
	}
	err = r.HDelR(ctx, ConfigSynTestsAllNamespace, name) // Needed by UI
	if err != nil {
		return errors.Wrap(err, "error deleting syntest from 'namespace' set in ext-storage"+", testName="+name)
	}
	err = r.PublishR(ctx, ConfigChannel, "deleting "+name)
	if err != nil {
		return errors.Wrap(err, "error publishing delete signal to config channel")
	}
	return nil
}

func (r *RedisSynHeartStore) WritePluginStatus(ctx context.Context, pluginId string, pluginState common.PluginState) error {
	healthKey := fmt.Sprintf(SynTestHealthFmt, pluginId)

	b, err := json.Marshal(pluginState)
	if err != nil {
		return errors.Wrap(err, "error marshalling plugin state json")
	}

	err = r.SetR(ctx, healthKey, string(b), 0)
	if err != nil {
		return errors.Wrap(err, "error writing health status to redis, plugin: "+pluginId)
	}
	return nil
}

func (r *RedisSynHeartStore) FetchPluginStatus(ctx context.Context, pluginId string) (common.PluginState, error) {
	healthKey := fmt.Sprintf(SynTestHealthFmt, pluginId)
	val, err := r.GetR(ctx, healthKey)
	if err != nil {
		return common.PluginState{}, errors.Wrap(err, "error reading health status frp, redis, plugin: "+pluginId)
	}

	state := common.PluginState{}
	err = json.Unmarshal([]byte(val), &state)
	if err != nil {
		return common.PluginState{}, errors.Wrap(err, "error unmarshalling plugin state json")
	}
	return state, nil
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

// Fetches a value
func (r *RedisSynHeartStore) GetR(ctx context.Context, key string) (string, error) {
	r.logger.Trace("redis cmd", "cmd", "get", "key", key)
	var val *string
	err := retry.OnError(common.DefaultBackoff, func(err error) bool {
		_, isRedisError := err.(redis.Error)
		isCtxError := goerrors.Is(err, context.DeadlineExceeded) || goerrors.Is(err, context.Canceled)
		return err != nil && !isRedisError && !isCtxError
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
