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

// Package loop implements a generic server loop.
package loop

import (
	"context"
	"log"
	"net"
	"time"

	"github.com/bobvawter/latch"
	"github.com/pkg/errors"
)

// A Loop is a generic server loop.
type Loop struct {
	options []Option
}

// New constructs a runtime loop configured by the given options.
func New(options ...Option) *Loop {
	safe := make([]Option, len(options))
	copy(safe, options)
	return &Loop{
		options: safe,
	}
}

// Start executes the server loop.
//
// This method is non-blocking and will spawn additional goroutines.
func (s *Loop) Start(ctx context.Context) {
	toLatch := latch.New()
	var handlers []*handler
	wait := func(ctx context.Context) (context.Context, error) { return ctx, ctx.Err() }

	// Filter out "static" options.
	n := 0
	for _, opt := range s.options {
		switch t := opt.(type) {
		case *preflight:
			wait = t.fn
		case *handler:
			handlers = append(handlers, t)
		case withLatch:
			toLatch = t.latch
		default:
			s.options[n] = opt
			n++
		}
	}
	s.options = s.options[:n]

	for _, h := range handlers {
		go func(h *handler) {
			for {
				reqContext, err := wait(ctx)
				if err != nil {
					log.Print(errors.Wrapf(err, "server loop exiting"))
					return
				}

				conn, err := listen(reqContext, h.listener)
				if err != nil {
					if err != context.Canceled {
						log.Print(errors.Wrap(err, "server loop exiting"))
					}
					return
				}

				reqContext, cancel := context.WithCancel(reqContext)
				for i := range s.options {
					if x, ok := s.options[i].(interface{ onConnection(context.Context) }); ok {
						x.onConnection(reqContext)
					}
				}

				toLatch.Hold()
				go func() {
					if err := h.fn(reqContext, conn); err != nil {
						log.Printf("handler error %v", err)
					}
					_ = conn.Close()
					cancel()
					toLatch.Release()
				}()
			}
		}(h)
	}
}

// listen is a cancellable loop to accept an incoming network connection.
func listen(ctx context.Context, listener net.Listener) (net.Conn, error) {
	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}
		if x, ok := listener.(interface{ SetDeadline(time.Time) error }); ok {
			_ = x.SetDeadline(time.Now().Add(1 * time.Second))
		}
		conn, err := listener.Accept()
		if opErr, ok := err.(*net.OpError); ok && opErr.Timeout() {
			continue
		}
		if err != nil {
			return nil, err
		}
		return conn, nil
	}
}
