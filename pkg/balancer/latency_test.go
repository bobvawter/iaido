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
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestLatencyBucketAssignment(t *testing.T) {
	a := assert.New(t)

	tcs := map[time.Duration]int{
		time.Millisecond:       0,
		2 * time.Millisecond:   0,
		3 * time.Millisecond:   0,
		57 * time.Millisecond:  1,
		63 * time.Millisecond:  1,
		65 * time.Millisecond:  1,
		67 * time.Millisecond:  2,
		68 * time.Millisecond:  2,
		100 * time.Millisecond: 3,
	}

	samples := make(latencySamples, 0, len(tcs))
	for latency := range tcs {
		samples = append(samples, latencySample{&Backend{}, latency})
	}

	samples.assign(10 * time.Millisecond)

	for i := range samples {
		latency := samples[i].backend.mu.lastLatency
		tier := samples[i].backend.mu.tier

		a.Equalf(tcs[latency], tier, "latency: %s", latency)
	}
}
