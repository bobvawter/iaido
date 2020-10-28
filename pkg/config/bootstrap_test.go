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
	"io/ioutil"
	"testing"

	"github.com/stretchr/testify/assert"
	"gopkg.in/yaml.v3"
)

func TestBootstrap(t *testing.T) {
	a := assert.New(t)

	f, err := ioutil.TempFile("", "")
	if !a.NoError(err) {
		return
	}

	cfg := Config{}
	a.NoError(Bootstrap(&cfg, f.Name(), "0.0.0.0", []string{
		"8080:127.0.0.1:8080",
		"8080:127.0.0.2:8080",
		"26257:127.0.0.1:26257",
		"26257:127.0.0.1:26258",
	}))
	data, err := yaml.Marshal(cfg)
	a.NoError(err)
	t.Log(string(data))

	expected := Config{
		DiagAddr: "0.0.0.0:13013",
		Frontends: []Frontend{
			{
				BindAddress: "0.0.0.0:26257",
				BackendPool: BackendPool{
					Tiers: []Tier{
						{
							Targets: []Target{
								{
									Hosts: []string{"127.0.0.1"},
									Port:  26257,
								},
								{
									Hosts: []string{"127.0.0.1"},
									Port:  26258,
								},
							},
						},
					},
				},
			},
			{
				BindAddress: "0.0.0.0:8080",
				BackendPool: BackendPool{
					Tiers: []Tier{
						{
							Targets: []Target{
								{
									Hosts: []string{"127.0.0.1", "127.0.0.2"},
									Port:  8080,
								},
							},
						},
					},
				},
			},
		},
	}
	a.NoError(expected.Validate())

	a.Equal(expected, cfg)
}
