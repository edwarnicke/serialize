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
	channelSize = 256 // 256 is chosen because 256*8 = 2kb, or about the cost of a go routine
)

// Executor - a struct that can be used to guarantee exclusive, in order execution of functions.
type Executor struct {
	orderCh     chan func()
	buffer      []func()
	bufferMutex sync.Mutex
	init        sync.Once
	count       int32
}

// AsyncExec - guarantees f() will be executed Exclusively and in the Order submitted.
//        It immediately returns a channel that will be closed when f() has completed execution.
func (e *Executor) AsyncExec(f func()) <-chan struct{} {
	e.init.Do(func() {
		e.orderCh = make(chan func(), channelSize)
	})
	// Start go routine if we don't have one
	if atomic.AddInt32(&e.count, 1) == 1 {
		result := make(chan struct{})
		go func() {
			f()
			close(result)
			if atomic.AddInt32(&e.count, -1) == 0 {
				return
			}
			for {
				e.bufferMutex.Lock()
				buf := e.buffer[0:]
				e.buffer = e.buffer[len(e.buffer):]
				e.bufferMutex.Unlock()
				for _, f := range buf {
					f()
				}
				if len(buf) > 0 && atomic.AddInt32(&e.count, -int32(len(buf))) == 0 {
					return
				}
			}
		}()
		return result
	}
	result := make(chan struct{})
	e.orderCh <- func() {
		f()
		close(result)
	}
	e.bufferMutex.Lock()
	e.buffer = append(e.buffer, <-e.orderCh)
	e.bufferMutex.Unlock()
	return result
}
