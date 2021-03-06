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

package balancer

import (
	"context"
	"errors"
	"io/ioutil"
	"log"
	"net"
	"testing"
	"time"

	"github.com/bobvawter/iaido/pkg/config"
	"github.com/bobvawter/iaido/pkg/loop"
	it "github.com/bobvawter/iaido/pkg/testing"
	"github.com/stretchr/testify/assert"
)

// Verify basic plumbing.
func TestBalancerEndToEnd(t *testing.T) {
	const backends = 16
	const requestCount = 128
	a := assert.New(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	counters := make([]func() uint64, backends)
	targets := make([]config.Target, backends)

	for i := range targets {
		var opt loop.Option
		opt, counters[i] = loop.WithConnectionCounter()
		addr, cgOpt, err := it.CharGen(ctx, 4096)
		if !a.NoError(err) {
			return
		}
		loop.New(cgOpt, opt).Start(ctx)

		targets[i], err = config.ParseTarget(addr.String())
		if !a.NoError(err) {
			return
		}
	}

	cfg := config.BackendPool{
		Tiers: []config.Tier{{
			DialFailureTimeout: 10 * time.Second,
			Targets:            targets,
		}},
	}
	a.NoError(cfg.Validate())

	b := &Balancer{}
	a.NoError(b.Configure(ctx, cfg, false))
	b.Rebalance(ctx)

	for i := 0; i < requestCount; i++ {
		token := b.Pick()
		checkBackend(a, token.Entry())
		token.Release()
	}

	token, err := b.Wait(ctx)
	a.NotNil(token)
	a.NoError(err)

	// Ensure that all backends have received traffic
	for idx, fn := range counters {
		a.Greaterf(fn(), uint64(0), "index %d", idx)
		log.Printf("Backend %d got %d requests", idx, fn())
	}

	t.Log(b)

	// Disable all backends and verify behavior
	for i := range cfg.Tiers[0].Targets {
		cfg.Tiers[0].Targets[i].Disabled = true
	}
	a.NoError(b.Configure(ctx, cfg, false))
	a.Nil(b.Pick())
	shortCtx, cancel2 := context.WithTimeout(ctx, time.Millisecond)
	defer cancel2()
	_, err = b.Wait(shortCtx)
	a.True(errors.Is(err, context.DeadlineExceeded))
}

// Ensure that Backend instances are preserved across reconfigurations.
func TestBalancerReconfigure(t *testing.T) {
	const backends = 16
	a := assert.New(t)
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	cfg := config.BackendPool{
		Tiers: []config.Tier{{
			DialFailureTimeout: 10 * time.Second,
		}},
	}

	b := &Balancer{}
	a.NoError(b.Configure(ctx, cfg, false))
	b.Rebalance(ctx)

	for i := 0; i < backends; i++ {
		cfg.Tiers[0].Targets = append(cfg.Tiers[0].Targets, config.Target{Hosts: []string{"127.0.0.1"}, Port: i, Proto: config.TCP})
		a.NoError(b.Configure(ctx, cfg, false))
		a.Equal(len(cfg.Tiers[0].Targets), b.p.Len())
	}

	for len(cfg.Tiers[0].Targets) > 0 {
		cfg.Tiers[0].Targets = cfg.Tiers[0].Targets[:len(cfg.Tiers[0].Targets)-1]
		a.NoError(b.Configure(ctx, cfg, false))
		a.Equal(len(cfg.Tiers[0].Targets), b.p.Len())
	}
}

func checkBackend(a *assert.Assertions, b *Backend) {
	conn, err := net.Dial(b.Addr().Network(), b.Addr().String())
	if !a.NoError(err) {
		return
	}

	buf, err := ioutil.ReadAll(conn)
	a.NoError(err, b)
	a.Len(buf, 4096, b)
}
