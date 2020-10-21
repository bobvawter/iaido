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
	"net"
	"testing"
	"time"

	"github.com/bobvawter/iaido/pkg/loop"
	"github.com/stretchr/testify/assert"
)

func TestIgnore(t *testing.T) {
	a := assert.New(t)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	addr, opt, err := Ignore()
	if !a.NoError(err) {
		return
	}
	loop.New(opt).Start(ctx)

	conn, err := net.Dial(addr.Network(), addr.String())
	if !a.NoError(err) {
		return
	}

	a.NoError(conn.SetDeadline(time.Now().Add(10 * time.Millisecond)))
	_, err = conn.Write(make([]byte, 10*1024*1024))
	if a.IsType(&net.OpError{}, err) {
		a.True(err.(*net.OpError).Timeout())
	}
}
