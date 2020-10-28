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
	"fmt"
	"log"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/pkg/errors"
	"gopkg.in/yaml.v3"
)

var bindRegex = regexp.MustCompile(`^(\d+):(.*):(\d+)$`)

// Bootstrap will create a minimal configuration with sane defaults.
//
// If cfgPath is non-empty, the configuration will be written to this
// path.
func Bootstrap(cfg *Config, cfgPath string, bootAddr string, binds []string) error {
	if len(binds) == 0 {
		return errors.New("no bindings specified")
	}
	cfg.DiagAddr = fmt.Sprintf("%s:13013", bootAddr)

	// A map of local port -> remote port -> Target
	bindings := make(map[int]map[int]*Target)
	for _, bind := range binds {
		matches := bindRegex.FindStringSubmatch(bind)
		if matches == nil {
			return errors.Errorf("could not parse binding %q into pattern %s", bind, bindRegex)
		}
		localPort, err := strconv.Atoi(matches[1])
		if err != nil {
			return errors.Errorf("could not parse port number %q from binding %q", matches[1], bind)
		}
		remoteHost := matches[2]
		remotePort, err := strconv.Atoi(matches[3])
		if err != nil {
			return errors.Errorf("could not parse port number %q from binding %q", matches[3], bind)
		}

		sub := bindings[localPort]
		if sub == nil {
			sub = make(map[int]*Target)
			bindings[localPort] = sub
		}

		tgt := sub[remotePort]
		if tgt == nil {
			tgt = &Target{Port: remotePort}
			sub[remotePort] = tgt
		}

		tgt.Hosts = append(tgt.Hosts, remoteHost)
	}

	for localPort, sub := range bindings {
		fe := Frontend{
			BindAddress: fmt.Sprintf("%s:%d", bootAddr, localPort),
		}

		t := Tier{}
		for _, tgt := range sub {
			sort.Strings(tgt.Hosts)
			t.Targets = append(t.Targets, *tgt)
		}
		sort.Slice(t.Targets, func(i, j int) bool {
			return t.Targets[i].Port < t.Targets[j].Port
		})

		fe.BackendPool.Tiers = []Tier{t}
		cfg.Frontends = append(cfg.Frontends, fe)
	}
	sort.Slice(cfg.Frontends, func(i, j int) bool {
		return strings.Compare(cfg.Frontends[i].BindAddress, cfg.Frontends[j].BindAddress) < 0
	})

	if err := cfg.Validate(); err != nil {
		return err
	}

	if cfgPath != "" {
		f, err := os.OpenFile(cfgPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
		if err != nil {
			return err
		}
		enc := yaml.NewEncoder(f)
		enc.SetIndent(2)
		if err := enc.Encode(cfg); err != nil {
			return err
		}
		if err := enc.Close(); err != nil {
			return err
		}

		log.Printf("wrote bootstrapped configuration file to %s", cfgPath)
	}
	return nil
}
