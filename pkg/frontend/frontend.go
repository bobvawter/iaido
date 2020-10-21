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
		// A map of local listen addresses to the balancer handling the
		// fanout.
		balancers                map[config.Target]*balancer.Balancer
		cancelPreviousGeneration context.CancelFunc
		config                   *config.Config
		// We want to preserve active network listeners across
		// reconfigurations. The value type is io.Closer, since the only
		// thing that Frontend might need to do is to close the listener
		// if it's no longer needed.
		listeners map[config.Target]io.Closer
	}
}

// Ensure will start/stop/continue the server loops defined in the
// configuration.
func (f *Frontend) Ensure(ctx context.Context, cfg *config.Config) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.mu.balancers == nil {
		f.mu.balancers = make(map[config.Target]*balancer.Balancer)
	}
	if f.mu.listeners == nil {
		f.mu.listeners = make(map[config.Target]io.Closer)
	}

	activeBalancers := make(map[config.Target]*balancer.Balancer)
	var toStart []*loop.Loop

	for _, fe := range cfg.Frontends {
		tgt, listener, err := f.listenerLocked(fe.BindAddress)
		if err != nil {
			return errors.Wrapf(err, "could not bind %q", fe.BindAddress)
		}

		switch tgt.Proto {
		case config.TCP:
			idleDuration, err := time.ParseDuration(fe.IdleDuration)
			if err != nil {
				return errors.Wrapf(err, "could not bind %q", fe.BindAddress)
			}

			b := f.mu.balancers[tgt]
			if b == nil {
				b = &balancer.Balancer{}
				f.mu.balancers[tgt] = b
			}

			if err := b.Configure(ctx, &fe.BackendPool); err != nil {
				return errors.Wrapf(err, "could not configure backend pool for %q", fe.BindAddress)
			}
			activeBalancers[tgt] = b

			s := loop.New(
				tcp.Proxy(listener.(*net.TCPListener), b, idleDuration),
				loop.WithMaxWorkers(fe.IncomingConnections),
				loop.WithWaitGroup(&f.wg),
			)
			toStart = append(toStart, s)
		}
	}

	// Kill any previous loops and create a new context for the next
	// generation of balancers and loops.
	if f.mu.cancelPreviousGeneration != nil {
		log.Print("stopping previous generation of server loops")
		f.mu.cancelPreviousGeneration()
	}
	ctx, f.mu.cancelPreviousGeneration = context.WithCancel(ctx)

	for _, b := range activeBalancers {
		go func(b *balancer.Balancer) {
			ticker := time.NewTicker(5 * time.Second)
			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
					// With tolerateErrors=true, this should log its own errors.
					if err := b.ResolvePool(ctx, true); err != nil {
						log.Printf("error while refreshing backend pool: %v", err)
					}
				}
			}
		}(b)
	}
	for _, s := range toStart {
		s.Start(ctx)
	}
	f.mu.config = cfg

	f.pruneLocked(activeBalancers)

	return nil
}

// MarshalJSON implements json.Marshaler and provides diagnostic information.
func (f *Frontend) MarshalJSON() ([]byte, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	payload := struct {
		BuildID   string
		Config    *config.Config
		Listeners map[string]*balancer.Balancer
	}{
		BuildID:   buildID,
		Config:    f.mu.config,
		Listeners: make(map[string]*balancer.Balancer),
	}
	for tgt, b := range f.mu.balancers {
		payload.Listeners[tgt.String()] = b
	}
	return json.Marshal(payload)
}

// Wait for all active connections to drain.
func (f *Frontend) Wait() {
	f.wg.Wait()
}

func (f *Frontend) listenerLocked(bindAddr string) (config.Target, io.Closer, error) {
	tgt, err := config.ParseTarget(bindAddr)
	if err != nil {
		return tgt, nil, err
	}
	existing := f.mu.listeners[tgt]
	if existing != nil {
		return tgt, existing, nil
	}

	switch tgt.Proto {
	case config.TCP:
		existing, err = net.ListenTCP("tcp", &net.TCPAddr{
			IP:   net.ParseIP(tgt.Host),
			Port: tgt.Port,
		})
	case config.UDP:
		existing, err = net.ListenUDP("udp", &net.UDPAddr{
			IP:   net.ParseIP(tgt.Host),
			Port: tgt.Port,
		})
	default:
		panic(errors.Errorf("listener for protocol %s unimplemented", tgt.Proto))
	}
	if err != nil {
		return tgt, nil, err
	}
	f.mu.listeners[tgt] = existing
	return tgt, existing, nil
}

// pruneLocked will delete any balancers and listeners not in the active
// target map.
func (f *Frontend) pruneLocked(active map[config.Target]*balancer.Balancer) {
	for tgt := range f.mu.balancers {
		if active[tgt] == nil {
			delete(f.mu.balancers, tgt)
		}
	}
	for tgt, closer := range f.mu.listeners {
		if active[tgt] == nil {
			_ = closer.Close()
			delete(f.mu.listeners, tgt)
		}
	}
}
