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

// A Pool provides a tiered, round-robin selection mechanism.
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
		mark uint64
		q    entryPQueue
		// Broadcasts calls to Add and Rebalance. Synchronized on mu.
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

	if p.mu.pickMightSucceed != nil {
		p.mu.pickMightSucceed.Broadcast()
	}
}

// All returns all entries currently in the Pool.
func (p *Pool) All() []Entry {
	p.mu.Lock()
	all := make([]*entryMeta, len(p.mu.q))
	copy(all, p.mu.q)
	p.mu.Unlock()

	ret := make([]Entry, len(all))
	for i := range ret {
		ret[i] = all[i].Entry
	}
	return ret
}

// Len returns the total size of the pool.
func (p *Pool) Len() int {
	p.mu.Lock()
	ret := len(p.mu.q)
	p.mu.Unlock()
	return ret
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
		payload.Tiers[entry.tier] = append(payload.Tiers[entry.tier], entry)
	}
	for _, tier := range payload.Tiers {
		sort.SliceStable(tier, func(i, j int) bool {
			return tier[i].load > tier[j].load
		})
	}
	return payload, nil
}

// Pick will return the next Entry to use.
func (p *Pool) Pick() Entry {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.pickLocked()
}

// Choose the best enabled entry.
func (p *Pool) pickLocked() Entry {
	for range p.mu.q {
		top := p.mu.q[0]
		p.mu.mark++
		p.mu.q.update(top, p.mu.mark)

		if !top.disabled {
			return top.Entry
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
	p.mu.q.updateAll()
	if p.mu.pickMightSucceed != nil {
		p.mu.pickMightSucceed.Broadcast()
	}
	p.mu.Unlock()
}

// Remove discards the given entry from the pool.
func (p *Pool) Remove(e Entry) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if existing := p.mu.canonical[e]; existing != nil {
		delete(p.mu.canonical, e)
		p.mu.q.remove(existing)
	}
}

// Wait is a blocking version of Pick().
func (p *Pool) Wait(ctx context.Context) (Entry, error) {
	ch := make(chan Entry, 1)

	go func() {
		p.mu.Lock()
		if p.mu.pickMightSucceed == nil {
			p.mu.pickMightSucceed = sync.NewCond(&p.mu)
		}

		var picked Entry
		for picked = p.pickLocked(); picked == nil; picked = p.pickLocked() {
			p.mu.pickMightSucceed.Wait()
		}
		p.mu.Unlock()
		ch <- picked
	}()

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case picked := <-ch:
		return picked, nil
	}
}
