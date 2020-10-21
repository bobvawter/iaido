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
	cfgPath := fs.StringP("config", "c", "iado.json", "the configuration file to load")
	cfgReloadPeriod := fs.Duration("reloadPeriod", 30*time.Second, "how often to check the configuration file for changes")
	diagAddr := fs.String("diagAddr", "", "an IP:Port pair to bind a diagnostic HTTP server to (e.g. 127.0.0.1:6060)")
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

	cfgFile, err := os.Open(*cfgPath)
	if err != nil {
		log.Fatal(err)
	}

	cfg, err := config.ParseConfig(cfgFile)
	if err != nil {
		log.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Set up a signal handler to gratefully, then forcefully terminate.
	{
		ch := make(chan os.Signal, 1)
		signal.Notify(ch, os.Interrupt)
		go func() {
			<-ch
			log.Print("signal received, draining")
			cancel()
			<-ch
			log.Print("second interrupt received, goodbye!")
			os.Exit(1)
		}()
	}

	fe := &frontend.Frontend{}
	if err := fe.Ensure(ctx, cfg); err != nil {
		log.Fatal(err)
	}

	if *diagAddr != "" {
		go func() {
			elts := map[string]interface{}{
				"/statusz": fe,
				"/healthz": "OK",
			}
			log.Print(diagnostics.ListenAndServe(*diagAddr, elts))
		}()
	}

	// A goroutine to look for configuration file changes.
	go func() {
		info, err := os.Stat(*cfgPath)
		if err != nil {
			log.Print(errors.Wrapf(err, "could not stat configuration file %s; config reload disabled", *cfgPath))
			return
		}
		mTime := info.ModTime()

		hupped := make(chan os.Signal, 1)
		signal.Notify(hupped, syscall.SIGHUP)

		ticker := time.NewTicker(*cfgReloadPeriod)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			case <-hupped:
			}

			info, err := os.Stat(*cfgPath)
			if err != nil {
				log.Print(errors.Wrapf(err, "could not stat configuration file %s; config reload deferred", *cfgPath))
				continue
			}

			if !info.ModTime().After(mTime) {
				continue
			}
			log.Print("reloading configuration file")
			mTime = info.ModTime()

			if _, err := cfgFile.Seek(0, 0); err != nil {
				log.Print(errors.Wrap(err, "could not seek in config file"))
				continue
			}

			cfg, err := config.ParseConfig(cfgFile)
			if err != nil {
				log.Print(errors.Wrap(err, "configuration reload failed"))
				continue
			}

			if err := fe.Ensure(ctx, cfg); err != nil {
				log.Print(errors.Wrapf(err, "configuration reload failed"))
				continue
			}
			log.Print("configuration reloaded")
		}
	}()

	<-ctx.Done()
	fe.Wait()
	log.Print("drained, goodbye!")
	os.Exit(0)
}
