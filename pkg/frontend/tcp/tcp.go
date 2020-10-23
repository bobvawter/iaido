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
	"math/rand"
	"net"
	"sync"
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
func Proxy(listener *net.TCPListener, idleDuration time.Duration) loop.Option {
	return loop.WithHandler(listener, func(ctx context.Context, incoming net.Conn) error {
		// An initial, happy-path backend will already be reserved
		// for us by the loop preflight.
		token := ctx.Value(balancer.KeyBackendToken).(*balancer.BackendToken)

		for {
			addr := token.Entry().Addr()
			conn, err := net.Dial(addr.Network(), addr.String())
			if err != nil {
				token.Release()
				token, err = token.Balancer().Wait(ctx)
				if err != nil {
					return err
				}
				continue
			}
			// CONTEXT BREAK: We want to isolate the copy operation from
			// any external context cancellation to allow for graceful
			// drain operations.
			return proxy(context.Background(), token, incoming.(*net.TCPConn), conn.(*net.TCPConn), idleDuration)
		}
	})
}

// proxy implements the main data-copying behavior for a TCP frontend.
func proxy(
	ctx context.Context,
	token *balancer.BackendToken,
	incoming, outgoing *net.TCPConn,
	idleDuration time.Duration,
) error {
	defer token.Release()
	backend := token.Entry()
	maintenanceTime := token.Balancer().Config().MaintenanceTime

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
	// * Shed load if a better backend has been available for some time.
	// * Enforce idleness timeout
	errGroup.Go(func() error {
		// We're likely to have a large number of connections arrive at
		// more or less the same time.  Smear the intervals over which
		// we're performing any kind of maintenance to ensure that we
		// don't drop all connections at exactly the same moment.
		smear := time.Duration(rand.Int63n(maintenanceTime.Nanoseconds()))
		ticker := time.NewTicker(maintenanceTime/2 + smear)
		defer ticker.Stop()

		var forceDisconnectReason error
		var promotableSince time.Time
		for {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case now := <-ticker.C:
				// See if there's room to promote the connection.
				if backend.Tier() > 0 && backend.ForcePromotionAfter() != 0 && promotableSince.IsZero() {
					if better := token.Balancer().BestAvailableTier(); better < backend.Tier() {
						promotableSince = now
					} else {
						promotableSince = time.Time{}
					}
				}

				var shouldDisconnect error

				if token.Balancer().IsOverloaded(backend) {
					shouldDisconnect = errors.Errorf("backend overloaded %s -> %s", incoming.RemoteAddr(), outgoing.RemoteAddr())
				} else if !promotableSince.IsZero() && now.After(promotableSince.Add(backend.ForcePromotionAfter())) {
					shouldDisconnect = errors.Errorf("forcing promotion of %s -> %s", incoming.RemoteAddr(), outgoing.RemoteAddr())
				} else if idle := idleSince.Load().(time.Time); !idle.IsZero() && now.After(idle.Add(idleDuration)) {
					shouldDisconnect = errors.Errorf("closing idle connection %s -> %s", incoming.RemoteAddr(), outgoing.RemoteAddr())
				}

				if shouldDisconnect == nil {
					forceDisconnectReason = nil
				} else if forceDisconnectReason == nil {
					forceDisconnectReason = shouldDisconnect
				} else {
					return forceDisconnectReason
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
			WakePeriod: time.Second,
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
			WakePeriod: time.Second,
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
