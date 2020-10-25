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

// Package balancer implements tiered pooling behavior across resolved
// backends.
package balancer

import (
	"context"
	"fmt"
	"log"
	"math"
	"net"
	"sync"

	"github.com/bobvawter/iaido/pkg/config"
	"github.com/bobvawter/iaido/pkg/pool"
	"github.com/pkg/errors"
	"gopkg.in/yaml.v3"
)

// BackendToken is a typesafe wrapper around pool.Token.
type BackendToken struct {
	*pool.Token
	balancer *Balancer
}

// Balancer returns the owner.
func (t *BackendToken) Balancer() *Balancer {
	return t.balancer
}

// Entry provides type safety around Token.Entry.
func (t *BackendToken) Entry() *Backend {
	ret, _ := t.Token.Entry().(*Backend)
	return ret
}

// A Balancer implements pooling behavior across resolved backends.
type Balancer struct {
	p  pool.Pool
	mu struct {
		sync.Mutex
		cfg *config.BackendPool
	}
}

// BestAvailableTier returns the tier of the backend that will likely
// be returned to service the next incoming connection.
func (b *Balancer) BestAvailableTier() int {
	t := b.p.Peek()
	if t == nil {
		return math.MaxInt64
	}
	return t.Entry().Tier()
}

// Config returns the currently-active configuration.
func (b *Balancer) Config() *config.BackendPool {
	b.mu.Lock()
	ret := b.mu.cfg
	b.mu.Unlock()
	return ret
}

// Configure will initialize or update the pool configuration and
// attempt to resolve the targets.
func (b *Balancer) Configure(ctx context.Context, cfg config.BackendPool, tolerateErrors bool) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.mu.cfg = &cfg

	// Try to retain existing Backend objects across refreshes
	// to retain metadata / metrics.
	key := func(addr net.Addr) string {
		return fmt.Sprintf("%s:%s", addr.Network(), addr.String())
	}
	canonicalBackends := make(map[string]*Backend, b.p.Len())
	for _, x := range b.p.All() {
		backend := x.(*Backend)
		canonicalBackends[key(backend.addr)] = backend
	}

	backendsInUse := make(map[*Backend]bool)

	for tierIdx, tier := range cfg.Tiers {
		for _, target := range tier.Targets {
			addrs, err := target.Resolve(ctx)
			if err != nil {
				if tolerateErrors {
					log.Printf("unable to resolve %q: %v", target, err)
				} else {
					return errors.Wrapf(err, "could not resolve target %q", target)
				}
			}

			for _, addr := range addrs {
				backend := canonicalBackends[key(addr)]
				if backend == nil {
					backend = &Backend{addr: addr}
					canonicalBackends[key(addr)] = backend
				}
				backendsInUse[backend] = true

				// (Re-)configure the backend.
				backend.mu.Lock()
				backend.mu.dialTimeout = tier.DialFailureTimeout
				backend.mu.forcePromotionAfter = tier.ForcePromotionAfter
				if target.Disabled {
					backend.mu.maxConnections = 0
				} else if tier.MaxBackendConnections == 0 {
					// Unconfigured means no limit.
					backend.mu.maxConnections = math.MaxInt64
				} else {
					backend.mu.maxConnections = tier.MaxBackendConnections
				}
				backend.mu.tier = tierIdx
				backend.mu.Unlock()
			}
		}
	}

	for backend := range backendsInUse {
		b.p.Add(backend)
	}

	for _, backend := range canonicalBackends {
		if !backendsInUse[backend] {
			log.Printf("removing backend %q", backend.addr)
			b.p.Remove(backend)

			// Force existing connections to go into drain mode.
			backend.mu.Lock()
			backend.mu.maxConnections = 0
			backend.mu.Unlock()
		}
	}

	b.Rebalance(ctx)

	return nil
}

// IsOverloaded returns true if the given backend should be considered
// to be in an overload situation (e.g. when draining).
func (b *Balancer) IsOverloaded(backend *Backend) bool {
	return b.p.IsOverloaded(backend)
}

// MarshalYAML implements yaml.Marshaler and provides a diagnostic
// view of the Balancer.
func (b *Balancer) MarshalYAML() (interface{}, error) {
	payload := struct {
		Pool   *pool.Pool
		Config *config.BackendPool
	}{
		Pool:   &b.p,
		Config: b.Config(),
	}
	return payload, nil
}

// Pick returns the next best choice from the pool or nil if one is not
// available.
func (b *Balancer) Pick() *BackendToken {
	t := b.p.Pick()
	if t == nil {
		return nil
	}
	return &BackendToken{t, b}
}

// Rebalance will rebalance the underlying pool.
func (b *Balancer) Rebalance(context.Context) {
	b.p.Rebalance()
}

func (b *Balancer) String() string {
	bytes, _ := yaml.Marshal(b)
	return string(bytes)
}

// Wait blocks until a Backend can be returned or the context is
// canceled.
func (b *Balancer) Wait(ctx context.Context) (*BackendToken, error) {
	ret, err := b.p.Wait(ctx, 1)
	if ret != nil {
		return &BackendToken{ret, b}, nil
	}
	return nil, err
}
