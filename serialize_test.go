// Copyright (c) 2020 Cisco and/or its affiliates.
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

// Package serialize_test tests the contracts of the serialize.Executor which are:
//  1.  One at a time - the executor will never execute more than one func()
//                         provided to it at a time
//  2.  In Order - the order of execution of func()s provided to it will always be preserved: first in, first executed
package serialize_test

import (
	"testing"

	"go.uber.org/goleak"

	"github.com/edwarnicke/serialize"
)

func TestASyncExec(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())
	exec := serialize.Executor{}
	count := 0
	completion0 := make(chan struct{})
	exec.AsyncExec(func() {
		if count != 0 {
			t.Errorf("expected count == 0, actual %d", count)
		}
		count = 1
		close(completion0)
	})
	select {
	case <-completion0:
		t.Error("exec.AsyncExec did run to completion before returning.")
	default:
	}
}

func TestSyncExec(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())
	var exec serialize.Executor
	count := 0
	completion0 := make(chan struct{})
	<-exec.AsyncExec(func() {
		if count != 0 {
			t.Errorf("expected count == 0, actual %d", count)
		}
		count = 1
		close(completion0)
	})
	select {
	case <-completion0:
	default:
		t.Error("exec.SyncExec did not run to completion before returning.")
	}
}

func TestExecOrder1(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())
	var exec serialize.Executor
	count := 0
	trigger0 := make(chan struct{})
	completion0 := make(chan struct{})
	exec.AsyncExec(func() {
		<-trigger0
		if count != 0 {
			t.Errorf("expected count == 0, actual %d", count)
		}
		count = 1
		close(completion0)
	})
	trigger1 := make(chan struct{})
	completion1 := make(chan struct{})
	exec.AsyncExec(func() {
		<-trigger1
		if count != 1 {
			t.Errorf("expected count == 1, actual %d", count)
		}
		count = 2
		close(completion1)
	})
	// Let the second one start first
	close(trigger1)
	close(trigger0)
	<-completion0
	<-completion1
}

// Same as TestExecOrder1 but making sure out of order fails as expected
func TestExecOrder2(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())
	var exec serialize.Executor
	count := 0

	trigger1 := make(chan struct{})
	completion1 := make(chan struct{})
	exec.AsyncExec(func() {
		<-trigger1
		if !(count != 1) {
			t.Errorf("expected count != 1, actual %d", count)
		}
		count = 2
		close(completion1)
	})

	trigger0 := make(chan struct{})
	completion0 := make(chan struct{})
	exec.AsyncExec(func() {
		<-trigger0
		if !(count != 0) {
			t.Errorf("expected count != 0, actual %d", count)
		}
		count = 1
		close(completion0)
	})
	// Let the second one start first
	close(trigger1)
	close(trigger0)
	<-completion0
	<-completion1
}

func TestExecOneAtATime(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())
	var exec serialize.Executor
	start := make(chan struct{})
	count := 100000
	finished := make([]chan struct{}, count)
	var running int
	for i := 0; i < count; i++ {
		finished[i] = make(chan struct{})
		end := finished[i]
		exec.AsyncExec(func() {
			<-start
			if running != 0 {
				t.Errorf("expected running == 0, actual %d", count)
			}
			running++
			if running != 1 {
				t.Errorf("expected running == 1, actual %d", count)
			}
			running--
			close(end)
		})
	}
	close(start)
	for _, done := range finished {
		<-done
	}
}

func TestNestedAsyncDeadlock(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())
	var exec serialize.Executor
	startCh := make(chan struct{})
	exec.AsyncExec(func() {
		<-startCh
		exec.AsyncExec(func() {
			<-startCh
			exec.AsyncExec(func() {
				<-startCh
			})
		})
	})
	for i := 0; i < 1000; i++ {
		exec.AsyncExec(func() {})
	}
	close(startCh)
	<-exec.AsyncExec(func() {})
}

func BenchmarkExecutorAsync(b *testing.B) {
	defer goleak.VerifyNone(b, goleak.IgnoreCurrent())
	var exec serialize.Executor
	for i := 0; i < b.N-1; i++ {
		exec.AsyncExec(func() {})
	}
	<-exec.AsyncExec(func() {})
}

func BenchmarkExecutorSync(b *testing.B) {
	defer goleak.VerifyNone(b, goleak.IgnoreCurrent())
	var exec serialize.Executor
	for i := 0; i < b.N; i++ {
		<-exec.AsyncExec(func() {})
	}
}
