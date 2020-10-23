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

// Package frontend contains the logic for running a network frontend
// that proxies connections to a backend pool.
package frontend

import (
	"context"
	"io"
	"log"
	"net"
	"runtime/debug"
	"sync"
	"time"

	"github.com/bobvawter/iaido/pkg/balancer"
	"github.com/bobvawter/iaido/pkg/config"
	"github.com/bobvawter/iaido/pkg/frontend/tcp"
	"github.com/bobvawter/iaido/pkg/latch"
	"github.com/bobvawter/iaido/pkg/loop"
	"github.com/pkg/errors"
)

// Set by linker during binary builds.
var buildID = "development"

// BuildID returns a string that will be injected during the link phase.
func BuildID() string {
	return buildID
}

// A Frontend is intended to be a durable instance which is
// reconfigured as the need arises.
type Frontend struct {
	mu struct {
		sync.Mutex
		latch *latch.Latch
		// A map of local listen addresses to the plumbing that handles
		// the connectivity.
		loops map[string]*balanceLoop
	}
}

type balanceLoop struct {
	balancer *balancer.Balancer
	cancel   context.CancelFunc
	listener io.Closer
}

// Ensure will start/stop/continue the server loops defined in the
// configuration.
func (f *Frontend) Ensure(ctx context.Context, cfg *config.Config) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.mu.loops == nil {
		f.mu.loops = make(map[string]*balanceLoop)
	}

	if f.mu.latch == nil {
		f.mu.latch = latch.New()
	}

	activeTargets := make(map[string]bool)

	for _, fe := range cfg.Frontends {
		tgt, err := config.ParseTarget(fe.BindAddress)
		if err != nil {
			return errors.Wrapf(err, "could not bind %q", fe.BindAddress)
		}
		activeTargets[tgt.String()] = true

		bl := f.mu.loops[tgt.String()]
		if bl == nil {
			loopCtx, cancel := context.WithCancel(ctx)
			bl = &balanceLoop{
				balancer: &balancer.Balancer{},
				cancel:   cancel,
			}

			if err := bl.balancer.Configure(loopCtx, fe.BackendPool, true); err != nil {
				return errors.Wrapf(err, "could not configure backend pool for %q", fe.BindAddress)
			}

			f.mu.loops[tgt.String()] = bl

			// Set up a loop to rebalance the pool (e.g. to reactivate
			// disabled backends or prune overloaded nodes.)
			go func(b *balancer.Balancer) {
				ticker := time.NewTicker(time.Second)
				defer ticker.Stop()
				for {
					select {
					case <-loopCtx.Done():
						return
					case <-ticker.C:
						b.Rebalance(loopCtx)
					}
				}
			}(bl.balancer)

			switch tgt.Proto {
			case config.TCP:
				listener, err := net.ListenTCP("tcp", &net.TCPAddr{
					IP:   net.ParseIP(tgt.Hosts[0]),
					Port: tgt.Port,
				})
				if err != nil {
					return errors.Wrapf(err, "could not bind %q", tgt)
				}
				log.Printf("listening on %s", listener.Addr())
				bl.listener = listener

				loop.New(
					tcp.Proxy(listener, bl.balancer, fe.IdleDuration),
					loop.WithPreflight(func(ctx context.Context) error {
						_, err := bl.balancer.Wait(ctx)
						return err
					}),
					loop.WithLatch(f.mu.latch),
				).Start(loopCtx)
			default:
				panic(errors.Errorf("unimplemented: %s", tgt.Proto))
			}
		} else if err := bl.balancer.Configure(ctx, fe.BackendPool, true); err != nil {
			return errors.Wrapf(err, "could not configure backend pool for %q", fe.BindAddress)
		}

	}

	for tgt, bl := range f.mu.loops {
		if !activeTargets[tgt] {
			bl.cancel()
			_ = bl.listener.Close()
			delete(f.mu.loops, tgt)
		}
	}
	return nil
}

// MarshalYAML implements yaml.Marshaler and provides diagnostic information.
func (f *Frontend) MarshalYAML() (interface{}, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	payload := struct {
		BuildID   string                        `yaml:"buildID"`
		BuildInfo *debug.BuildInfo              `yaml:"buildInfo"`
		Listeners map[string]*balancer.Balancer `yaml:"listeners"`
	}{
		BuildID:   buildID,
		Listeners: make(map[string]*balancer.Balancer),
	}
	if info, ok := debug.ReadBuildInfo(); ok {
		payload.BuildInfo = info
	}
	for tgt, bl := range f.mu.loops {
		payload.Listeners[tgt] = bl.balancer
	}
	return payload, nil
}

// Wait for all active connections to drain.
func (f *Frontend) Wait() {
	f.mu.Lock()
	latch := f.mu.latch
	f.mu.Unlock()

	if latch != nil {
		latch.Wait()
	}
}
