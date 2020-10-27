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

package frontend_test

import (
	"context"
	"io"
	"io/ioutil"
	"log"
	"net"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/bobvawter/iaido/pkg/config"
	"github.com/bobvawter/iaido/pkg/frontend"
	"github.com/bobvawter/iaido/pkg/loop"
	it "github.com/bobvawter/iaido/pkg/testing"
	"github.com/stretchr/testify/assert"
	"gopkg.in/yaml.v3"
)

// This is an end-to-end smoke test of the Iaido proxy.
func TestSmoke(t *testing.T) {
	a := assert.New(t)
	const backendCount = 4
	const payloadSize = 4096
	const requestCount = 64
	const requestWorkers = 8

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	backends := make([]*CharGenServer, backendCount)
	tier := config.Tier{
		DialFailureTimeout: 10 * time.Second,
	}
	for i := range backends {
		var err error
		backends[i], err = startChargen(ctx, payloadSize)
		if !a.NoError(err) {
			return
		}
		tier.Targets = append(tier.Targets, config.Target{
			Hosts: []string{backends[i].Addr.IP.String()},
			Port:  backends[i].Addr.Port,
			Proto: config.TCP,
		})
	}

	cfg := &config.Config{
		Frontends: []config.Frontend{
			{
				BackendPool: config.BackendPool{
					// Disable extra pings so our request counts are correct.
					LatencyBucket: -1,
					Tiers:         []config.Tier{tier},
				},
				BindAddress:  ":13013",
				IdleDuration: time.Minute,
			},
		},
	}
	if !a.NoError(cfg.Validate()) {
		return
	}

	fe := frontend.Frontend{}
	if !a.NoError(fe.Ensure(ctx, cfg)) {
		return
	}

	var wg sync.WaitGroup
	wg.Add(requestWorkers)
	var remainingRequests = int32(requestCount)
	for i := 0; i < requestWorkers; i++ {
		go func() {
			defer wg.Done()
			for {
				if atomic.AddInt32(&remainingRequests, -1) < 0 {
					return
				}
				conn, err := net.Dial("tcp", "127.0.0.1:13013")
				if !a.NoError(err) {
					return
				}
				count, err := io.Copy(ioutil.Discard, conn)
				_ = conn.Close()
				a.NoError(err)
				a.Equal(payloadSize, int(count))
			}
		}()
	}
	wg.Wait()
	fe.Wait()

	// Ensure that the total number of requests was made.
	count := uint64(0)
	for _, cg := range backends {
		count += cg.ConnectionCount()
	}
	a.Equal(requestCount, int(count))

	data, err := yaml.Marshal(&fe)
	a.NoError(err)
	log.Print(string(data))
}

type CharGenServer struct {
	Addr            *net.TCPAddr
	ConnectionCount func() uint64
}

func startChargen(ctx context.Context, payloadSize int) (*CharGenServer, error) {
	addr, opt, err := it.CharGen(ctx, payloadSize)
	if err != nil {
		return nil, err
	}
	copt, counter := loop.WithConnectionCounter()
	loop.New(opt, copt).Start(ctx)
	return &CharGenServer{addr.(*net.TCPAddr), counter}, nil
}
