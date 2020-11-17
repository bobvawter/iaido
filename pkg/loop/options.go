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

package loop

import (
	"context"
	"net"
	"sync/atomic"

	"github.com/bobvawter/latch"
)

// An Option is used to provide additional configuration to a server or
// to request additional information from the server internals.
type Option interface {
	is(option)
}

type option struct{}

// WithConnectionCounter returns a metric function that indicates how
// many network connections have been received.
func WithConnectionCounter() (Option, func() uint64) {
	var counter connectionCounter
	return &counter, func() uint64 {
		return atomic.LoadUint64(&counter.ctr)
	}
}

type connectionCounter struct {
	ctr uint64
}

func (c *connectionCounter) onConnection(_ context.Context) {
	atomic.AddUint64(&c.ctr, 1)
}

func (c *connectionCounter) is(option) {}

// WithPreflight registers a function that can block connection
// acceptance and well as replace or update the Context to be used later
// in the request.
func WithPreflight(fn func(context.Context) (context.Context, error)) Option {
	return &preflight{fn}
}

type preflight struct {
	fn func(context.Context) (context.Context, error)
}

func (*preflight) is(option) {}

// WithLatch will inject a Latch that is held whenever a
// connection is active.  This allows callers to implement a graceful
// draining strategy.
func WithLatch(l *latch.Counter) Option {
	return withLatch{l}
}

type withLatch struct {
	latch *latch.Counter
}

func (withLatch) is(option) {}

// WithHandler provides a listeners
func WithHandler(listener net.Listener, fn func(context.Context, net.Conn) error) Option {
	return &handler{listener, fn}
}

type handler struct {
	listener net.Listener
	fn       func(context.Context, net.Conn) error
}

func (*handler) is(option) {}
