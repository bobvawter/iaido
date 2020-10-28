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

// Package config contains the Iaido configuration format.
package config

import (
	"io"
	"time"

	"github.com/pkg/errors"
	"gopkg.in/yaml.v3"
)

// Config is the top-level YAML-compatible configuration.
type Config struct {
	// A local address to bind a diagnostic HTTP endpoint to.
	DiagAddr    string        `yaml:"diagAddr"`
	Frontends   []Frontend    `yaml:"frontends"`
	GracePeriod time.Duration `yaml:"gracePeriod"`
}

// DecodeConfig parses and validates a serialized YAML representation of
// a Config.
func DecodeConfig(reader io.Reader, cfg *Config) error {
	d := yaml.NewDecoder(reader)
	d.KnownFields(true)
	if err := d.Decode(cfg); err != nil {
		return err
	}
	return cfg.Validate()
}

// Validate checks the value.
func (c *Config) Validate() error {
	for i := range c.Frontends {
		if err := c.Frontends[i].Validate(); err != nil {
			return errors.Wrapf(err, "Frontends[%d]", i)
		}
	}
	if c.GracePeriod == 0 {
		c.GracePeriod = 5 * time.Second
	}
	return nil
}

// Frontend represents an active proxy frontend.
type Frontend struct {
	BackendPool BackendPool `yaml:"backendPool"`
	// The local address to listen for incoming connections on.
	BindAddress string `yaml:"bindAddress"`
	// The time after which an idle connection should be pruned.
	IdleDuration time.Duration `yaml:"idleDuration"`
	// How often to rebalance backends across tiers.
	RebalanceDuration time.Duration `yaml:"rebalanceDuration"`
}

// Validate checks the value.
func (f *Frontend) Validate() error {
	if f.BindAddress == "" {
		f.BindAddress = ":0"
	} else if _, err := ParseTarget(f.BindAddress); err != nil {
		return errors.Wrapf(err, "bad BindAddress")
	}
	if f.IdleDuration == 0 {
		f.IdleDuration = time.Hour
	}
	if f.RebalanceDuration == 0 {
		f.RebalanceDuration = 10 * time.Second
	}
	return f.BackendPool.Validate()
}

// BackendPool represents the actual machines to connect to.
type BackendPool struct {
	// LatencyBucket allows backends to be assigned to tiers
	// automatically. Backends are grouped such that the latencies of
	// the fastest and slowest entries in a tier are no greater than the
	// given value.
	//
	// Automatic latency detection can be disabled by setting this to a
	// negative value.
	LatencyBucket time.Duration `yaml:"latencyBucket"`
	// How often connections within the pool should be evaluated for
	// over-load conditions, promotion to a higher tier, etc.
	// Longer values provide better damping of behavior, at the cost
	// of simply taking more time to effect configuration changes.
	MaintenanceTime time.Duration `yaml:"maintenanceTime"`
	// Backends are arranged in tiers with a "fill and spill" behavior.
	Tiers []Tier `yaml:"tiers"`
}

// Validate checks the value.
func (b *BackendPool) Validate() error {
	if b.LatencyBucket == 0 {
		b.LatencyBucket = 10 * time.Millisecond
	}
	if b.MaintenanceTime == 0 {
		b.MaintenanceTime = 30 * time.Second
	}
	if len(b.Tiers) == 0 {
		return errors.New("Tiers must not be empty")
	}
	for i := range b.Tiers {
		if err := b.Tiers[i].Validate(); err != nil {
			return errors.Wrapf(err, "Tiers[%d]", i)
		}
	}
	return nil
}

// Tier contains the targets that the frontend will forward connections to.
type Tier struct {
	// How long to disable a backend if a connection cannot be established.
	DialFailureTimeout time.Duration `yaml:"dialFailureTimeout"`
	// If non-zero, connections to this tier will be forcefully
	// disconnected if a backend in a closer tier becomes available.
	ForcePromotionAfter time.Duration `yaml:"forcePromotionAfter"`
	// The maximum number of connections to make to any given backend.
	MaxBackendConnections int      `yaml:"maxBackendConnections"`
	Targets               []Target `yaml:"targets"`
}

// Validate checks the value.
func (t *Tier) Validate() error {
	if t.DialFailureTimeout == 0 {
		t.DialFailureTimeout = time.Second
	}
	for i := range t.Targets {
		if err := t.Targets[i].Validate(); err != nil {
			return errors.Wrapf(err, "Targets[%d]", i)
		}
	}
	return nil
}
