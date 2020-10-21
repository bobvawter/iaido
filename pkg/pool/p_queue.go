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

// This is based on the reference code from the heap package.
type entryPQueue []*entryMeta

// Len implements heap.Interface.
func (e entryPQueue) Len() int {
	return len(e)
}

// Less implements heap.Interface.  Since we want a priority-queue
// behavior, we're inverting the comparison.
func (e entryPQueue) Less(i, j int) bool {
	a, b := e[i], e[j]
	return !a.lowerPriorityThan(b)
}

// Swap implements heap.Interface.
func (e entryPQueue) Swap(i, j int) {
	e[i], e[j] = e[j], e[i]
	e[i].index = i
	e[j].index = j
}

// Push implements heap.Interface.
func (e *entryPQueue) Push(x interface{}) {
	n := len(*e)
	entry := x.(*entryMeta)
	entry.index = n
	*e = append(*e, entry)
}

// Pop implements heap.Interface.
func (e *entryPQueue) Pop() interface{} {
	old := *e
	n := len(old)
	item := old[n-1]
	old[n-1] = nil
	item.index = -1
	*e = old[0 : n-1]
	return item
}
