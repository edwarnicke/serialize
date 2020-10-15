// Copyright (c) 2020 Cisco Systems, Inc.
//
// SPDX-License-Identifier: Apache-2.0
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at:
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Package serialize provides a simple means for Async or Sync execution of a func()
// with the guarantee that each func() will be executed exactly once and that all funcs()
// will be executed in order
package serialize

import (
	"sync"
	"sync/atomic"
)

const (
	channelSize = 256 // 256 is chosen because 256*8 = 2kb, or about the cost of a go routing
)

type job struct {
	f    func()
	done chan struct{}
}

// Executor - a struct that can be used to guarantee exclusive, in order execution of functions.
type Executor struct {
	execCh    chan *job
	init      sync.Once
	count     int32
	jobs      []*job
	runningCh chan struct{}
	idle      bool
}

// AsyncExec - guarantees f() will be executed Exclusively and in the Order submitted.
//        It immediately returns a channel that will be closed when f() has completed execution.
func (e *Executor) AsyncExec(f func()) <-chan struct{} {
	// Initialize *once*
	e.init.Do(func() {
		e.execCh = make(chan *job, channelSize)
	})
	exec := &job{
		f:    f,
		done: make(chan struct{}),
	}
	// Start go routine if we don't have one
	if atomic.AddInt32(&e.count, 1) == 1 {
		go e.process(exec)
		return exec.done
	}

	e.execCh <- exec
	return exec.done
}

func (e *Executor) process(exec *job) {
	// Selects are expensive.  If we only need to execute one thing,
	// do it here and be done with it.
	e.jobs = []*job{exec}
	if e.executeJobs() {
		return
	}
	for {
		select {
		case exec := <-e.execCh:
			e.jobs = append(e.jobs, exec)
			// Selects are expensive.  If our e.runningCh is just a placeholder
			// so we don't busy wait on incoming executables (ie e.idle == true)
			// then execute here
			if e.idle {
				if e.executeJobs() {
					return
				}
			}

		case <-e.runningCh:
			// If we can run again, but we have nothing *to* run, note we are e.idle, setup
			// a placeholder e.runningCh to keep us from hitting this case again until we have work
			// to do, and break out of this case
			if len(e.jobs) == 0 {
				e.idle = true
				e.runningCh = make(chan struct{})
				break
			}
			// If we have jobs to execute, execute them
			if e.executeJobs() {
				return
			}
		}
	}
}

func (e *Executor) executeJobs() bool {
	// Execute jobs
	go func(backlog []*job) {
		for _, exec := range backlog {
			exec.f()
			close(exec.done)
		}
	}(e.jobs)

	// e.runningCh is busy till the last job is complete
	e.runningCh = e.jobs[len(e.jobs)-1].done
	jobsCompleted := -int32(len(e.jobs))

	// e.jobs is now empty, and can start accumulating more jobs to do
	e.jobs = nil
	// We are definitely not idle
	e.idle = false
	return atomic.AddInt32(&e.count, jobsCompleted) == 0
}
