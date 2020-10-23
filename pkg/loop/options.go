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

	"github.com/bobvawter/iaido/pkg/latch"
)

// An Option is used to provide additional configuration to a server or
// to request additional information from the server internals.
type Option interface {
	onConnection(ctx context.Context)
}

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

func (c *connectionCounter) onConnection(context.Context) {
	atomic.AddUint64(&c.ctr, 1)
}

// WithPreflight registers a function that may block acceptance of
// incoming connections.
func WithPreflight(fn func(context.Context) error) Option {
	return &preflight{fn}
}

type preflight struct {
	fn func(context.Context) error
}

func (*preflight) onConnection(context.Context) {}

// WithLatch will inject a Latch that is held whenever a
// connection is active.  This allows callers to implement a graceful
// draining strategy.
func WithLatch(l *latch.Latch) Option {
	return withLatch{l}
}

type withLatch struct {
	latch *latch.Latch
}

func (withLatch) onConnection(context.Context) {}

// WithTCPHandler defines a callback for TCP connections.
func WithTCPHandler(listener *net.TCPListener, fn func(context.Context, *net.TCPConn) error) Option {
	return &tcpHandler{listener, fn}
}

type tcpHandler struct {
	listener *net.TCPListener
	fn       func(context.Context, *net.TCPConn) error
}

func (h *tcpHandler) onConnection(context.Context) {}
