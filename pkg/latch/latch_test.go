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

package latch

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestLatch(t *testing.T) {
	a := assert.New(t)

	l := New()
	a.Equal(0, l.Count())
	a.False(l.Wait())

	l.Hold()
	a.Equal(1, l.Count())

	go func() {
		// This is gross :-(
		time.Sleep(10 * time.Millisecond)
		l.Release()
	}()

	a.True(l.Wait())
	a.Equal(0, l.Count())
}
