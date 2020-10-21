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
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/bobvawter/iaido/pkg/config"
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
	addr   net.Addr
	config func() *config.BackendPool
	tier   int
	mu     struct {
		sync.Mutex
		disabledUntil time.Time
	}
	atomic struct {
		openConns int32
	}
}

// DisableFor takes the Backend out of rotation for the given duration.
func (b *Backend) DisableFor(d time.Duration) {
	when := time.Now().Add(d)
	b.mu.Lock()
	b.mu.disabledUntil = when
	b.mu.Unlock()
	log.Printf("disabling %s until %s", b.addr, when)
}

// Dial opens a connection to the backend and invokes the given callback.
// The number of concurrently-active session is available from Load().
func (b *Backend) Dial(ctx context.Context, fn func(context.Context, net.Conn) error) (shouldRetry bool, err error) {
	numConns := int(atomic.AddInt32(&b.atomic.openConns, 1))
	defer atomic.AddInt32(&b.atomic.openConns, -1)

	maxConns := b.config().MaxBackendConnections
	if maxConns != 0 && numConns > maxConns {
		return true, errors.Errorf("reached maximum connection count of %d", maxConns)
	}

	conn, err := net.Dial(b.addr.Network(), b.addr.String())
	if err != nil {
		timeout, _ := time.ParseDuration(b.config().DialFailureTimeout)
		b.DisableFor(timeout)
		return true, err
	}
	defer conn.Close()

	ctx = context.WithValue(ctx, KeyBackend, b)
	return false, fn(ctx, conn)
}

// Disabled implements pool.Entry and will take the backend ouf of
// rotation due to calls to DisableFor or if the number of connections
// equals or exceeds the maximum number permitted to the backend.
func (b *Backend) Disabled() bool {
	maxConns := b.config().MaxBackendConnections
	if maxConns > 0 && int(atomic.LoadInt32(&b.atomic.openConns)) >= maxConns {
		return true
	}

	b.mu.Lock()
	when := b.mu.disabledUntil
	b.mu.Unlock()

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
		DisabledUntil   *time.Time
		OpenConnections int
		Tier            int
	}{
		Address:         b.addr.String(),
		OpenConnections: b.Load(),
		Tier:            b.Tier(),
	}
	if b.Disabled() {
		b.mu.Lock()
		when := b.mu.disabledUntil
		b.mu.Unlock()
		payload.DisabledUntil = &when
	}
	return json.Marshal(payload)
}

func (b *Backend) String() string {
	bytes, _ := b.MarshalJSON()
	return string(bytes)
}

// Tier implements pool.Entry and reflects the overall priority of the
// Backend.
func (b *Backend) Tier() int {
	return b.tier
}
