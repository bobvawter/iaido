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
	"encoding/json"
	"time"

	"io"

	"github.com/pkg/errors"
)

// Config is the top-level JSON-compatible configuration.
type Config struct {
	Frontends []Frontend
}

// ParseConfig parses and validates a serialized JSON representation of
// a Config.
func ParseConfig(reader io.Reader) (*Config, error) {
	ret := &Config{}
	d := json.NewDecoder(reader)
	d.DisallowUnknownFields()
	if err := d.Decode(ret); err != nil {
		return nil, err
	}
	return ret, ret.Validate()
}

// Validate checks the value.
func (c *Config) Validate() error {
	for i := range c.Frontends {
		if err := c.Frontends[i].Validate(); err != nil {
			return errors.Wrapf(err, "Frontends[%d]", i)
		}
	}
	return nil
}

// Frontend represents an active proxy frontend.
type Frontend struct {
	BackendPool         BackendPool
	BindAddress         string
	IncomingConnections int
	IdleDuration        string
}

// Validate checks the value.
func (f *Frontend) Validate() error {
	if f.BindAddress == "" {
		f.BindAddress = ":0"
	} else if _, err := ParseTarget(f.BindAddress); err != nil {
		return errors.Wrapf(err, "bad BindAddress")
	}
	if f.IncomingConnections == 0 {
		f.IncomingConnections = 1024
	}
	if f.IdleDuration == "" {
		f.IdleDuration = "1h"
	} else if _, err := time.ParseDuration(f.IdleDuration); err != nil {
		return errors.Wrapf(err, "bad IdleDuration")
	}
	return f.BackendPool.Validate()
}

type BackendPool struct {
	DialFailureTimeout    string
	MaxBackendConnections int
	Tiers                 []Tier
}

// Validate checks the value.
func (b *BackendPool) Validate() error {
	if len(b.Tiers) == 0 {
		return errors.New("Tiers must not be empty")
	}
	if b.DialFailureTimeout == "" {
		b.DialFailureTimeout = "30s"
	} else if _, err := time.ParseDuration(b.DialFailureTimeout); err != nil {
		return errors.Wrapf(err, "bad DialFailureTimeout")
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
	Targets []Target
}

// Validate checks the value.
func (t *Tier) Validate() error {
	for i := range t.Targets {
		if err := t.Targets[i].Validate(); err != nil {
			return errors.Wrapf(err, "Targets[%d]", i)
		}
	}
	return nil
}
