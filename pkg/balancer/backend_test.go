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
	"net"
	"testing"
	"time"

	"github.com/bobvawter/iaido/pkg/config"
	"github.com/bobvawter/iaido/pkg/loop"
	it "github.com/bobvawter/iaido/pkg/testing"
	"github.com/stretchr/testify/assert"
)

// Smoke test all of the methods in Backend.
func TestBackend(t *testing.T) {
	a := assert.New(t)
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	addr, opt, err := it.CharGen(4096)
	if !a.NoError(err) {
		return
	}
	loop.New(opt).Start(ctx)

	b := &Backend{
		addr: addr,
		config: func() *config.BackendPool {
			return &config.BackendPool{}
		},
		tier: 2,
	}
	a.Equal(0, b.Load())
	a.Equal(2, b.Tier())
	a.False(b.Disabled())
	t.Log(b)

	_, err = b.Dial(ctx, func(ctx context.Context, conn net.Conn) error {
		a.Same(b, ctx.Value(KeyBackend))
		a.Equal(1, b.Load())
		return nil
	})
	a.NoError(err)
	a.Equal(0, b.Load())

	b.DisableFor(time.Hour)
	a.True(b.Disabled())

	b.DisableFor(0)
	a.False(b.Disabled())
}

// Determine that a backend self-disables when it's unable to dial.
func TestBackendToNowhere(t *testing.T) {
	a := assert.New(t)
	b := &Backend{
		addr: &net.TCPAddr{
			IP:   []byte{127, 0, 0, 1},
			Port: 2,
		},
		config: func() *config.BackendPool {
			return &config.BackendPool{DialFailureTimeout: "1h"}
		},
	}

	retry, err := b.Dial(context.Background(), func(ctx context.Context, conn net.Conn) error {
		return nil
	})
	a.True(retry)
	a.NotNil(err)
	a.True(b.Disabled())
}
