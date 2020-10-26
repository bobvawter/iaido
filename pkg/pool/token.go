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

package pool

import (
	"runtime"
	"sync"
)

// A Token represents the intent to use an Entry in the Pool. Tokens
// should be released by callers in order to avoid delaying the return
// of an entry to a pool.
type Token struct {
	mu struct {
		sync.Mutex
		entry *entryMeta
		pool  *Pool
	}
	release int
}

func tokenFinalizer(t *Token) {
	t.doRelease(false)
}

// newToken constructs a Token for the pool and entry.  It also sets up
// a finalizer to automatically release the token should a caller fail
// to do so. This finalizer will be cleared once the token is released
// to avoid unnecessary work by the GC.
func newToken(p *Pool, load int, entry *entryMeta) *Token {
	t := &Token{release: -load}
	t.mu.entry = entry
	if load > 0 {
		t.mu.pool = p
		runtime.SetFinalizer(t, tokenFinalizer)
	}
	return t
}

// Entry retrieves the Entry that is tracked by the token.  This method
// may be called as many times as desired until Release is called.  Once
// the Token has been released, this method will return nil.
func (t *Token) Entry() Entry {
	t.mu.Lock()
	ret := t.mu.entry
	t.mu.Unlock()
	if ret == nil {
		return nil
	}
	return ret.Entry
}

// Release will return the associated Entry to the Pool.
//
// Release will return true the first time it is called and then false
// thereafter.
func (t *Token) Release() bool {
	return t.doRelease(true)
}

func (t *Token) doRelease(clearFinalizer bool) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	e := t.mu.entry
	if e == nil {
		return false
	}
	t.mu.entry = nil

	if clearFinalizer {
		runtime.SetFinalizer(t, nil)
	}

	// pool will be nil for 0-load tokens
	if t.mu.pool == nil {
		return true
	}

	t.mu.pool.mu.Lock()
	defer t.mu.pool.mu.Unlock()

	// If the index is -1, then the Entry has been removed from the Pool.
	if e.index != notInPQueue {
		t.mu.pool.mu.q.update(e, t.release, e.mark)
		t.mu.pool.pickMightSucceedLocked()
	}
	t.mu.pool = nil
	return true
}
