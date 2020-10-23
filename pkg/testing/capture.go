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
	"io"
	"log"
	"net"

	"github.com/bobvawter/iaido/pkg/loop"
)

// Capture is a trivial server that will accept connections and capture the bytes.
//
// Note that access to dest is not synchronized and may be written to
// from multiple goroutines simultaneously.
func Capture(ctx context.Context, dest io.Writer) (net.Addr, loop.Option, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, nil, err
	}
	go func() {
		<-ctx.Done()
		_ = l.Close()
	}()
	tcp := l.(*net.TCPListener)
	opt := loop.WithTCPHandler(tcp, func(ctx context.Context, conn *net.TCPConn) error {
		log.Printf("recording data from %s", conn.RemoteAddr())
		if err := conn.CloseWrite(); err != nil {
			return err
		}
		_, err := io.Copy(dest, conn)
		return err
	})
	return tcp.Addr(), opt, nil
}
