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

package copier

import "sync"

const bufferCapacity = 32767

var (
	allZeros = make([]byte, bufferCapacity)
	thePool  bufPool
)

type bufPool struct {
	pool sync.Pool
}

// Get retrieves a clean byte buffer.
func (p *bufPool) Get() []byte {
	ret := p.pool.Get()
	if ret == nil {
		return make([]byte, bufferCapacity)
	}
	return *ret.(*[]byte)
}

// Put cleans the buffer before adding it to the free list.
func (p *bufPool) Put(buf []byte) {
	buf = buf[:bufferCapacity:bufferCapacity]
	copy(buf, allZeros)
	p.pool.Put(&buf)
}
