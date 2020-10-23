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

	"github.com/bobvawter/iaido/pkg/latch"
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
	var tcpLoops []*tcpHandler
	wait := func(ctx context.Context) error { return ctx.Err() }

	// Filter out "static" options.
	n := 0
	for _, opt := range s.options {
		switch t := opt.(type) {
		case *preflight:
			wait = t.fn
		case *tcpHandler:
			tcpLoops = append(tcpLoops, t)
		case withLatch:
			toLatch = t.latch
		default:
			s.options[n] = opt
			n++
		}
	}
	s.options = s.options[:n]

	for _, tcp := range tcpLoops {
		go func(tcp *tcpHandler) {
			for {
				if err := wait(ctx); err != nil {
					log.Print(errors.Wrapf(err, "server loop exiting"))
					return
				}
				_ = tcp.listener.SetDeadline(time.Now().Add(1 * time.Second))
				conn, err := tcp.listener.AcceptTCP()
				if opErr, ok := err.(*net.OpError); ok && opErr.Timeout() {
					continue
				}
				if err != nil {
					log.Print(errors.Wrap(err, "server loop exiting"))
					return
				}
				toLatch.Hold()
				go processConnection(ctx, toLatch, tcp, conn, s.options)
			}
		}(tcp)
	}
}

func processConnection(
	ctx context.Context, l *latch.Latch,
	tcp *tcpHandler, conn *net.TCPConn, options []Option,
) {
	defer l.Release()
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	for i := range options {
		options[i].onConnection(ctx)
	}
	if err := tcp.fn(ctx, conn); err != nil {
		log.Printf("handler error %v", err)
	}
	_ = conn.Close()
}
