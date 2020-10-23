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
	"testing"
	"time"

	"github.com/bobvawter/iaido/pkg/loop"
	it "github.com/bobvawter/iaido/pkg/testing"
	"github.com/stretchr/testify/assert"
)

// Smoke test all of the methods in Backend.
func TestBackend(t *testing.T) {
	a := assert.New(t)
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	addr, opt, err := it.CharGen(ctx, 4096)
	if !a.NoError(err) {
		return
	}
	loop.New(opt).Start(ctx)

	b := &Backend{
		addr: addr,
	}
	b.mu.maxConnections = 2
	b.mu.tier = 2
	a.Equal(2, b.Tier())
	a.Equal(2, b.MaxLoad())
	t.Log(b)

	b.DisableFor(time.Hour)
	a.Equal(0, b.MaxLoad())

	b.DisableFor(0)
	a.Equal(2, b.MaxLoad())
}
