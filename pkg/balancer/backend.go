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
	"encoding/json"
	"log"
	"math/rand"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/pkg/errors"
)

// Key is a typesafe Context key.
type Key int

const (
	// KeyBackend is a context key that can retrieve a *Backend.
	KeyBackend Key = iota + 1
)

// A Backend creates connections to one specific backend (e.g. a TCP
// connection to a specific IP address).
type Backend struct {
	addr net.Addr
	mu   struct {
		sync.RWMutex
		dialTimeout time.Duration
		// This field can be set via configuration to pull a backend from
		// service.
		disabled        bool
		disableDuration time.Duration
		disabledUntil   time.Time
		maxConnections  int
		tier            int
	}
	atomic struct {
		openConns int32
	}
}

// Dial opens a connection to the backend and invokes the given callback.
// The number of concurrently-active session is available from Load().
func (b *Backend) Dial(ctx context.Context, fn func(context.Context, net.Conn) error) (shouldRetry bool, err error) {
	numConns := int(atomic.AddInt32(&b.atomic.openConns, 1))
	defer atomic.AddInt32(&b.atomic.openConns, -1)

	// Only hold the lock long enough to get the information needed
	// to set up the connection (or not).
	b.mu.RLock()
	dialTimeout := b.mu.dialTimeout
	maxConns := b.mu.maxConnections
	b.mu.RUnlock()

	if maxConns != 0 && numConns > maxConns {
		return true, errors.Errorf("reached maximum connection count of %d", maxConns)
	}

	conn, err := net.Dial(b.addr.Network(), b.addr.String())
	if err != nil {
		b.DisableFor(dialTimeout)
		return true, err
	}
	defer conn.Close()

	return false, fn(context.WithValue(ctx, KeyBackend, b), conn)
}

// DisableFor takes the Backend out of rotation for the given duration.
func (b *Backend) DisableFor(d time.Duration) {
	when := time.Now().Add(d)
	log.Printf("disabling %s until %s", b.addr, when)
	b.mu.Lock()
	b.mu.disabledUntil = when
	b.mu.Unlock()
}

// Disabled implements pool.Entry and will take the backend ouf of
// rotation due to calls to DisableFor or if ShedLoad is true.
func (b *Backend) Disabled() bool {
	if b.ShedLoad() {
		return true
	}

	b.mu.RLock()
	defer b.mu.RUnlock()

	when := b.mu.disabledUntil
	if when.IsZero() {
		return false
	}
	return time.Now().Before(when)
}

// Load implements pool.Entry and reflects the number of concurrent
// calls to Dial().
func (b *Backend) Load() int {
	return int(atomic.LoadInt32(&b.atomic.openConns))
}

// MarshalJSON implements json.Marshaler and provides a diagnostic
// view of the Backend.
func (b *Backend) MarshalJSON() ([]byte, error) {
	payload := struct {
		Address         string
		Disabled        bool
		DisabledUntil   *time.Time
		OpenConnections int
		Tier            int
	}{
		Address:         b.addr.String(),
		Disabled:        b.Disabled(),
		OpenConnections: b.Load(),
		Tier:            b.Tier(),
	}
	if b.Disabled() {
		b.mu.RLock()
		when := b.mu.disabledUntil
		b.mu.RUnlock()
		payload.DisabledUntil = &when
	}
	return json.Marshal(payload)
}

// ShedLoad provides advice to flows through the Backend as to whether
// or not they should attempt to aggressively remove load. The value
// returned is probabilistic, reflecting the amount of overload,
// relative to the desired number of concurrent users of the Backend.
func (b *Backend) ShedLoad() bool {
	b.mu.RLock()
	disabled := b.mu.disabled
	max := int32(b.mu.maxConnections)
	b.mu.RUnlock()

	if disabled {
		return true
	}

	if max == 0 {
		return false
	}

	open := atomic.LoadInt32(&b.atomic.openConns)
	if open <= max {
		return false
	}

	if open >= 2*max {
		return true
	}

	prob := float32(open-max) / float32(max)
	return rand.Float32() <= prob
}

func (b *Backend) String() string {
	bytes, _ := b.MarshalJSON()
	return string(bytes)
}

// Tier implements pool.Entry and reflects the overall priority of the
// Backend.
func (b *Backend) Tier() int {
	b.mu.RLock()
	ret := b.mu.tier
	b.mu.RUnlock()
	return ret
}
