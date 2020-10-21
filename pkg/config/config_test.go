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
	"bytes"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
)

// This test creates a minimal configuration that can be re-parsed.
func TestMinimalConfig(t *testing.T) {
	a := assert.New(t)

	cfg := &Config{
		Frontends: []Frontend{
			{
				BackendPool: BackendPool{
					Tiers: []Tier{
						{
							Targets: []Target{
								{
									Host: "127.0.0.1",
									Port: 8080,
								},
							},
						},
					},
				},
			},
		},
	}

	data, err := json.Marshal(cfg)
	if !a.NoError(err) {
		return
	}

	cfg2, err := ParseConfig(bytes.NewReader(data))
	a.NoError(err)
	data2, err := json.Marshal(cfg2)
	t.Log(string(data2))
}
