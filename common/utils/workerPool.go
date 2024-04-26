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

package utils

import (
	"bytes"
	"context"
	"log"
)

type WorkerPool struct {
	workers    int
	jobChan    chan interface{}
	workFunc   func(context.Context, *log.Logger, interface{}) (interface{}, error)
	ResultChan chan JobResult
	debug      bool
}

type JobResult struct {
	Logs         string
	Error        error
	Job          interface{}
	ReturnValues interface{}
}

func NewWorkerPool(workers int, jobBuffer int,
	work func(context.Context, *log.Logger, interface{}) (interface{}, error), debug bool) WorkerPool {

	wp := WorkerPool{
		workers:    workers,
		jobChan:    make(chan interface{}, jobBuffer),
		ResultChan: make(chan JobResult, jobBuffer),
		workFunc:   work,
		debug:      debug,
	}

	return wp
}

func (wp *WorkerPool) AddJob(job interface{}) {
	wp.jobChan <- job
}

func (wp *WorkerPool) Start(ctx context.Context) {
	for i := 0; i < wp.workers; i++ {
		go func(id int, jobs <-chan interface{}, results chan<- JobResult) {
			for {
				select {
				case job := <-jobs:
					if job == nil {
						return
					}
					var buf bytes.Buffer
					logger := log.Logger{}
					logger.SetFlags(log.Flags())
					logger.SetOutput(&buf)
					if wp.debug {
						log.Printf("worker %d started job: %v", id, job)
					}
					val, err := wp.workFunc(ctx, &logger, job)
					if wp.debug {
						log.Printf("worker %d finished job: %v", id, job)
					}
					result := JobResult{
						Logs:         buf.String(),
						Error:        err,
						ReturnValues: val,
						Job:          job,
					}
					results <- result
				case <-ctx.Done():
					return
				}
			}
		}(i, wp.jobChan, wp.ResultChan)
	}
}

func (wp *WorkerPool) Stop() {
	close(wp.jobChan)
}
