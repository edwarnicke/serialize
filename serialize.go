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
	"sort"
	"sync"
	"sync/atomic"
)

const (
	channelSize = 256 // 256 is chosen because 256*8 = 2kb, or about the cost of a go routine
)

type job struct {
	f func()
	done chan struct{}
	ticket uintptr
}

// Executor - a struct that can be used to guarantee exclusive, in order execution of functions.
type Executor struct {
	buffer []*job
	mu     sync.Mutex
	ticket uintptr
	sorted int
}

// AsyncExec - guarantees f() will be executed Exclusively and in the Order submitted.
//        It immediately returns a channel that will be closed when f() has completed execution.
func (e *Executor) AsyncExec(f func()) <-chan struct{} {
	ticket := atomic.AddUintptr(&e.ticket,1)
	if ticket == 0 {
		panic("ticket == 0 - you've overrun a uintptr counter of jobs and are probably deadlocked")
	}
	jb := &job{
		f: f,
		done: make(chan struct{}),
		ticket: ticket,
	}
	if ticket == 1 {
		go e.process()
	}
	e.mu.Lock()
	e.buffer = append(e.buffer, jb)
	e.mu.Unlock()
	return jb.done
}

func (e * Executor) process() {
	ticket := uintptr(1)
	var buf []*job
	for {
		e.mu.Lock()
		buf = append(buf,e.buffer[0:]...)
		e.buffer = e.buffer[len(e.buffer):]
		e.mu.Unlock()
		sort.SliceStable(buf, func(i, j int) bool {
			return buf[i].ticket < buf[j].ticket
		})
		for len(buf) > 0 {
			jb := buf[0]
			if jb.ticket == ticket {
				jb.f()
				close(jb.done)
				if atomic.CompareAndSwapUintptr(&e.ticket,ticket,0) {
					return
				}
				buf = buf[1:]
				ticket++
				continue
			}
			break
		}
	}
}

