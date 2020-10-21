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

import (
	"bytes"
	"context"
	"net"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/bobvawter/iaido/pkg/loop"
	it "github.com/bobvawter/iaido/pkg/testing"
	"github.com/stretchr/testify/assert"
)

// This is a simple smoke-test to validate end-to-end reception.
func TestCopierHappyPath(t *testing.T) {
	const numChars = 4 * 1024 * 1024
	a := assert.New(t)
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	cgAddr, cgOpt, err := it.CharGen(numChars)
	if !a.NoError(err) {
		return
	}
	loop.New(cgOpt).Start(ctx)

	cgConn, err := net.Dial(cgAddr.Network(), cgAddr.String())
	if !a.NoError(err) {
		return
	}

	var sink bytes.Buffer
	var wg sync.WaitGroup
	sinkAddr, opt, err := it.Capture(&sink)
	if !a.NoError(err) {
		return
	}
	loop.New(opt, loop.WithWaitGroup(&wg)).Start(ctx)

	sinkConn, err := net.Dial(sinkAddr.Network(), sinkAddr.String())
	if !a.NoError(err) {
		return
	}

	var totalRead uint32
	var totalWrite uint32

	activity := func(read, write int) {
		atomic.AddUint32(&totalRead, uint32(read))
		atomic.AddUint32(&totalWrite, uint32(write))
	}

	c := &Copier{
		From:     cgConn,
		To:       sinkConn,
		Activity: activity,
		Linger:   10 * time.Millisecond,
	}
	a.NoError(c.Copy(ctx))
	wg.Wait()
	a.Equal(numChars, int(atomic.LoadUint32(&totalRead)))
	a.Equal(numChars, int(atomic.LoadUint32(&totalWrite)))
	a.Equal(numChars, sink.Len())
}
