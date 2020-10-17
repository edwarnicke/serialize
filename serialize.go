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
	"runtime"
	"sync/atomic"
)

type job struct {
	f    func()
	done chan struct{}
	next atomic.Value
}

// Executor - a struct that can be used to guarantee exclusive, in order execution of functions.
type Executor struct {
	ticket uintptr
	queued uintptr
	head   *job
	tail   atomic.Value
}

// AsyncExec - guarantees f() will be executed Exclusively and in the Order submitted.
//        It immediately returns a channel that will be closed when f() has completed execution.
func (e *Executor) AsyncExec(f func()) <-chan struct{} {
	ticket := atomic.AddUintptr(&e.ticket, 1)
	if ticket == 0 {
		panic("AsyncExec - ticket == 0 indicates we have deadlocked by wrapping the uintptr all the way back around to 0")
	}

	newJob := &job{
		f:    f,
		done: make(chan struct{}),
	}

	// Wait for our turn to queue our job
	for atomic.LoadUintptr(&e.queued) != ticket-1 {
		runtime.Gosched()
	}
	tail, _ := e.tail.Load().(*job)
	if tail != nil {
		tail.next.Store(newJob)
	}
	e.tail.Store(newJob)

	// Let the next invoker have their turn
	if atomic.AddUintptr(&e.queued, 1) != ticket {
		panic("e.queued does not match ticket on equeue")
	}

	if ticket == 1 {
		go e.process(ticket, newJob)
	}
	return newJob.done
}

func (e *Executor) process(ticket uintptr, newJob *job) {
	// Note, as e.head is *only* ever accessed by the processing job
	// and there is *only* ever one processing job at time, no need
	// to wrap access in atomics
	e.head = newJob
	for {
		if ticket = e.execute(ticket, e.head); ticket == 0 {
			return
		}
	}
}

func (e *Executor) execute(ticket uintptr, j *job) uintptr {
	// Run the job
	j.f()
	close(j.done)
	// If this is the last ticket issued, reset the ticket counter
	if atomic.CompareAndSwapUintptr(&e.ticket, ticket, 0) {
		// Reset the queued counter too.
		atomic.CompareAndSwapUintptr(&e.queued, ticket, 0)
		return 0
	}

	// Spinlock on the queued counter catching up to the ticket
	// to insure that j.next is non-nil
	for atomic.LoadUintptr(&e.queued) <= ticket {
		runtime.Gosched()
	}
	e.head, _ = j.next.Load().(*job)
	return ticket + 1
}
