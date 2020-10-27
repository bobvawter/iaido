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
	"log"
	"net"
	"sync"
	"time"

	"gopkg.in/yaml.v3"
)

// Key is a typesafe Context key.
type Key int

const (
	// KeyBackendToken retrieves a *BackendToken from a Context.
	KeyBackendToken Key = iota + 1
)

// A Backend creates connections to one specific backend (e.g. a TCP
// connection to a specific IP address).
type Backend struct {
	addr net.Addr
	mu   struct {
		sync.RWMutex
		dialTimeout         time.Duration
		disableDuration     time.Duration
		disabledUntil       time.Time
		forcePromotionAfter time.Duration
		lastLatency         time.Duration
		maxConnections      int
		tier                int
	}
}

// Addr returns the Backend address.
func (b *Backend) Addr() net.Addr {
	return b.addr
}

// AssignTier (re-)assigns a Backend to a tier based on a latency metric.
func (b *Backend) AssignTier(latency time.Duration, tier int) {
	b.mu.Lock()
	b.mu.lastLatency = latency
	b.mu.tier = tier
	b.mu.Unlock()
}

// Disable takes the Backend out of rotation for the configured duration.
func (b *Backend) Disable() {
	b.mu.Lock()
	when := time.Now().Add(b.mu.disableDuration)
	b.mu.disabledUntil = when
	b.mu.Unlock()
	log.Printf("disabling %s until %s", b.addr, when)
}

// ForcePromotionAfter indicates that clients should be forcefully
// disconnected from this backend if a better choice is available.
func (b *Backend) ForcePromotionAfter() time.Duration {
	b.mu.RLock()
	ret := b.mu.forcePromotionAfter
	b.mu.RUnlock()
	return ret
}

// MaxLoad implements pool.Entry and reflects the number of concurrent
// calls to Dial().
func (b *Backend) MaxLoad() int {
	b.mu.RLock()
	defer b.mu.RUnlock()

	if time.Now().Before(b.mu.disabledUntil) {
		return 0
	}

	return b.mu.maxConnections
}

// MarshalYAML implements yaml.Marshaler interfare and provides a
// diagnostic view of the Backend.
func (b *Backend) MarshalYAML() (interface{}, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	payload := struct {
		Address              string
		DisabledUntil        time.Time     `yaml:"disabledUntil"`
		ForcePromotionsAfter time.Duration `yaml:"forcePromotionsAfter"`
		Latency              time.Duration `yaml:"latency"`
		MaxConnections       int           `yaml:"maxConnections"`
		Tier                 int           `yaml:"tier"`
	}{
		Address:              b.addr.String(),
		DisabledUntil:        b.mu.disabledUntil,
		ForcePromotionsAfter: b.mu.forcePromotionAfter,
		Latency:              b.mu.lastLatency,
		MaxConnections:       b.mu.maxConnections,
		Tier:                 b.mu.tier,
	}
	return payload, nil
}

func (b *Backend) String() string {
	bytes, _ := yaml.Marshal(b)
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
