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
	"gopkg.in/yaml.v3"
)

func main() {
	fs := pflag.NewFlagSet(os.Args[0], pflag.ContinueOnError)
	cfgPath := fs.StringP("config", "c", "iaido.yaml", "the configuration file to load")
	cfgReloadPeriod := fs.Duration("reloadPeriod", time.Second, "how often to check the configuration file for changes")
	checkOnly := fs.Bool("check", false, "if true, check and print the configuration file and exit")
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

	cfg := &config.Config{}
	var mTime time.Time
	loadConfig := func() (bool, error) {
		info, err := os.Stat(*cfgPath)
		if err != nil {
			return false, errors.Wrapf(err, "unable to stat config file: %s", *cfgPath)
		}
		if info.ModTime() == mTime {
			return false, nil
		}
		mTime = info.ModTime()

		err = loadConfig(*cfgPath, cfg)
		if err != nil {
			return false, err
		}
		return true, nil
	}

	if _, err := loadConfig(); err != nil {
		log.Fatal(err)
	}

	if *checkOnly {
		bytes, err := yaml.Marshal(cfg)
		if err != nil {
			log.Fatal(err)
		}
		print(string(bytes))
		os.Exit(0)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Set up a signal handler to gratefully, then forcefully terminate.
	{
		interrupted := make(chan os.Signal, 1)
		signal.Notify(interrupted, syscall.SIGTERM, syscall.SIGINT)
		go func() {
			<-interrupted
			log.Printf("signal received, draining for %s", cfg.GracePeriod)
			cancel()
			select {
			case <-interrupted:
				log.Print("second interrupt received")
			case <-time.After(cfg.GracePeriod):
				log.Print("grace period expired")
			}
			os.Exit(1)
		}()
	}

	fe := &frontend.Frontend{}

	if cfg.DiagAddr != "" {
		go func() {
			elts := map[string]interface{}{
				"/configz": cfg,
				"/statusz": fe,
				"/healthz": "OK",
			}
			log.Print(diagnostics.ListenAndServe(cfg.DiagAddr, elts))
		}()
	}

	hupped := make(chan os.Signal, 1)
	signal.Notify(hupped, syscall.SIGHUP)

	ticker := time.NewTicker(*cfgReloadPeriod)
	defer ticker.Stop()

refreshLoop:
	for {
		if err := fe.Ensure(ctx, cfg); err != nil {
			log.Printf("unable to refresh frontend: %v", err)
		}
		select {
		case <-ctx.Done():
			break refreshLoop
		case <-ticker.C:
		case <-hupped:
		}
		if _, err := loadConfig(); err != nil {
			log.Printf("unable to reload configuration: %v", err)
		}
	}

	fe.Wait()
	log.Print("drained, goodbye!")
	os.Exit(0)
}

func loadConfig(path string, cfg *config.Config) error {
	cfgFile, err := os.Open(path)
	if err != nil {
		return errors.Wrapf(err, "could not open configuration file %s", path)
	}
	defer cfgFile.Close()

	return config.DecodeConfig(cfgFile, cfg)
}
