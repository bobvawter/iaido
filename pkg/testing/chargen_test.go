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

package testing

import (
	"context"
	"io"
	"net"
	"testing"

	"time"

	"github.com/bobvawter/iaido/pkg/loop"
	"github.com/stretchr/testify/assert"
	"golang.org/x/sync/errgroup"
)

func Test(t *testing.T) {
	const count = 1024
	a := assert.New(t)

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	addr, opt, err := CharGen(ctx, count)
	if !a.NoError(err) {
		return
	}
	loop.New(opt).Start(ctx)

	runOneTest(a, addr, count)
}

func TestParallel(t *testing.T) {
	const count = 1024
	const workers = 16
	const requests = 128

	a := assert.New(t)

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	addr, opt, err := CharGen(ctx, count)
	if !a.NoError(err) {
		return
	}
	loop.New(opt).Start(ctx)

	group, _ := errgroup.WithContext(ctx)
	for i := 0; i < workers; i++ {
		group.Go(func() error {
			for i := 0; i < requests; i++ {
				if err := runOneTest(a, addr, count); err != nil {
					return err
				}
			}
			return nil
		})
	}
	a.NoError(group.Wait())
}

func runOneTest(a *assert.Assertions, addr net.Addr, count int) error {
	conn, err := net.Dial(addr.Network(), addr.String())
	if err != nil {
		return err
	}
	defer conn.Close()

	buf := make([]byte, 2048)
	read, err := io.ReadAtLeast(conn, buf, count)
	a.Equal(count, read)
	return err
}
