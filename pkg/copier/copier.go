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

// Package copier provides a utility type for simplex proxy connections.
package copier

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"time"

	"github.com/pkg/errors"
)

// Copier implements a simplex proxy connection.
type Copier struct {
	From net.Conn
	To   net.Conn

	// Activity will be called if it is non-nil whenever traffic
	// has been copied.
	Activity func(written int64)

	// The period of time to wait before interrupting a copy to check
	// for Context cancellation.
	WakePeriod time.Duration
}

// Copy executes the given copy operation, but adds an occasional check
// for context cancellation.
func (c *Copier) Copy(ctx context.Context) error {
	if c.WakePeriod == 0 {
		c.WakePeriod = time.Second
	}
	for {
		now := time.Now()
		if err := c.To.SetWriteDeadline(now.Add(time.Minute)); err != nil {
			return errors.Wrapf(err, "could not set zero write deadline")
		}
		if err := c.From.SetReadDeadline(time.Now().Add(c.WakePeriod)); err != nil {
			return errors.Wrapf(err, "could not set read deadline")
		}
		bytes, err := io.Copy(c.To, c.From)
		if c.Activity != nil {
			c.Activity(bytes)
		}
		if op := (&net.OpError{}); errors.As(err, &op) && op.Timeout() {
			select {
			case <-ctx.Done():
				err = ctx.Err()
			default:
				continue
			}
		}
		return errors.Wrapf(err, "copy %s", c)
	}
}

// MarshalJSON implements json.Marshaler and provides diagnostic information.
func (c *Copier) MarshalJSON() ([]byte, error) {
	payload := struct {
		From   string
		To     string
		Linger time.Duration
	}{
		From:   c.From.RemoteAddr().String(),
		To:     c.To.RemoteAddr().String(),
		Linger: c.WakePeriod,
	}
	return json.Marshal(payload)
}

func (c *Copier) String() string {
	data, _ := c.MarshalJSON()
	return string(data)
}
