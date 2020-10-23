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
	"bytes"
	"context"
	"io"
	"net"
	"testing"

	"sync/atomic"

	"github.com/bobvawter/iaido/pkg/latch"
	"github.com/bobvawter/iaido/pkg/loop"
	"github.com/stretchr/testify/assert"
)

func TestCapture(t *testing.T) {
	a := assert.New(t)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var ctr byteCounter
	addr, opt, err := Capture(ctx, &ctr)
	if !a.NoError(err) {
		return
	}

	l := latch.New()
	loop.New(opt, loop.WithLatch(l)).Start(ctx)

	conn, err := net.Dial(addr.Network(), addr.String())
	if !a.NoError(err) {
		return
	}

	const size = 1024 * 1024
	_, err = io.Copy(conn, bytes.NewReader(make([]byte, size)))
	a.NoError(err)
	a.NoError(conn.Close())
	l.Wait()
	a.Equal(uint64(size), atomic.LoadUint64(&ctr.count))
}

type byteCounter struct {
	count uint64
}

func (c *byteCounter) Write(data []byte) (int, error) {
	l := len(data)
	atomic.AddUint64(&c.count, uint64(l))
	return l, nil
}
