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
	"log"
	"math/rand"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/bobvawter/iaido/pkg/pool"
	"github.com/stretchr/testify/assert"
	"gopkg.in/yaml.v3"
)

type trivial struct {
	disabled bool
	name     string
	max      int
	tier     int
}

func (t *trivial) MarshalYAML() (interface{}, error) {
	return t.name, nil
}

func (t *trivial) MaxLoad() int {
	if t.disabled {
		return 0
	}
	if t.max == 0 {
		return 1
	}
	return t.max
}

func (t *trivial) String() string {
	return t.name
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
}

func TestRefCount(t *testing.T) {
	a := assert.New(t)

	// Create a pool with one entry.
	var p pool.Pool
	e := &trivial{}

	a.Equal(0, p.Load(e))
	p.Add(e)
	a.Equal(0, p.Load(e))

	// It should be possible to Pick the entry out.
	t0 := p.Pick()
	a.NotNil(t0)
	a.Same(e, t0.Entry())
	a.Equal(1, p.Load(e))

	// We can't pick it again until it's released.
	a.Nil(p.Pick())

	// Peeking is OK, however
	a.Same(e, p.Peek().Entry())

	a.True(t0.Release())
	// Check over-release.
	a.False(t0.Release())
	a.Nil(t0.Entry())
	a.Equal(0, p.Load(e))

	t1 := p.Pick()
	a.NotNil(t1)

	a.Nil(t0.Entry())
	a.Same(e, t1.Entry())
	a.Equal(1, p.Load(e))

	// Remove the entry and ensure that an issued Token still works
	// and that it can still be released, which will be a no-op.
	p.Remove(e)
	a.Same(e, t1.Entry())
	a.Equal(0, p.Load(e))
	t1.Release()
	a.Nil(p.Pick())
	a.Equal(0, p.Load(e))
}

func TestEmptyPoolWait(t *testing.T) {
	a := assert.New(t)
	var p pool.Pool
	a.Nil(p.Pick())
	a.Equal(0, p.Len())

	ch := make(chan *pool.Token)
	go func() {
		picked, err := p.Wait(context.Background(), 1)
		a.NoError(err)
		a.NotNil(picked)
		ch <- picked
	}()

	// Try with an already-canceled context.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	picked, err := p.Wait(ctx, 1)
	a.Nil(picked)
	a.True(errors.Is(err, context.Canceled))

	e := &trivial{}
	p.Add(e)
	log.Print("ADDED")

	select {
	case picked := <-ch:
		a.Same(e, picked.Entry())
	case <-time.NewTimer(10 * time.Minute).C:
		a.Fail("timeout")
	}
}

func TestLoadDistribution(t *testing.T) {
	a := assert.New(t)
	const count = 100
	const minLoad = 2

	loads := make([]int, count)
	var p pool.Pool
	for i := 0; i < count; i++ {
		p.Add(&trivial{name: strconv.Itoa(i + minLoad), max: i + minLoad})
		loads[i] = i + minLoad
	}

	rand.Shuffle(count, func(i, j int) {
		loads[i], loads[j] = loads[j], loads[i]
	})

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var wg sync.WaitGroup
	wg.Add(count)
	for i := 0; i < count; i++ {
		go func(idx int) {
			token, err := p.Wait(ctx, loads[idx])
			if a.NoError(err) {
				a.True(token.Release())
			}
			wg.Done()
		}(i)
	}
	wg.Wait()
}

func TestMarshalYAML(t *testing.T) {
	a := assert.New(t)

	e0 := &trivial{name: "e0"}
	e1 := &trivial{name: "e1"}

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

	a.Same(e0, p.Pick().Entry())

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
		t := p.Pick()
		a.Same(e, t.Entry())
		t.Release()
		a.Nil(t.Entry())
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

	// Pick() will only ever take an entry out of rotation, so we need
	// an explicit Rebalance() call here to kick all of the entries.
	p.Rebalance()

	checkRoundRobin(a, &p, tier0)
	checkRoundRobin(a, &p, tier0)
	checkRoundRobin(a, &p, tier0)
}

func checkRoundRobin(a *assert.Assertions, p *pool.Pool, expected []*trivial) {
	seen := make(map[*trivial]bool, len(expected))

	for range expected {
		t := p.Pick()
		found := t.Entry().(*trivial)
		a.Contains(expected, found)
		a.Falsef(seen[found], "saw %s again", found)
		seen[found] = true
		t.Release()
	}
}
