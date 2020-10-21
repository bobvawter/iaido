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
	Activity func(read, written int)

	// The amount of time to wait for an idle connection before
	// checking for Context cancellation.
	Linger time.Duration
}

// Copy executes the given proxy operation.
//
// This method will run in a tight loop as long as there is incoming
// data to transfer.  The Context state is polled only once all
// incoming data has been drained.
func (c *Copier) Copy(ctx context.Context) error {
	// Use a pre-allocated buffer.
	buffer := thePool.Get()
	defer thePool.Put(buffer)

	linger := c.Linger
	if linger == 0 {
		linger = time.Second
	}

	for running := true; running; {
		if err := c.From.SetReadDeadline(time.Now().Add(linger)); err != nil {
			return errors.Wrapf(err, "error setting read deadline %s", c)
		}

		read, err := c.From.Read(buffer)
		if err != nil {
			if errors.Is(err, io.EOF) {
				running = false
			} else if op, ok := err.(*net.OpError); ok && op.Timeout() {
				// If we've hit the read timeout (like we'd expect on an
				// idle connection), we want to check for context
				// cancellation.
				select {
				case <-ctx.Done():
					return ctx.Err()
				default:
					continue
				}
			} else {
				return errors.Wrapf(err, "error reading %s", c)
			}
		}

		if c.Activity != nil {
			c.Activity(read, 0)
		}

		for idx := 0; idx < read; {
			if err := c.To.SetWriteDeadline(time.Now().Add(linger)); err != nil {
				return errors.Wrapf(err, "error setting write deadline %s", c)
			}
			written, err := c.To.Write(buffer[idx:read])
			if err != nil {
				return errors.Wrapf(err, "error writing %s", c)
			}
			idx += written
			if c.Activity != nil {
				c.Activity(0, written)
			}
		}
	}
	return nil
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
		Linger: c.Linger,
	}
	return json.Marshal(payload)
}

func (c *Copier) String() string {
	data, _ := c.MarshalJSON()
	return string(data)
}
