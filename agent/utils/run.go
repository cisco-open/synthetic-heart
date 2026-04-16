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

//go:build !windows

package utils

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"syscall"
	"time"
)

// RunWithContext runs a command with a context and makes sure to kill
// the subprocess and child processes when the command finishes
func RunWithContext(c *exec.Cmd, ctx context.Context) ([]byte, error) {
	c.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	c.Stdout = &stdout
	c.Stderr = &stderr

	err := c.Start()
	if err != nil {
		return nil, err
	}

	waitErrCh := make(chan error, 1)
	go func() {
		waitErrCh <- c.Wait()
	}()

	select {
	case waitErr := <-waitErrCh:
		if ee, ok := waitErr.(*exec.ExitError); ok {
			combinedError := fmt.Errorf("%w: %s", ee, stderr.String())
			return stdout.Bytes(), combinedError
		}
		return stdout.Bytes(), waitErr

	case <-ctx.Done():
		cancelErr := ctx.Err()
		pid := c.Process.Pid

		if killErr := syscall.Kill(-pid, syscall.SIGTERM); killErr != nil && !errors.Is(killErr, syscall.ESRCH) {
			return stdout.Bytes(), fmt.Errorf("%w: sending SIGTERM to process group: %v", cancelErr, killErr)
		}

		select {
		case <-waitErrCh:
			return stdout.Bytes(), cancelErr
		case <-time.After(3 * time.Second):
		}

		if killErr := syscall.Kill(-pid, syscall.SIGKILL); killErr != nil && !errors.Is(killErr, syscall.ESRCH) {
			return stdout.Bytes(), fmt.Errorf("%w: sending SIGKILL to process group: %v", cancelErr, killErr)
		}

		<-waitErrCh
		return stdout.Bytes(), cancelErr
	}
}
