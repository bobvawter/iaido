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
	"encoding/json"
	"io"
	"log"
	"net"
	"runtime/debug"
	"sync"
	"time"

	"github.com/bobvawter/iaido/pkg/balancer"
	"github.com/bobvawter/iaido/pkg/config"
	"github.com/bobvawter/iaido/pkg/frontend/tcp"
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
	wg sync.WaitGroup
	mu struct {
		sync.Mutex
		// A map of local listen addresses to the plumbing that handles
		// the connectivity.
		loops map[config.Target]*balanceLoop
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
		f.mu.loops = make(map[config.Target]*balanceLoop)
	}

	activeTargets := make(map[config.Target]bool)

	for _, fe := range cfg.Frontends {
		tgt, err := config.ParseTarget(fe.BindAddress)
		if err != nil {
			return errors.Wrapf(err, "could not bind %q", fe.BindAddress)
		}
		activeTargets[tgt] = true

		bl := f.mu.loops[tgt]
		if bl == nil {
			loopCtx, cancel := context.WithCancel(ctx)
			bl = &balanceLoop{
				balancer: &balancer.Balancer{},
				cancel:   cancel,
			}

			if err := bl.balancer.Configure(loopCtx, &fe.BackendPool, true); err != nil {
				return errors.Wrapf(err, "could not configure backend pool for %q", fe.BindAddress)
			}

			f.mu.loops[tgt] = bl

			switch tgt.Proto {
			case config.TCP:
				listener, err := net.ListenTCP("tcp", &net.TCPAddr{
					IP:   net.ParseIP(tgt.Host),
					Port: tgt.Port,
				})
				if err != nil {
					return errors.Wrapf(err, "could not bind %q", tgt)
				}
				log.Printf("listening on %s", listener.Addr())
				bl.listener = listener

				idleDuration, err := time.ParseDuration(fe.IdleDuration)
				if err != nil {
					return errors.Wrapf(err, "could not bind %q", tgt)
				}

				loop.New(
					tcp.Proxy(listener, bl.balancer, idleDuration),
					loop.WithMaxWorkers(fe.IncomingConnections),
					loop.WithWaitGroup(&f.wg),
				).Start(loopCtx)
			default:
				panic(errors.Errorf("unimplemented: %s", tgt.Proto))
			}
		} else if err := bl.balancer.Configure(ctx, &fe.BackendPool, true); err != nil {
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

// MarshalJSON implements json.Marshaler and provides diagnostic information.
func (f *Frontend) MarshalJSON() ([]byte, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	payload := struct {
		BuildID   string
		BuildInfo *debug.BuildInfo
		Listeners map[string]*balancer.Balancer
	}{
		BuildID:   buildID,
		Listeners: make(map[string]*balancer.Balancer),
	}
	if info, ok := debug.ReadBuildInfo(); ok {
		payload.BuildInfo = info
	}
	for tgt, bl := range f.mu.loops {
		payload.Listeners[tgt.String()] = bl.balancer
	}
	return json.Marshal(payload)
}

// Wait for all active connections to drain.
func (f *Frontend) Wait() {
	f.wg.Wait()
}
