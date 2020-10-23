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
		Entry:   entry,
		index:   notInPQueue,
		load:    0,
		mark:    0,
		maxLoad: entry.MaxLoad(),
		tier:    entry.Tier(),
	}
	heap.Push(q, ret)
	return ret
}

func (q *entryPQueue) remove(meta *entryMeta) {
	heap.Remove(q, meta.index)
}

func (q *entryPQueue) update(meta *entryMeta, loadDelta int, mark mark) bool {
	ret := false

	meta.mark = mark
	meta.maxLoad = meta.MaxLoad()
	meta.tier = meta.Tier()
	if meta.load+loadDelta <= meta.maxLoad {
		meta.load += loadDelta
		ret = true
	}
	heap.Fix(q, meta.index)
	return ret
}

func (q *entryPQueue) updateAll() {
	for _, meta := range *q {
		meta.maxLoad = meta.MaxLoad()
		meta.tier = meta.Tier()
	}
	heap.Init(q)
}
