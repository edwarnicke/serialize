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

// Executor - a struct that can be used to guarantee exclusive, in order execution of functions.
type Executor struct {
	sync.Map
	ticket uint64
}

// AsyncExec - guarantees f() will be executed Exclusively and in the Order submitted.
//        It immediately returns a channel that will be closed when f() has completed execution.
func (e *Executor) AsyncExec(f func()) <-chan struct{} {
	// Get a ticket - your ticket is your place in line
	ticket := atomic.AddUint64(&e.ticket, 1)
	// If this is the first ticket, start the go routine to execute
	if ticket == 1 {
		go func() {
			for {
				// Get the func() for the ticket we are processing
				g, ok := e.Map.Load(ticket)
				if !ok {
					// If we don't have a func() for the ticket, spin lock till we do
					continue
				}
				// Delete the func() from the Map
				e.Map.Delete(ticket)
				// Run the func()
				g.(func())()
				// If this is the last ticket, exit the go routine and reset the line by setting ticket to 0
				if atomic.CompareAndSwapUint64(&e.ticket, ticket, 0) {
					return
				}
				// process the next ticket
				ticket++
			}
		}()
	}
	// Add func() to its place in the map
	done := make(chan struct{})
	e.Map.Store(ticket, func() {
		f()
		close(done)
	})
	return done
}
