// Copyright 2026 Cisco Systems, Inc. and its affiliates
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
	"net/http"
	"net/http/httptest"
	"testing"

	gmux "github.com/gorilla/mux"
)

func TestRoutesAcceptDottedIDs(t *testing.T) {
	const (
		configID = "sip-ping-test-sip.wxsel-access1-mccdev-ocp.us-txwrtm1.dev.infra.webex.com-8934/wxc-edge"
		pluginID = configID + "/synheart-agent-xtww8/synthetic-heart"
	)

	tests := []struct {
		name   string
		path   string
		wantID string
	}{
		{name: "test config", path: "/api/v1/testconfig/" + configID, wantID: configID},
		{name: "plugin health", path: "/api/v1/plugin/" + pluginID + "/health", wantID: pluginID},
		{name: "plugin last unhealthy", path: "/api/v1/plugin/" + pluginID + "/lastUnhealthy", wantID: pluginID},
		{name: "latest test run", path: "/api/v1/testrun/" + pluginID + "/latest", wantID: pluginID},
		{name: "last failed test run", path: "/api/v1/testrun/" + pluginID + "/lastFailed", wantID: pluginID},
		{name: "latest logs", path: "/api/v1/testrun/" + pluginID + "/latest/logs", wantID: pluginID},
		{name: "last failed logs", path: "/api/v1/testrun/" + pluginID + "/lastFailed/logs", wantID: pluginID},
	}

	router := gmux.NewRouter()
	(&RestApi{}).registerRoutes(router)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, tt.path, nil)
			match := &gmux.RouteMatch{}
			if !router.Match(request, match) {
				t.Fatalf("route did not match %q", tt.path)
			}
			if got := match.Vars["id"]; got != tt.wantID {
				t.Fatalf("route id = %q, want %q", got, tt.wantID)
			}
		})
	}
}

func TestRoutesRejectExtraIDSegment(t *testing.T) {
	const pluginID = "test-name/test-namespace/agent-name/plugin-name"

	router := gmux.NewRouter()
	(&RestApi{}).registerRoutes(router)

	request := httptest.NewRequest(http.MethodGet, "/api/v1/testrun/"+pluginID+"/latest/logs/extra", nil)
	if router.Match(request, &gmux.RouteMatch{}) {
		t.Fatal("route matched an extra path segment")
	}
}
