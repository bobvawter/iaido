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
	// MaxLoad allows an entry to specify the number of times that it
	// can be concurrently picked from the pool.
	//
	// The MaxLoad value will be re-checked when an Entry is about to be
	// picked, which provides callers with the opportunity to provide
	// dynamic admission control.
	//
	// If MaxLoad() is less than 1, the Entry can no longer be picked,
	// however this will not invalidate any currently-issued Tokens.
	MaxLoad() int
	// Tier establishes equivalence classes within the pool.
	// Lower-numbered tiers are given higher priority when selecting an
	// entry.
	Tier() int
}

// entryMeta associates additional metadata with a user-provided Entry.
//
// This type contains snapshots of the information that we gather
// from the user-provided Entry in order to ensure stability during
// comparison operations.
type entryMeta struct {
	Entry

	// This is the internal index used by the heap package.
	index int
	// The current number of tokens outstanding for the entry.
	load int
	// Record when the entry was last chosen to create a round-robin
	// effect.
	mark mark
	// Snapshot of MaxLoad().
	maxLoad int
	// Snapshot of Tier().
	tier int
}

// A mark is used as a selection counter to provide a basic round-robin
// behavior, presuming all other weighting concerns are equal.
type mark uint64

func (e *entryMeta) MarshalYAML() (interface{}, error) {
	payload := struct {
		Entry   interface{}
		Load    int
		Mark    mark
		MaxLoad int
		Tier    int
	}{
		Entry:   e.Entry,
		Load:    e.load,
		Mark:    e.mark,
		MaxLoad: e.maxLoad,
		Tier:    e.tier,
	}
	return payload, nil
}

func (e *entryMeta) costLowerThan(other *entryMeta) bool {
	// Overloaded nodes are always higher-cost than not.
	eOverload := e.load >= e.maxLoad
	otherOverload := other.load >= other.maxLoad
	if eOverload != otherOverload {
		return otherOverload
	}

	// Prefer closer tiers.
	if c := e.tier - other.tier; c != 0 {
		return c < 0
	}
	// Prefer lower loads.
	if c := e.load - other.load; c != 0 {
		return c < 0
	}
	// Otherwise, round-robin.
	return e.mark < other.mark
}
