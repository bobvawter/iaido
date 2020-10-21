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
	"encoding/json"
)

// An Entry represents something that can be picked out of a Pool.
type Entry interface {
	// Disabled should return true if the entry is currently ineligible
	// to be returned from Pick.
	Disabled() bool
	// Load represents a weighting within a tier. Entries with lower
	// load values are given higher priority when selecting an entry.
	Load() int
	// Tier establishes an equivalence class within the pool.
	// Lower-numbered tiers are given higher priority when selecting an
	// entry.
	Tier() int
}

// entryMeta associates additional metadata with a user-provide Entry.
type entryMeta struct {
	Entry
	// This is the internal index used by the heap package.
	index int
	// Snapshot of Load().
	load int
	// Record when the entry was last chosen to create a round-robin
	// effect.
	mark uint64
	// Snapshot of Tier().
	tier int
}

func (e *entryMeta) MarshalJSON() ([]byte, error) {
	payload := struct {
		Entry interface{}
		Load  int
		Mark  uint64
		Tier  int
	}{
		Entry: e.Entry,
		Load:  e.load,
		Mark:  e.mark,
		Tier:  e.tier,
	}
	return json.Marshal(payload)
}

func (e *entryMeta) lowerPriorityThan(other *entryMeta) bool {
	if e.tier > other.tier {
		return true
	}
	if e.load > other.load {
		return true
	}
	if e.mark > other.mark {
		return true
	}
	return false
}

// snapshot updates the local metadata information.
func (e *entryMeta) snapshot() {
	e.load = e.Load()
	e.tier = e.Tier()
}
