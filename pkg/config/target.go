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
	"fmt"
	"net"
	"strconv"
	"strings"

	"github.com/pkg/errors"
)

// Resolver can be overridden for testing.
var Resolver = net.Resolver{PreferGo: true}

// Proto defines an enumeration of network protocols.
//
// TODO(bob): Expand this to include HTTP and PSQL options.
type Proto int

const (
	// TCP is Transmission Control Protocol.
	TCP Proto = iota
	// UDP is User Datagram Protocol.
	UDP
)

// UnmarshalText implements encoding.TextMarshaler.
func (p *Proto) UnmarshalText(data []byte) error {
	s := strings.ToUpper(string(data))
	switch s {
	case "TCP":
		*p = TCP
	case "UDP":
		*p = UDP
	default:
		return errors.Errorf("unknown value %q", s)
	}
	return nil
}

// MarshalText implements encoding.TextMarshaler and returns the enum name.
func (p Proto) MarshalText() ([]byte, error) {
	return []byte(p.String()), nil
}

func (p Proto) String() string {
	switch p {
	case TCP:
		return "TCP"
	case UDP:
		return "TCP"
	default:
		return ""
	}
}

// ParseTarget parses a human-provided description of a target.
// It should be of the form "host:port" with an optional "
func ParseTarget(target string) (Target, error) {
	tgt := Target{}
	if target == "" {
		return tgt, errors.New("target may not be empty")
	}
	if strings.Index(strings.ToLower(target), "tcp:") == 0 {
		target = target[4:]
	} else if strings.Index(strings.ToLower(target), "udp:") == 0 {
		tgt.Proto = UDP
		target = target[4:]
	}
	host, port, err := net.SplitHostPort(target)
	if err != nil {
		return Target{}, errors.Wrapf(err, "could not parse target %q", target)
	}
	tgt.Host = host
	tgt.Port, err = strconv.Atoi(port)
	if err != nil {
		return Target{}, errors.Wrapf(err, "could not parse port number in target %q", target)
	}
	return tgt, nil
}

// A Target is a string value that can be resolved to one or more
// network addresses.
//
// Targets are generally assumed to represent TCP/IP endpoints.
type Target struct {
	Host  string
	Port  int
	Proto Proto
}

// Resolve performs network name resolution.
func (t Target) Resolve(ctx context.Context) ([]net.Addr, error) {
	ips, err := Resolver.LookupIPAddr(ctx, t.Host)
	if err != nil {
		return nil, err
	}
	ret := make([]net.Addr, len(ips))
	for i, ip := range ips {
		switch t.Proto {
		case TCP:
			ret[i] = &net.TCPAddr{
				IP:   ip.IP,
				Port: t.Port,
				Zone: ip.Zone,
			}
		case UDP:
			ret[i] = &net.UDPAddr{
				IP:   ip.IP,
				Port: t.Port,
				Zone: ip.Zone,
			}
		default:
			panic(fmt.Sprintf("unimplemented: %q", t))
		}
	}
	return ret, nil
}

// Validate checks the value.
func (t *Target) Validate() error {
	if t.Host == "" {
		return errors.Errorf("Host field is required")
	}
	if t.Port == 0 {
		return errors.Errorf("Port field is required")
	}
	return nil
}

func (t Target) String() string {
	return fmt.Sprintf("%s:%s:%d", t.Proto, t.Host, t.Port)
}
