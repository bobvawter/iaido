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
	"container/heap"
	"encoding/json"
	"sort"
	"sync"
)

// A Pool provides a tiered, round-robin selection mechanism.
//
// A Pool must not be copied after first use.
type Pool struct {
	mu struct {
		sync.Mutex
		disabled []*entryMeta
		mark     uint64
		meta     map[Entry]*entryMeta
		q        entryPQueue
	}
}

// Add enrolls the given Entry into the Pool. It is a no-op to add the
// same Entry more than once.
func (p *Pool) Add(e Entry) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.mu.meta == nil {
		p.mu.meta = make(map[Entry]*entryMeta)
	}

	if existing := p.mu.meta[e]; existing != nil {
		return
	}

	entry := &entryMeta{Entry: e}
	p.mu.meta[e] = entry
	entry.snapshot()
	heap.Push(&p.mu.q, entry)
}

// All returns all entries currently in the Pool.
func (p *Pool) All() []Entry {
	p.mu.Lock()
	all := make([]*entryMeta, 0, len(p.mu.q)+len(p.mu.disabled))
	all = append(append(all, p.mu.q...), p.mu.disabled...)
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
	ret := len(p.mu.q) + len(p.mu.disabled)
	p.mu.Unlock()
	return ret
}

// MarshalJSON dumps the current status of the Pool into a JSON structure.
func (p *Pool) MarshalJSON() ([]byte, error) {
	p.mu.Lock()
	d := make([]*entryMeta, len(p.mu.disabled))
	copy(d, p.mu.disabled)
	q := make([]*entryMeta, len(p.mu.q))
	copy(q, p.mu.q)
	p.mu.Unlock()

	payload := struct {
		Disabled []*entryMeta
		Tiers    map[int][]*entryMeta
	}{
		Disabled: d,
		Tiers:    make(map[int][]*entryMeta, len(q)),
	}
	for _, entry := range q {
		payload.Tiers[entry.tier] = append(payload.Tiers[entry.tier], entry)
	}
	for _, tier := range payload.Tiers {
		sort.SliceStable(tier, func(i, j int) bool {
			return tier[i].load > tier[j].load
		})
	}
	return json.Marshal(payload)
}

// Rebalance will re-prioritize all entries based on their Load and
// Tier. This method only needs to be called if entries are expected
// to be picked infrequently, as their status will be sampled during
// a call to Pick().
func (p *Pool) Rebalance() {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.reenableLocked()
	for i := range p.mu.q {
		p.mu.q[i].snapshot()
	}
	heap.Init(&p.mu.q)
}

// Pick will return the next Entry to use.
func (p *Pool) Pick() Entry {
	p.mu.Lock()
	defer p.mu.Unlock()

	// Re-seed with re-enabled entries.
	p.reenableLocked()

	for {
		// Empty case, all entries are disabled.
		if len(p.mu.q) == 0 {
			return nil
		}

		// Choose the best enabled entry.
		if top := p.chooseLocked(); top != nil {
			return top.Entry
		}
	}
}

// Remove discards the given entry from the pool.
func (p *Pool) Remove(e Entry) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if existing := p.mu.meta[e]; existing != nil {
		delete(p.mu.meta, e)

		for i := range p.mu.disabled {
			if p.mu.disabled[i].Entry == e {
				p.mu.disabled = append(p.mu.disabled[:i], p.mu.disabled[i+1:]...)
				return
			}
		}

		heap.Remove(&p.mu.q, existing.index)
	}
}

// chooseLocked will return the best entry.  If the best entry is
// disabled, it will be moved into the disabled pool and nil will be
// returned.
func (p *Pool) chooseLocked() *entryMeta {
	top := p.mu.q[0]
	if top.Disabled() {
		heap.Pop(&p.mu.q)
		p.mu.disabled = append(p.mu.disabled, top)
		return nil
	}
	top.snapshot()
	p.mu.mark++
	top.mark = p.mu.mark
	heap.Fix(&p.mu.q, top.index)
	return top
}

// reenabledLocked moves re-enabled entries from the disabled list back
// into the head.
func (p *Pool) reenableLocked() {
	n := 0
	for i, meta := range p.mu.disabled {
		if meta.Disabled() {
			p.mu.disabled[n] = p.mu.disabled[i]
			n++
		} else {
			meta.snapshot()
			heap.Push(&p.mu.q, meta)
		}
	}
	p.mu.disabled = p.mu.disabled[:n]
}
