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

package config

import (
	"context"
	"flag"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

var enableDNS = flag.Bool("enableDNS", false, "Enable actual DNS resolutions in testing")

func TestParseTarget(t *testing.T) {
	tcs := []struct {
		val      string
		err      string
		expected Target
	}{
		{
			val: "127.0.0.1:8080",
			expected: Target{
				Hosts: []string{"127.0.0.1"},
				Port:  8080,
			},
		},
		{
			val: "tcp:127.0.0.1:8080",
			expected: Target{
				Hosts: []string{"127.0.0.1"},
				Port:  8080,
			},
		},
		{
			val: "udp:127.0.0.1:8080",
			expected: Target{
				Hosts: []string{"127.0.0.1"},
				Port:  8080,
				Proto: UDP,
			},
		},
		{
			val: "localhost:8080",
			expected: Target{
				Hosts: []string{"localhost"},
				Port:  8080,
			},
		},
		{
			val: "[::1]:8080",
			expected: Target{
				Hosts: []string{"::1"},
				Port:  8080,
			},
		},
		{
			val: "localhost",
			err: "could not parse target \"localhost\": address localhost: missing port in address",
		},
		{
			val: "localhost:http",
			err: "could not parse port number in target \"localhost:http\": strconv.Atoi: parsing \"http\": invalid syntax",
		},
	}

	for _, tc := range tcs {
		t.Run(tc.val, func(t *testing.T) {
			a := assert.New(t)

			parsed, err := ParseTarget(tc.val)
			if tc.err != "" {
				a.EqualError(err, tc.err)
			} else {
				a.NoError(err)
				a.Equal(tc.expected, parsed)
			}
		})
	}
}

func TestResolve(t *testing.T) {
	if !*enableDNS {
		t.Skip("not testing actual resolver")
		return
	}
	a := assert.New(t)
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	tgt := &Target{Hosts: []string{"google.com"}, Port: 80}

	resolved, err := tgt.Resolve(ctx)
	a.NoError(err)
	t.Log(resolved)
}
