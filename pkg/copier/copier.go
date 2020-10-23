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
	"fmt"
	"io"
	"time"

	"net"

	"github.com/pkg/errors"
	"gopkg.in/yaml.v3"
)

// Copier implements a simplex proxy connection.
type Copier struct {
	// Activity will be called if it is non-nil whenever traffic
	// has been copied.
	Activity func(written int64)
	From     DeadlineReader
	To       DeadlineWriter
	// The period of time to wait before interrupting a copy to check
	// for Context cancellation or to report the number of bytes copied.
	WakePeriod time.Duration
}

// DeadlineReader is under pressure to perform.
type DeadlineReader interface {
	io.Reader
	SetReadDeadline(time.Time) error
}

// DeadlineWriter is under pressure to perform.
type DeadlineWriter interface {
	io.Writer
	SetWriteDeadline(time.Time) error
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
		// err will be nil here if the copy was successful.
		return errors.Wrapf(err, "copy %s", c)
	}
}

// MarshalYAML implements yaml.Marshaler and provides diagnostic information.
func (c *Copier) MarshalYAML() (interface{}, error) {
	payload := struct {
		From       string
		To         string
		WakePeriod time.Duration `yaml:"wakePeriod"`
	}{
		From:       fmt.Sprint(c.From),
		To:         fmt.Sprint(c.To),
		WakePeriod: c.WakePeriod,
	}
	return payload, nil
}

func (c *Copier) String() string {
	data, _ := yaml.Marshal(c)
	return string(data)
}
