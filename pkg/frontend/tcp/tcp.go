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

	"sync"

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
	const connectionSampleTime = time.Second

	ctx, cancel := context.WithCancel(ctx)
	errGroup, ctx := errgroup.WithContext(ctx)

	// This sets up an idle-detection loop.
	var idleSince atomic.Value
	idleSince.Store(time.Time{})
	activity := func(written int64) {
		if written == 0 {
			idleSince.Store(time.Now())
		} else {
			idleSince.Store(time.Time{})
		}
	}

	// Maintenance tasks:
	// * Shed load on overloaded backends
	// * Shed load if a better backend has been avilable for some time
	// * Enforce idleness timeout
	errGroup.Go(func() error {
		// Capture
		idleDuration := idleDuration
		ticker := time.NewTicker(connectionSampleTime)
		var promotionAvailableSince time.Time
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-ticker.C:
				if backend.ShedLoad() {
					return errors.Errorf("shedding load %s -> %s", incoming.RemoteAddr(), outgoing.RemoteAddr())
				}

				if backend.Tier() > 0 && backend.ForcePromotionAfter() != 0 {
					if better := balancer.Pick(); better != nil && better.Tier() < backend.Tier() {
						if promotionAvailableSince.IsZero() {
							promotionAvailableSince = time.Now()
						} else if time.Now().After(promotionAvailableSince.Add(backend.ForcePromotionAfter())) {
							return errors.Errorf("forcing promotion of %s -> %s", incoming.RemoteAddr(), outgoing.RemoteAddr())
						}
					} else {
						promotionAvailableSince = time.Time{}
					}
				}

				when := idleSince.Load().(time.Time)
				if !when.IsZero() && time.Now().After(when.Add(idleDuration)) {
					return errors.Errorf("closing idle connection %s -> %s", incoming.RemoteAddr(), outgoing.RemoteAddr())
				}
			}
		}
	})

	// Copy incoming data to outgoing connection.
	var copyGroup sync.WaitGroup
	copyGroup.Add(2)
	errGroup.Go(func() error {
		err := (&copier.Copier{
			Activity:   activity,
			From:       incoming,
			To:         outgoing,
			WakePeriod: connectionSampleTime,
		}).Copy(ctx)
		_ = incoming.CloseRead()
		_ = outgoing.CloseWrite()
		copyGroup.Done()
		return err
	})

	// Copy returning data back to client.
	errGroup.Go(func() error {
		err := (&copier.Copier{
			Activity:   activity,
			From:       outgoing,
			To:         incoming,
			WakePeriod: connectionSampleTime,
		}).Copy(ctx)
		_ = outgoing.CloseRead()
		_ = incoming.CloseWrite()
		copyGroup.Done()
		return err
	})

	copyGroup.Wait()
	cancel()

	err := errGroup.Wait()
	if errors.Is(err, context.Canceled) {
		err = nil
	}
	return err
}
