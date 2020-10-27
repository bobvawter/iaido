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
	"sort"
	"time"
)

type latencySample struct {
	backend *Backend
	latency time.Duration
}

type latencySamples []latencySample

// Group the samples together such that all entries in a tier have
// latencies are are within the given bucket size.
func (s latencySamples) assign(bucketSize time.Duration) {
	sort.Slice(s, func(i, j int) bool {
		return s[i].latency < s[j].latency
	})

	start := time.Duration(0)
	tier := 0

	for _, sample := range s {
		if sample.latency >= bucketSize+start {
			start = sample.latency
			tier++
		}
		sample.backend.AssignTier(sample.latency, tier)
	}
}
