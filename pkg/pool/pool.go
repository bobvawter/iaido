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

// Package pool contains an implementation of a hierarchical, weighted
// selection pool.
package pool

import (
	"context"
	"sort"
	"sync"
)

// A Pool provides a weighted, tiered, round-robin selection mechanism
// for instances of pool.Entry.
//
// A Pool must not be copied after first use.
type Pool struct {
	mu struct {
		sync.Mutex
		// This is used to create a canonicalization of user-provided
		// Entry instances to our tracking metadata.
		canonical map[Entry]*entryMeta
		// Mark is a monotonic counter that's used to create a
		// round-robin effect.
		mark mark
		q    entryPQueue
		// Broadcasts calls to Add, Rebalance, and Release.
		//
		// Synchronized on mu.
		pickMightSucceed *sync.Cond
	}
}

// Add enrolls the given Entry into the Pool. It is a no-op to add the
// same Entry more than once.
func (p *Pool) Add(e Entry) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.mu.canonical == nil {
		p.mu.canonical = make(map[Entry]*entryMeta)
	}

	if existing := p.mu.canonical[e]; existing != nil {
		return
	}

	p.mu.canonical[e] = p.mu.q.add(e)
	p.pickMightSucceedLocked()
}

// All returns all entries currently in the Pool, sorted by preference.
func (p *Pool) All() []Entry {
	p.mu.Lock()
	all := make([]*entryMeta, len(p.mu.q))
	copy(all, p.mu.q)
	p.mu.Unlock()

	sort.Slice(all, func(i, j int) bool {
		return all[i].costLowerThan(all[j])
	})
	ret := make([]Entry, len(all))
	for i := range ret {
		ret[i] = all[i].Entry
	}
	return ret
}

// IsOverloaded returns true if the Entry is known to have a higher
// load than its configured maximum (e.g. when draining).
func (p *Pool) IsOverloaded(e Entry) bool {
	p.mu.Lock()
	defer p.mu.Unlock()

	meta := p.mu.canonical[e]
	if meta == nil {
		return true
	}
	meta.mu.RLock()
	defer meta.mu.RUnlock()

	return meta.mu.load > meta.mu.maxLoad
}

// Len returns the total size of the pool.
func (p *Pool) Len() int {
	p.mu.Lock()
	defer p.mu.Unlock()

	return len(p.mu.q)
}

// Load returns the currently outstanding load count on the entry.
func (p *Pool) Load(e Entry) int {
	p.mu.Lock()
	defer p.mu.Unlock()

	meta := p.mu.canonical[e]
	if meta == nil {
		return 0
	}
	meta.mu.RLock()
	defer meta.mu.RUnlock()
	return meta.mu.load
}

// MarshalYAML dumps the current status of the Pool into a YAML structure.
func (p *Pool) MarshalYAML() (interface{}, error) {
	p.mu.Lock()
	snapshot := make([]*entryMeta, len(p.mu.q))
	copy(snapshot, p.mu.q)
	p.mu.Unlock()

	payload := struct {
		Tiers map[int][]*entryMeta
	}{
		Tiers: make(map[int][]*entryMeta),
	}

	for _, entry := range snapshot {
		entry.mu.RLock()
		payload.Tiers[entry.mu.tier] = append(payload.Tiers[entry.mu.tier], entry)
		entry.mu.RUnlock()
	}
	return payload, nil
}

// Peek is a shortcut for PickLoad(0).
//
// This effectively returns the best-guess as to the next Entry to
// be returned from the pool.
func (p *Pool) Peek() *Token {
	return p.PickLoad(0)
}

// Pick is a shortcut for PickLoad(1).
func (p *Pool) Pick() *Token {
	return p.PickLoad(1)
}

// PickLoad will return a Token that provides access to an Entry in the
// pool that has at least the requested load available.
//
// The Token exists to act as a reference-counter on the Entry to
// ensure that no more than Entry.MaxLoad() Tokens exist for than entry
// when Pick is called.
//
// Token.Release() should be called once the Entry is no longer needed
// by the caller. A GC finalizer will be set up to avoid leaks, but the
// timing of the GC cannot be relied upon for correctness.
func (p *Pool) PickLoad(load int) *Token {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.pickLocked(load)
}

// Choose the best enabled entry.
func (p *Pool) pickLocked(load int) *Token {
	if len(p.mu.q) == 0 {
		return nil
	}

	// Fast-path: The min-heap satisfies the request with the first
	// element.
	top := p.mu.q[0]
	p.mu.mark++
	if p.mu.q.update(top, load, p.mu.mark) {
		return newToken(p, load, top)
	}

	// Slow-path: Iterate over sorted entries.
	// TODO(bob): Iterate directly over min-heap.
	sorted := make([]*entryMeta, len(p.mu.q)-1)
	copy(sorted, p.mu.q[1:])
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].costLowerThan(sorted[j])
	})

	for i := range sorted {
		if p.mu.q.update(sorted[i], load, p.mu.mark) {
			return newToken(p, load, sorted[i])
		}
	}

	return nil
}

// Rebalance will re-prioritize all entries based on their Load and
// Tier. This method only needs to be called if entries are expected
// to be picked infrequently, as their status will be sampled during
// a call to Pick().
func (p *Pool) Rebalance() {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.mu.q.updateAll()
	p.pickMightSucceedLocked()
}

// Remove discards the given entry from the pool.
//
// Any currently-issued Tokens for the Entry will remain valid, but
// became no-ops.
func (p *Pool) Remove(e Entry) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if existing := p.mu.canonical[e]; existing != nil {
		delete(p.mu.canonical, e)
		p.mu.q.remove(existing)
	}
}

// Wait is a blocking version of Pick() than can be interrupted by
// Context cancellation.
func (p *Pool) Wait(ctx context.Context, load int) (*Token, error) {
	ch := make(chan *Token)

	// This goroutine will loop until a call to pickLocked() succeeds.
	go func() {
		var picked *Token

		// Loop using a Condition until we're able to select an entry,
		// or the context has been canceled.
		p.mu.Lock()
		{
			if p.mu.pickMightSucceed == nil {
				p.mu.pickMightSucceed = sync.NewCond(&p.mu)
			}

			for picked = p.pickLocked(load); picked == nil && ctx.Err() == nil; picked = p.pickLocked(load) {
				p.mu.pickMightSucceed.Wait()
			}
		}
		p.mu.Unlock()

		if picked != nil {
			select {
			case <-ctx.Done():
				// Release the token, since there's nobody to receive it.
				picked.Release()
			case ch <- picked:
			}
		}
		close(ch)
	}()

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case picked := <-ch:
		return picked, nil
	}
}

// pickMightSucceedLocked is called to resume any Wait-ers.
//
// It uses Broadcast instead of Signal since there may be waiters
// of different costs.
func (p *Pool) pickMightSucceedLocked() {
	if p.mu.pickMightSucceed != nil {
		p.mu.pickMightSucceed.Broadcast()
	}
}
