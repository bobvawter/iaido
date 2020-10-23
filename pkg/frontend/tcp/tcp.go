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

// Package tcp implements a TCP proxy service.
package tcp

import (
	"context"
	"net"
	"sync/atomic"
	"time"

	"github.com/bobvawter/iaido/pkg/balancer"
	"github.com/bobvawter/iaido/pkg/copier"
	"github.com/bobvawter/iaido/pkg/loop"
	"github.com/pkg/errors"
	"golang.org/x/sync/errgroup"
)

// Proxy constructs a TCP proxy frontend around a balancer service.
//
// A non-zero idleDuration will disconnect lingering connections.
func Proxy(listener *net.TCPListener, balancer *balancer.Balancer, idleDuration time.Duration) loop.Option {
	return loop.WithTCPHandler(listener, func(_ context.Context, incoming *net.TCPConn) error {
		// CONTEXT BREAK: We want to isolate the copy operation from
		// any external context cancellation to allow for graceful
		// drain operations.
		ctx := context.Background()

		for {
			backend := balancer.Pick()
			if backend == nil {
				return errors.Errorf("no backend available")
			}

			retry, err := backend.Dial(ctx, func(ctx context.Context, outgoing net.Conn) error {
				return proxy(ctx, balancer, backend, incoming, outgoing.(*net.TCPConn), idleDuration)
			})
			if retry {
				continue
			}
			return err
		}
	})
}

// proxy implements the main data-copying behavior for a TCP frontend.
func proxy(
	ctx context.Context,
	balancer *balancer.Balancer,
	backend *balancer.Backend,
	incoming, outgoing *net.TCPConn,
	idleDuration time.Duration,
) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	group, ctx := errgroup.WithContext(ctx)

	// This sets up an idle-detection loop.
	var activeAt atomic.Value
	activeAt.Store(time.Now().Add(idleDuration))
	activity := func(int64) {
		activeAt.Store(time.Now())
	}

	group.Go(func() error {
		for {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.NewTimer(idleDuration).C:
				if time.Now().After(activeAt.Load().(time.Time).Add(idleDuration)) {
					return errors.Errorf("idle timeout (%s) exceeded %s -> %s", idleDuration, incoming.RemoteAddr(), outgoing.RemoteAddr())
				}
			}
		}
	})

	group.Go(func() error {
		maybeForce := backend.Tier() > 0
		for {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.NewTimer(time.Second).C:
				// Try to force clients back to a higher tier down to a
				// reasonable minimum for highly-active flows.
				if maybeForce {
					if better := balancer.Pick(); better != nil && better.Tier() < backend.Tier() {
						maybeForce = false
						idleDuration = 10 * time.Millisecond
					}
				}
				if backend.ShedLoad() {
					return errors.Errorf("shedding load %s -> %s", incoming.RemoteAddr(), outgoing.RemoteAddr())
				}
			}
		}
	})

	// Copy incoming data to outgoing connection.
	group.Go(func() error {
		err := (&copier.Copier{
			From:     incoming,
			To:       outgoing,
			Activity: activity,
		}).Copy(ctx)
		_ = incoming.CloseRead()
		_ = outgoing.CloseWrite()
		cancel()
		return err
	})

	// Copy returning data back to client.
	group.Go(func() error {
		err := (&copier.Copier{
			From:     outgoing,
			To:       incoming,
			Activity: activity,
		}).Copy(ctx)
		_ = outgoing.CloseRead()
		_ = incoming.CloseWrite()
		cancel()
		return err
	})

	err := group.Wait()
	if errors.Is(err, context.Canceled) {
		err = nil
	}
	return err
}
