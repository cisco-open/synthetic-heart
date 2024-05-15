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

//go:build !race
// +build !race

package pluginmanager

const (
	tConfigpath     = "../test-configs/"
	tConfigfiletype = "yaml"
)

// Dummy test plugin names (for testing purposes)
const (
	DummySimpleTestName = "dummySynTest"
)

func injectDummyPlugin() {
	RegisterSynTestPlugin(DummySimpleTestName, []string{"../build/bin/test-dummySynTest"})
}

//func TestCtxCancelDeadline(t *testing.T) {
//	injectDummyPlugin()
//	configFileName := "ctx-cancel"
//	v, err := config.Initialise(configFileName, tConfigfiletype, []string{tConfigpath})
//	if err != nil {
//		t.Error(err)
//		return
//	}
//
//	err = os.Setenv("NODE_NAME", "test_node")
//	if err != nil {
//		t.Error("couldn't set env var : NODE_NAME")
//		return
//	}
//
//	pm, err := NewPluginManager(v)
//	if err != nil {
//		t.Error(err)
//		return
//	}
//	ctx, cancel := context.WithCancel(context.Background())
//	done := make(chan struct{}, 1)
//
//	go func() {
//		err := pm.Start(ctx) // should block until ctx.cancel is called
//		if err != nil {
//			t.Error(err)
//		}
//		done <- struct{}{}
//		close(done)
//	}()
//
//	timeout := (v.GetDuration("timeouts.tests.finish") + v.GetDuration("timeouts.tests.init")) * time.Second
//
//	log.Println("sending cancellation")
//	cancel()
//
//	select {
//	case <-done:
//		break
//
//	case <-time.After(timeout):
//		t.Error("context cancelled, but plugin manager didn't exit within " + timeout.String())
//
//	}
//}
//
//func TestSynTestGoRoutineLeakage(t *testing.T) {
//	injectDummyPlugin()
//	configFileName := "syntest-routine-leakage"
//	v, err := config.Initialise(configFileName, tConfigfiletype, []string{tConfigpath})
//	if err != nil {
//		t.Error(err)
//		return
//	}
//
//	err = os.Setenv("NODE_NAME", "test_node")
//	if err != nil {
//		t.Error("couldn't set env var : NODE_NAME")
//		return
//	}
//
//	pm, err := NewPluginManager(v)
//	if err != nil {
//		t.Error(err)
//		return
//	}
//	ctx, cancel := context.WithCancel(context.Background())
//	done := make(chan struct{}, 1)
//
//	go func() {
//		numRoutineStart := runtime.NumGoroutine()
//		err := pm.Start(ctx) // should block until ctx.cancel is called
//		if err != nil {
//			t.Error(err)
//		}
//		numRoutineEnd := runtime.NumGoroutine()
//
//		if numRoutineStart < numRoutineEnd {
//			t.Error("go routine leakage!", " start:", numRoutineStart, "end:", numRoutineEnd)
//		}
//		done <- struct{}{}
//		close(done)
//	}()
//
//	<-time.After(4 * time.Second) // Allow plugins to setup
//	cancel()
//	<-done
//}
//
//func TestSynTestPluginPanic(t *testing.T) {
//	injectDummyPlugin()
//	configFileName := "syntest-plugin-panic"
//	v, err := config.Initialise(configFileName, tConfigfiletype, []string{tConfigpath})
//	if err != nil {
//		t.Error(err)
//		return
//	}
//
//	err = os.Setenv("NODE_NAME", "test_node")
//	if err != nil {
//		t.Error("couldn't set env var : NODE_NAME")
//		return
//	}
//
//	pm, err := NewPluginManager(v)
//	if err != nil {
//		t.Error(err)
//		return
//	}
//	ctx, cancel := context.WithCancel(context.Background())
//	done := make(chan struct{}, 1)
//
//	go func() {
//		err := pm.Start(ctx) // should block until ctx.cancel is called
//		if err != nil {
//			t.Error(err)
//		}
//		done <- struct{}{}
//	}()
//	<-time.After(4 * time.Second) // Allow plugins to setup
//	cancel()
//	<-done
//}

//func TestSynTestPluginTestPass(t *testing.T) {
//	injectDummyPlugin(common.DummySimpleTestName)
//	configFileName := "syntest-plugin-panic"
//	v, err := config.Initialise(configFileName, tConfigfiletype, []string{tConfigpath})
//	if err != nil {
//		t.Error(err)
//		return
//	}
//	pm := PluginManager{
//		AgentConfig:                  v,
//		ResultsPluginPathPrefix: tResultspluginpathprefix,
//		TestsPluginPathPrefix:   tTestspluginpathprefix,
//	}
//	ctx, cancel := context.WithCancel(context.Background())
//	done := make(chan struct{}, 1)
//	go func() {
//		pm.Run(ctx) // should block until ctx.cancel is called
//		done <- struct{}{}
//	}()
//
//	select {
//	case res := <-resCh:
//		if res.TestResult.Marks != res.TestResult.MaxMarks {
//			t.Error("test returned failure")
//		}
//
//	case <-time.After(10 * time.Second):
//		t.Error("no results within timeout")
//	}
//	cancel()
//	<-done
//}
//
//func TestSyntestPluginTestTimeout(t *testing.T) {
//
//}
//
//func TestSyntestPluginFinishTimeout(t *testing.T) {
//
//}
