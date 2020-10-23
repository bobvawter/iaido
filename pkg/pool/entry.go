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
//
// This type contains snapshots of the information that we gather
// from the user-provided Entry in order to ensure stability during
// comparison operations.
type entryMeta struct {
	Entry

	// Snapshot of Disabled().
	disabled bool
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

func (e *entryMeta) MarshalYAML() (interface{}, error) {
	payload := struct {
		Disabled bool
		Entry    interface{}
		Load     int
		Mark     uint64
		Tier     int
	}{
		Disabled: e.disabled,
		Entry:    e.Entry,
		Load:     e.load,
		Mark:     e.mark,
		Tier:     e.tier,
	}
	return payload, nil
}

func (e *entryMeta) costLowerThan(other *entryMeta) bool {
	if e.disabled != other.disabled {
		return other.disabled
	}
	if c := e.tier - other.tier; c != 0 {
		return c < 0
	}
	if c := e.load - other.load; c != 0 {
		return c < 0
	}
	return e.mark < other.mark
}
