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
	"encoding/json"
	"fmt"
	"github.com/cisco-open/synthetic-heart/common"
	"github.com/cisco-open/synthetic-heart/common/proto"
	"github.com/cisco-open/synthetic-heart/common/storage"
	gmux "github.com/gorilla/mux"
	"github.com/hashicorp/go-hclog"
	"github.com/pkg/errors"
	"github.com/rs/cors"
	"io/ioutil"
	"log"
	"net/http"
	_ "net/http/pprof"
	"os"
	"strconv"
	"sync"
	"time"
)

const PingRefreshFrequency = 15 * time.Second

type RestApi struct {
	config        RestApiConfig
	srv           *http.Server
	store         storage.RedisSynHeartStore
	PingResponse  PingApiResponse
	pingRespMutex *sync.Mutex
	logger        hclog.Logger
}

type PingApiResponse struct {
	Message     string                    `json:"message"`
	LastUpdated string                    `json:"lastUpdated"`
	Details     string                    `json:"details"`
	Status      int                       `json:"status"`
	FailedTests map[string]FailedTestInfo `json:"failedTests"`
}

type FailedTestInfo struct {
	Name         string `json:"name"`
	Namespace    string `json:"namespace"`
	TestConfigId string `json:"testId"`
	DisplayName  string `json:"displayName"`
	Status       int    `json:"status"`
}

type RestApiConfig struct {
	Address        string `yaml:"address"`
	StorageAddress string `yaml:"storageAddress"`
	UIAddress      string `yaml:"uiAddress"`
	DebugMode      bool   `yaml:"debugMode"`
}

func NewRestApi(configPath string) (*RestApi, error) {

	r := RestApi{}
	pluginConfig := RestApiConfig{}
	r.pingRespMutex = &sync.Mutex{}

	r.logger = hclog.New(&hclog.LoggerOptions{
		Name:  "restapi",
		Level: hclog.LevelFromString(os.Getenv("LOG_LEVEL")),
	})

	b, err := ioutil.ReadFile(configPath)
	if err != nil {
		return &RestApi{}, errors.Wrap(err, "error reading file")
	}

	err = common.ParseYMLConfig(string(b), &pluginConfig)
	if err != nil {
		return &RestApi{}, errors.Wrap(err, "error parsing config")
	}
	r.config = pluginConfig

	router := gmux.NewRouter()

	// Setup HTTP response
	router.HandleFunc("/ui", r.RedirectToUi)
	router.HandleFunc("/api/v1/version", r.GetVersion)

	router.HandleFunc("/api/v1/ping", r.GetPing)
	router.HandleFunc("/api/v1/agents", r.GetAllAgents)
	router.HandleFunc("/api/v1/tests", r.GetAllTests)
	router.HandleFunc("/api/v1/tests/status", r.GetAllTestStatus)
	router.HandleFunc("/api/v1/tests/displayNames", r.GetAllTestNames)
	router.HandleFunc("/api/v1/tests/namespace", r.GetAllTestNamespaces)
	router.HandleFunc("/api/v1/test/{id:[a-zA-z0-9-]+\\/[a-zA-z0-9-]+\\/[a-zA-z0-9-]+\\/[a-zA-z0-9-]+}/latest", r.GetTest)
	router.HandleFunc("/api/v1/test/{id:[a-zA-z0-9-]+\\/[a-zA-z0-9-]+\\/[a-zA-z0-9-]+\\/[a-zA-z0-9-]+}/latest/logs", r.GetLatestTestLogs)

	if pluginConfig.DebugMode {
		router.PathPrefix("/debug/").Handler(http.DefaultServeMux)
	}
	handler := cors.Default().Handler(router)
	srv := &http.Server{Addr: r.config.Address, Handler: handler}
	r.srv = srv

	extStore := storage.NewRedisSynHeartStore(storage.SynHeartStoreConfig{
		Type:       "redis",
		BufferSize: 1000,
		Address:    r.config.StorageAddress,
	}, r.logger)
	r.store = extStore

	return &r, nil
}

func (r *RestApi) Finish() error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	err := r.srv.Shutdown(ctx)
	if err != nil {
		return err
	}
	return nil
}

func (r *RestApi) RedirectToUi(w http.ResponseWriter, req *http.Request) {
	r.PrintIPAndUserAgent(req)
	redirectUrl := r.config.UIAddress
	if redirectUrl == "" {
		r.logger.Warn("No UI address configured, redirecting to base url")
		redirectUrl = req.URL.Path // set it back to root path, if no ui address set
	}
	if req.URL.RawQuery != "" {
		redirectUrl = fmt.Sprintf("%s&%s", redirectUrl, req.URL.RawQuery)
	} else {
		redirectUrl = fmt.Sprintf("%s", redirectUrl)
	}
	http.Redirect(w, req, redirectUrl, http.StatusSeeOther)
	return
}

func (r *RestApi) GetVersion(w http.ResponseWriter, req *http.Request) {
	r.PrintIPAndUserAgent(req)
	w.Header().Set("Content-Type", "application/json")
	type Response struct {
		Version string `json:"version"`
	}
	resp := Response{
		Version: "2",
	}
	err := json.NewEncoder(w).Encode(resp)
	if err != nil {
		r.logger.Error("error encoding json", "err", err)
	}
}

func (r *RestApi) GetAllAgents(w http.ResponseWriter, req *http.Request) {
	r.PrintIPAndUserAgent(req)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	agents, err := r.store.FetchAllAgentStatus(ctx)
	if err != nil {
		r.logger.Error("error fetching agent statuses", "err", err)
		http.Error(w, "unable to fetch all tests", http.StatusInternalServerError)
	}
	w.Header().Set("Content-Type", "application/json")
	err = json.NewEncoder(w).Encode(agents)
	if err != nil {
		r.logger.Error("error encoding json", "err", err)
	}
}

func (r *RestApi) GetAllTests(w http.ResponseWriter, req *http.Request) {
	r.PrintIPAndUserAgent(req)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	syntests, err := r.store.FetchAllTestConfig(ctx)
	if err != nil {
		r.logger.Error("error fetching syntests", "err", err)
		http.Error(w, "unable to fetch all tests", http.StatusInternalServerError)
	}
	w.Header().Set("Content-Type", "application/json")
	err = json.NewEncoder(w).Encode(syntests)
	if err != nil {
		r.logger.Error("error encoding json", "err", err)
	}
}

func (r *RestApi) GetAllTestStatus(w http.ResponseWriter, req *http.Request) {
	r.PrintIPAndUserAgent(req)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	w.Header().Set("Content-Type", "application/json")
	status, err := r.store.FetchAllTestRunStatus(ctx)
	if err != nil {
		r.logger.Error("error fetching status from extStore", "err", err)
		http.Error(w, "error fetching status from extStore", http.StatusInternalServerError)

	}
	err = json.NewEncoder(w).Encode(status)
	if err != nil {
		r.logger.Error("error encoding json", "err", err)
	}
}

func (r *RestApi) GetTest(w http.ResponseWriter, req *http.Request) {
	r.PrintIPAndUserAgent(req)
	id, ok := gmux.Vars(req)["id"]
	if !ok {
		http.Error(w, "no test id provided", http.StatusUnprocessableEntity)
		return
	}
	w.Header().Set("Content-Type", "application/json")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Get Test
	test, err := r.store.GetR(ctx, fmt.Sprintf(storage.SynTestLatestTestRunFmt, id))
	if err != nil {
		r.logger.Error("error getting latest test run for syntest", "id", id, "err", err)
		test = "{}"
	}

	// Get Health
	health, err := r.store.GetR(ctx, fmt.Sprintf(storage.SynTestHealthFmt, id))
	if err != nil {
		r.logger.Error("error getting health for syntest", "id", id, "err", err)
		health = "{}"
	}

	// Get status
	statuses, err := r.store.HGetAllR(ctx, storage.SynTestAllTestRunStatus)
	status := "0"
	if err != nil {
		r.logger.Error("error getting status for syntest", "id", id, "err", err)
	} else {
		if s, ok := statuses[id]; ok {
			status = s
		}
	}

	// Get raw config (i.e. crd) for the test plugin
	testName, testNs, _, _, err := common.GetPluginIdComponents(id)
	raw, err := r.store.GetR(ctx, fmt.Sprintf(storage.ConfigSynTestRawFmt, common.ComputeSynTestConfigId(testName, testNs)))
	if err != nil {
		r.logger.Error("error getting raw config for syntest", "id", id, "err", err)
	}
	type Response struct {
		TestRun      json.RawMessage
		PluginHealth json.RawMessage
		Status       string
		RawConfig    string
	}
	err = json.NewEncoder(w).Encode(Response{
		TestRun:      json.RawMessage(test),
		PluginHealth: json.RawMessage(health),
		Status:       status,
		RawConfig:    raw,
	})
	if err != nil {
		r.logger.Error("error encoding json", "err", err)
	}
}

func (r *RestApi) GetLatestTestLogs(w http.ResponseWriter, req *http.Request) {
	r.PrintIPAndUserAgent(req)
	id, ok := gmux.Vars(req)["id"]
	if !ok {
		http.Error(w, "no test id provided", http.StatusUnprocessableEntity)
		return
	}
	w.Header().Set("Content-Type", "text/plain")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Get Test
	test, err := r.store.GetR(ctx, fmt.Sprintf(storage.SynTestLatestTestRunFmt, id))
	if err != nil {
		r.logger.Error("error getting latest test run for syntest", "id", id, "err", err)
		test = "{}"
	}

	testRun := proto.TestRun{}
	err = json.Unmarshal([]byte(test), &testRun)
	if err != nil {
		r.logger.Error("error decoding json", "err", err)
		http.Error(w, "error fetching logs", http.StatusInternalServerError)
		return
	}

	logs := testRun.Details[common.LogKey]
	w.Write([]byte(logs))
}

func (r *RestApi) GetAllTestNames(w http.ResponseWriter, req *http.Request) {
	r.PrintIPAndUserAgent(req)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	displayNames, err := r.store.HGetAllR(ctx, storage.ConfigSynTestsAllDisplayName)
	if err != nil {
		r.logger.Error("error fetching display Names from extStore", "err", err)
		http.Error(w, "error fetching  display Names from extStore", http.StatusInternalServerError)

	}
	w.Header().Set("Content-Type", "application/json")
	err = json.NewEncoder(w).Encode(displayNames)
	if err != nil {
		r.logger.Error("error encoding json", "err", err)
	}
}

func (r *RestApi) GetAllTestNamespaces(w http.ResponseWriter, req *http.Request) {
	r.PrintIPAndUserAgent(req)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	namespaces, err := r.store.HGetAllR(ctx, storage.ConfigSynTestsAllNamespace)
	if err != nil {
		r.logger.Error("error fetching namespaces from extStore", "err", err)
		http.Error(w, "error fetching  namespaces from extStore", http.StatusInternalServerError)

	}
	w.Header().Set("Content-Type", "application/json")
	err = json.NewEncoder(w).Encode(namespaces)
	if err != nil {
		r.logger.Error("error encoding json", "err", err)
	}
}

func (r *RestApi) GetPing(w http.ResponseWriter, req *http.Request) {
	r.PrintIPAndUserAgent(req)
	w.Header().Set("Content-Type", "application/json")
	r.pingRespMutex.Lock()
	defer r.pingRespMutex.Unlock()
	w.WriteHeader(http.StatusOK)
	err := json.NewEncoder(w).Encode(r.PingResponse)
	if err != nil {
		r.logger.Error("error encoding json", "err", err)
	}
}

func (r *RestApi) UpdatePingResponse(ctx context.Context, redisAddress string) {
	logger := r.logger.Named("ping-loop")
	storageClient := storage.NewRedisSynHeartStore(storage.SynHeartStoreConfig{
		Type:       "redis",
		BufferSize: 1000,
		Address:    redisAddress,
	}, logger)

	defer storageClient.Close()

	allStatus, err := storageClient.FetchAllTestRunStatus(ctx)
	if err != nil {
		logger.Error("error fetching status from extStore", "err", err)
		return
	}

	displayNames, err := storageClient.HGetAllR(ctx, storage.ConfigSynTestsAllDisplayName)
	if err != nil {
		logger.Error("error fetching status displayNames", "err", err)
		return
	}

	resp := PingApiResponse{
		Message:     "",
		LastUpdated: "",
		Details:     "",
		FailedTests: map[string]FailedTestInfo{},
		Status:      0,
	}

	maxFailedTestNames := 3
	failedTestNames := map[string]bool{}
	failedTests := map[string]FailedTestInfo{} // Used to construct the details string later
	overallStatus := 3

	for pluginId, status := range allStatus {
		statusInt, err := strconv.Atoi(status)
		if err != nil {
			logger.Error("error converting status to int", "err", err, "status", status)
			continue
		}
		testName, testNs, _, _, err := common.GetPluginIdComponents(pluginId)
		if err != nil {
			logger.Error("error decomposing plugin id", "err", err)
			continue
		}

		if statusInt < int(common.Passing) {
			if failedTestInfo, ok := failedTests[testName]; ok {
				failedTests[testName] = failedTestInfo
			} else {
				fti := FailedTestInfo{
					Name:         testName,
					Namespace:    testNs,
					TestConfigId: common.ComputeSynTestConfigId(testName, testNs),
					DisplayName:  displayNames[testName],
					Status:       statusInt,
				}
				failedTests[testName] = fti
			}

			if len(failedTestNames) < 3 {
				// add the name to list, so we can concatenate it and
				if failedTests[testName].DisplayName != "" {
					failedTestNames[failedTests[testName].DisplayName] = true
				} else {
					failedTestNames[testName] = true
				}

			}

			// set the overall status
			if statusInt < overallStatus {
				if statusInt != int(common.Unknown) {
					overallStatus = statusInt
				} else {
					overallStatus = int(common.Warning) // if the test status is unknown we set the overall status to warning
				}
			}
		}
	}

	failedTestNamesString := ""
	i := 0
	for testName, _ := range failedTestNames {
		if i == maxFailedTestNames || i == (len(failedTestNames)-1) { // Dont include comma in the last one
			failedTestNamesString += testName
		} else {
			failedTestNamesString += testName + ", "
		}
		i++
	}

	if len(failedTests) > 3 {
		resp.Details = fmt.Sprintf("%d failed tests: %s and %d more", len(failedTests), failedTestNamesString, len(failedTests)-3)
	} else if len(failedTests) == 0 {
		resp.Details = fmt.Sprintf("No failed tests")
	} else if len(failedTests) == 1 {
		resp.Details = fmt.Sprintf("%d failed test: %s", len(failedTestNames), failedTestNamesString)
	} else {
		resp.Details = fmt.Sprintf("%d failed tests: %s", len(failedTestNames), failedTestNamesString)
	}

	resp.Status = overallStatus
	resp.LastUpdated = time.Now().Format(common.TimeFormat)
	if resp.Status == 3 {
		resp.Message = "Healthy"
	} else {
		resp.Message = "UnHealthy"
	}
	resp.FailedTests = failedTests

	r.pingRespMutex.Lock()
	r.PingResponse = resp
	r.pingRespMutex.Unlock()
}

func (r *RestApi) PrintIPAndUserAgent(req *http.Request) {
	ip := req.Header.Get("X-Real-Ip")
	if ip == "" {
		ip = req.Header.Get("X-Forwarded-For")
	}
	if ip == "" {
		ip = req.RemoteAddr
	}
	userAgent := req.UserAgent()
	r.logger.Info("request", "path", req.URL.Path, "ip", ip, "user", userAgent)
}

func main() {
	configFilePath := ""
	if len(os.Args) > 1 { // Override if a path has been provided - used for debugging
		configFilePath = os.Args[1]
	} else {
		log.Fatal("no config file path provided")
	}
	restApi, err := NewRestApi(configFilePath)
	if err != nil {
		log.Fatal(err)
	}
	// Start the Ping Api polling/updating
	go func() {
		ticker := time.NewTicker(PingRefreshFrequency)
		for {
			select {
			case <-ticker.C:
				ctx, cancel := context.WithTimeout(context.Background(), PingRefreshFrequency)
				restApi.UpdatePingResponse(ctx, restApi.config.StorageAddress)
				cancel()
			}
		}
	}()

	// Start the Server
	log.Println("running server at: " + restApi.config.Address)
	err = restApi.srv.ListenAndServe()
	if err != nil && err != http.ErrServerClosed {
		log.Println("error running server: ", err)
		os.Exit(1)
	}
}
