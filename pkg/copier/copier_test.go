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
	"context"
	"net"
	"sync/atomic"
	"testing"
	"time"

	"io/ioutil"

	"github.com/bobvawter/iaido/pkg/loop"
	it "github.com/bobvawter/iaido/pkg/testing"
	"github.com/bobvawter/latch"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
)

// This is a simple smoke-test to validate end-to-end reception.
func TestCopier(t *testing.T) {
	const numChars = 10 * 1024 * 1024
	a := assert.New(t)
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	cgAddr, cgOpt, err := it.CharGen(ctx, numChars)
	if !a.NoError(err) {
		return
	}
	loop.New(cgOpt).Start(ctx)

	l := latch.New()
	sinkAddr, opt, err := it.Capture(ctx, ioutil.Discard)
	if !a.NoError(err) {
		return
	}
	loop.New(opt, loop.WithLatch(l)).Start(ctx)

	t.Run("HappyPath", func(t *testing.T) {
		a := assert.New(t)

		cgConn, err := net.Dial(cgAddr.Network(), cgAddr.String())
		if !a.NoError(err) {
			return
		}
		sinkConn, err := net.Dial(sinkAddr.Network(), sinkAddr.String())
		if !a.NoError(err) {
			return
		}
		var totalWrite int64

		activity := func(write int64) {
			atomic.AddInt64(&totalWrite, write)
		}

		c := &Copier{
			Activity: activity,
			From:     cgConn,
			To:       sinkConn,
		}
		a.NoError(c.Copy(ctx))
		_ = cgConn.Close()
		_ = sinkConn.Close()

		a.Equal(numChars, int(atomic.LoadInt64(&totalWrite)))
	})

	// Ensure that the context-checking deadline fires.
	t.Run("WithCancel", func(t *testing.T) {
		a := assert.New(t)
		ctx, cancel := context.WithCancel(ctx)
		defer cancel()

		cgConn, err := net.Dial(cgAddr.Network(), cgAddr.String())
		if !a.NoError(err) {
			return
		}
		sinkConn, err := net.Dial(sinkAddr.Network(), sinkAddr.String())
		if !a.NoError(err) {
			return
		}

		activity := func(write int64) {
			cancel()
		}

		c := &Copier{
			From:       cgConn,
			To:         sinkConn,
			Activity:   activity,
			WakePeriod: 1 * time.Millisecond, // Force context wakeups.
		}
		a.True(errors.Is(c.Copy(ctx), context.Canceled))
		_ = cgConn.Close()
		_ = sinkConn.Close()
	})
}
