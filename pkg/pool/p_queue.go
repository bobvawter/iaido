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
	"container/heap"
	"math"
)

const notInPQueue = math.MaxInt64

// This is based on the reference code from the heap package.
//
// Callers must not modify the slice directly, all updates should
// be done through the mutation methods.
//
// This type is not internally synchronized.
type entryPQueue []*entryMeta

// Len implements heap.Interface.
func (q entryPQueue) Len() int {
	return len(q)
}

// Less implements heap.Interface.
func (q entryPQueue) Less(i, j int) bool {
	a, b := q[i], q[j]
	return a.costLowerThan(b)
}

// Swap implements heap.Interface.
func (q entryPQueue) Swap(i, j int) {
	q[i], q[j] = q[j], q[i]
	q[i].index = i
	q[j].index = j
}

// Push implements heap.Interface.
func (q *entryPQueue) Push(x interface{}) {
	n := len(*q)
	entry := x.(*entryMeta)
	entry.index = n
	*q = append(*q, entry)
}

// Pop implements heap.Interface.
func (q *entryPQueue) Pop() interface{} {
	old := *q
	n := len(old)
	item := old[n-1]
	item.index = notInPQueue
	old[n-1] = nil
	*q = old[0 : n-1]
	return item
}

func (q *entryPQueue) add(entry Entry) *entryMeta {
	ret := &entryMeta{
		Entry: entry,
		index: notInPQueue,
	}
	ret.mu.maxLoad = entry.MaxLoad()
	ret.mu.tier = entry.Tier()
	heap.Push(q, ret)
	return ret
}

func (q *entryPQueue) remove(meta *entryMeta) {
	heap.Remove(q, meta.index)
}

func (q *entryPQueue) update(meta *entryMeta, loadDelta int, mark mark) bool {
	ret := false

	meta.mu.Lock()
	if mark != noMark {
		meta.mu.mark = mark
	}
	meta.mu.maxLoad = meta.MaxLoad()
	meta.mu.tier = meta.Tier()
	// Only allow the load to increase by the delta, up to the max.
	if loadDelta > 0 && meta.mu.load+loadDelta <= meta.mu.maxLoad {
		meta.mu.load += loadDelta
		ret = true
	} else if loadDelta <= 0 {
		// But always allow load to be shed (e.g. drain a 0-cap backend)
		meta.mu.load += loadDelta
		ret = true
	}
	meta.mu.Unlock()

	heap.Fix(q, meta.index)
	return ret
}

func (q *entryPQueue) updateAll() {
	for _, meta := range *q {
		meta.mu.Lock()
		meta.mu.maxLoad = meta.MaxLoad()
		meta.mu.tier = meta.Tier()
		meta.mu.Unlock()
	}
	heap.Init(q)
}
