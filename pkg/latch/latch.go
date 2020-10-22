// Copyright 2020 The Cockroach Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//   http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Package latch provides a simple counter latch.  This is conceptually
// similar to a sync.WaitGroup, except that it does not require
// foreknowledge of how many independent tasks there will be.
package latch

import "sync"

// Latch is a counter latch.
//
// A Latch should not be copied.
type Latch struct {
	cond  *sync.Cond
	count int
}

// New constructs a new Latch.
func New() *Latch {
	return &Latch{
		cond: sync.NewCond(&sync.Mutex{}),
	}
}

// Count returns the number of pending holds.
func (l *Latch) Count() int {
	l.cond.L.Lock()
	ret := l.count
	l.cond.L.Unlock()
	return ret
}

// Hold increments the use-count.
func (l *Latch) Hold() {
	l.cond.L.Lock()
	l.count++
	l.cond.L.Unlock()
}

// Release decrements the use-count.
func (l *Latch) Release() {
	l.cond.L.Lock()
	defer l.cond.L.Unlock()

	if l.count == 0 {
		panic("cannot release zero-count Latch")
	}
	l.count--
	if l.count == 0 {
		l.cond.Broadcast()
	}
}

// Wait pauses the caller until the use-count is zero.
// The return value will indicate if any delay was incurred.
func (l *Latch) Wait() bool {
	l.cond.L.Lock()
	defer l.cond.L.Unlock()

	if l.count == 0 {
		return false
	}

	for l.count > 0 {
		l.cond.Wait()
	}
	return true
}
