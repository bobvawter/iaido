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
	"encoding/json"
	"fmt"
	"log"
	"net"
	"sync/atomic"

	"github.com/bobvawter/iaido/pkg/config"
	"github.com/bobvawter/iaido/pkg/pool"
	"github.com/pkg/errors"
)

// A Balancer implements pooling behavior across resolved backends.
type Balancer struct {
	p   pool.Pool
	cfg atomic.Value
}

// Config returns a copy of the currently-active configuration.
func (b *Balancer) Config() *config.BackendPool {
	return b.cfg.Load().(*config.BackendPool)
}

// Configure will initialize or update the pool configuration and
// attempt to resolve the targets.
func (b *Balancer) Configure(ctx context.Context, cfg *config.BackendPool, tolerateErrors bool) error {
	b.cfg.Store(cfg)
	entryCount := b.p.Len()

	// Try to retain existing Backend objects across refreshes
	// to retain metadata / metrics.
	key := func(addr net.Addr) string {
		return fmt.Sprintf("%s:%s", addr.Network(), addr.String())
	}
	canonicalBackends := make(map[string]*Backend, entryCount)
	for _, x := range b.p.All() {
		backend := x.(*Backend)
		canonicalBackends[key(backend.addr)] = backend
	}

	backendsInUse := make(map[*Backend]bool)

	for tierIdx, tier := range b.Config().Tiers {
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
					backend = &Backend{
						addr:   addr,
						config: b.Config,
						tier:   tierIdx,
					}
					canonicalBackends[key(addr)] = backend
				} else {
					// Support moving backends between tiers.
					backend.tier = tierIdx
				}
				backendsInUse[backend] = true
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
		}
	}

	b.p.Rebalance()

	return nil
}

// MarshalJSON implements json.Marshaler and provides a diagnostic
// view of the Balancer.
func (b *Balancer) MarshalJSON() ([]byte, error) {
	payload := struct {
		Pool   *pool.Pool
		Config *config.BackendPool
	}{
		Pool:   &b.p,
		Config: b.Config(),
	}
	return json.Marshal(payload)
}

// Pick returns the next best choice from the pool or nil if one is not
// available.
func (b *Balancer) Pick() *Backend {
	ret, _ := b.p.Pick().(*Backend)
	return ret
}

func (b *Balancer) String() string {
	bytes, _ := b.MarshalJSON()
	return string(bytes)
}
