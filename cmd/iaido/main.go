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

package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/bobvawter/iaido/pkg/config"
	"github.com/bobvawter/iaido/pkg/diagnostics"
	"github.com/bobvawter/iaido/pkg/frontend"
	"github.com/pkg/errors"
	"github.com/spf13/pflag"
)

func main() {
	fs := pflag.NewFlagSet(os.Args[0], pflag.ContinueOnError)
	cfgPath := fs.StringP("config", "c", "iaido.json", "the configuration file to load")
	cfgReloadPeriod := fs.Duration("reloadPeriod", 30*time.Second, "how often to check the configuration file for changes")
	diagAddr := fs.String("diagAddr", "", "an IP:Port pair to bind a diagnostic HTTP server to (e.g. 127.0.0.1:6060)")
	gracePeriod := fs.Duration("gracePeriod", 30*time.Second, "how long to wait before forcefully terminating")
	version := fs.Bool("version", false, "print the version and exit")
	if err := fs.Parse(os.Args[1:]); err != nil {
		if err != pflag.ErrHelp {
			println(err.Error())
			println()
			println(fs.FlagUsagesWrapped(100))
		}
		os.Exit(1)
	}

	if *version {
		println(frontend.BuildID())
		os.Exit(0)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Set up a signal handler to gratefully, then forcefully terminate.
	{
		interrupted := make(chan os.Signal, 1)
		signal.Notify(interrupted, os.Interrupt)
		go func() {
			<-interrupted
			log.Printf("signal received, draining for %s", *gracePeriod)
			cancel()
			select {
			case <-interrupted:
				log.Print("second interrupt received")
			case <-time.After(*gracePeriod):
				log.Print("grace period expired")
			}
			os.Exit(1)
		}()
	}

	fe := &frontend.Frontend{}
	if *diagAddr != "" {
		go func() {
			elts := map[string]interface{}{
				"/statusz": fe,
				"/healthz": "OK",
			}
			log.Print(diagnostics.ListenAndServe(*diagAddr, elts))
		}()
	}

	var mTime time.Time

	hupped := make(chan os.Signal, 1)
	signal.Notify(hupped, syscall.SIGHUP)

	ticker := time.NewTicker(*cfgReloadPeriod)
	defer ticker.Stop()

refreshLoop:
	for {
		if info, err := os.Stat(*cfgPath); err == nil {
			if info.ModTime().After(mTime) {
				mTime = info.ModTime()

				cfg, err := loadConfig(*cfgPath)
				if err != nil {
					log.Print(errors.Wrap(err, "configuration load failed"))
					continue
				}

				if err := fe.Ensure(ctx, cfg); err != nil {
					log.Print(errors.Wrapf(err, "unable to refresh backends"))
					continue
				}
				log.Print("configuration updated")
			}
		} else {
			log.Printf("unable to stat config file: %v", err)
		}

		select {
		case <-ctx.Done():
			break refreshLoop
		case <-ticker.C:
		case <-hupped:
		}
	}

	fe.Wait()
	log.Print("drained, goodbye!")
	os.Exit(0)
}

func loadConfig(path string) (*config.Config, error) {
	cfgFile, err := os.Open(path)
	if err != nil {
		return nil, errors.Wrapf(err, "could not open configuration file %s", path)
	}
	defer cfgFile.Close()

	return config.ParseConfig(cfgFile)
}
