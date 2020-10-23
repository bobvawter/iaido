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

package pool_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/bobvawter/iaido/pkg/pool"
	"github.com/stretchr/testify/assert"
	"gopkg.in/yaml.v3"
)

type trivial struct {
	disabled bool
	name     string
	load     int
	tier     int
}

func (t *trivial) MarshalYAML() (interface{}, error) {
	return t.name, nil
}

func (t *trivial) String() string {
	return t.name
}

func (t *trivial) Disabled() bool {
	return t.disabled
}

func (t *trivial) Load() int {
	return t.load
}

func (t *trivial) Tier() int {
	return t.tier
}

var _ pool.Entry = &trivial{}

func TestAddIdempotent(t *testing.T) {
	a := assert.New(t)

	var p pool.Pool
	e := &trivial{}

	a.Equal(0, p.Len())
	p.Add(e)
	a.Equal(1, p.Len())
	p.Add(e)
	a.Equal(1, p.Len())

	ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond)
	defer cancel()
	picked, err := p.Wait(ctx)
	a.Same(e, picked)
	a.NoError(err)
}

func TestEmptyPoolWait(t *testing.T) {
	a := assert.New(t)
	var p pool.Pool
	a.Nil(p.Pick())
	a.Equal(0, p.Len())

	ch := make(chan pool.Entry, 1)
	go func() {
		picked, err := p.Wait(context.Background())
		a.NoError(err)
		a.NotNil(picked)
		ch <- picked
	}()

	ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond)
	defer cancel()
	picked, err := p.Wait(ctx)
	a.Nil(picked)
	a.True(errors.Is(err, context.DeadlineExceeded))

	p.Add(&trivial{})
	p.Rebalance()

	select {
	case picked := <-ch:
		a.NotNil(picked)
	case <-time.NewTimer(time.Second).C:
		a.Fail("timeout")
	}
}

func TestMarshalYAML(t *testing.T) {
	a := assert.New(t)

	e0 := &trivial{name: "e0", load: 2}
	e1 := &trivial{name: "e1", load: 1}

	var p pool.Pool
	p.Add(e0)
	p.Add(e1)
	p.Pick()
	p.Pick()

	var sb strings.Builder
	e := yaml.NewEncoder(&sb)
	a.NoError(e.Encode(&p))
	t.Log(sb.String())
}

func TestRemove(t *testing.T) {
	a := assert.New(t)

	e0 := &trivial{}
	e1 := &trivial{disabled: true}

	var p pool.Pool
	p.Add(e0)
	p.Add(e1)
	a.Equal(2, p.Len())
	a.Contains(p.All(), e0)
	a.Contains(p.All(), e1)

	a.Same(e0, p.Pick())

	p.Remove(e0)
	a.Nil(p.Pick())
	a.Equal(1, p.Len())

	p.Remove(e1)
	a.Nil(p.Pick())
	a.Equal(0, p.Len())
}

func TestSingletonPool(t *testing.T) {
	a := assert.New(t)

	e := &trivial{}
	var p pool.Pool
	p.Add(e)

	for i := 0; i < 100; i++ {
		a.Same(e, p.Pick())
	}
}

func TestOneTier(t *testing.T) {
	const entryCount = 128
	a := assert.New(t)

	var p pool.Pool

	tier0 := make([]*trivial, entryCount)
	for i := range tier0 {
		tier0[i] = &trivial{name: fmt.Sprintf("tier0-%d", i)}
		p.Add(tier0[i])
	}

	checkRoundRobin(a, &p, tier0)
	checkRoundRobin(a, &p, tier0)
	checkRoundRobin(a, &p, tier0)
}

func TestTwoTier(t *testing.T) {
	const entryCount = 128
	a := assert.New(t)

	var p pool.Pool

	tier0 := make([]*trivial, entryCount)
	for i := range tier0 {
		tier0[i] = &trivial{name: fmt.Sprintf("tier0-%d", i)}
		p.Add(tier0[i])
	}

	tier1 := make([]*trivial, entryCount)
	for i := range tier1 {
		tier1[i] = &trivial{name: fmt.Sprintf("tier1-%d", i), tier: 1}
		p.Add(tier1[i])
	}

	checkRoundRobin(a, &p, tier0)
	checkRoundRobin(a, &p, tier0)
	checkRoundRobin(a, &p, tier0)

	// Now, disable the top-tier entries and see that the next
	// tier is being chosen.
	for i := range tier0 {
		tier0[i].disabled = true
	}

	checkRoundRobin(a, &p, tier1)
	checkRoundRobin(a, &p, tier1)
	checkRoundRobin(a, &p, tier1)

	// Re-enable.
	for i := range tier0 {
		tier0[i].disabled = false
	}
	p.Rebalance()
	checkRoundRobin(a, &p, tier0)
	checkRoundRobin(a, &p, tier0)
	checkRoundRobin(a, &p, tier0)
}

func TestThreeTier(t *testing.T) {
	const entryCount = 128
	a := assert.New(t)

	var p pool.Pool

	tier0 := make([]*trivial, entryCount)
	for i := range tier0 {
		tier0[i] = &trivial{name: fmt.Sprintf("tier0-%d", i)}
		p.Add(tier0[i])
	}

	tier1 := make([]*trivial, entryCount)
	for i := range tier1 {
		tier1[i] = &trivial{name: fmt.Sprintf("tier1-%d", i), tier: 1}
		p.Add(tier1[i])
	}

	tier2 := make([]*trivial, entryCount)
	for i := range tier2 {
		tier2[i] = &trivial{name: fmt.Sprintf("tier2-%d", i), tier: 2}
		p.Add(tier2[i])
	}

	checkRoundRobin(a, &p, tier0)
	checkRoundRobin(a, &p, tier0)
	checkRoundRobin(a, &p, tier0)

	// Now, disable the top-tier entries and see that the next
	// tier is being chosen.
	for i := range tier0 {
		tier0[i].disabled = true
	}

	checkRoundRobin(a, &p, tier1)
	checkRoundRobin(a, &p, tier1)
	checkRoundRobin(a, &p, tier1)

	// Re-enable.
	for i := range tier0 {
		tier0[i].disabled = false
	}
	p.Rebalance()
	checkRoundRobin(a, &p, tier0)
	checkRoundRobin(a, &p, tier0)
	checkRoundRobin(a, &p, tier0)
}

func checkRoundRobin(a *assert.Assertions, p *pool.Pool, expected []*trivial) {
	seen := make(map[*trivial]bool, len(expected))

	for range expected {
		found := p.Pick().(*trivial)
		a.Contains(expected, found)
		a.Falsef(seen[found], "saw %s again", found)
		seen[found] = true
	}
}
