# Iaido
> A lightweight sidecar proxy for multi-region applications

[![Build Status](https://travis-ci.org/bobvawter/iaido.svg?branch=main)](https://travis-ci.org/bobvawter/iaido)
[![codecov](https://codecov.io/gh/bobvawter/iaido/branch/main/graph/badge.svg?token=B8C5YFH25N)](https://codecov.io/gh/bobvawter/iaido)
[![Go Report Card](https://goreportcard.com/badge/github.com/bobvawter/iaido)](https://goreportcard.com/report/github.com/bobvawter/iaido)

Iaido is intended to ease the way for organizations to adopt
multi-region application architectures without needing to roll out a
full service mesh, global traffic direction, or other complicated
infrastructure.

Because Iaido is lightweight, it can be run as a "sidecar" for each
instance of your application without significantly impacting your CPU,
memory, or latency budgets.  Configure Iaido with the desired proxy
configuration for an existing network service, and point your app at
`localhost`.

## Features

* Enhance existing application stacks without code changes
* In-region connection distribution and tiered fail-over/fail-back.
* Minimal CPU, memory, and latency footprint.
* File-based configuration that can be updated without restarting.
  * Iaido will poll its configuration file for changes, or you can send
    a `kill -HUP` to force a reload.
  * This works nicely with Kubernetes `ConfigMap` objects that are
    expanded into a container filesystem.
* Expands multi-A and multi-AAAA DNS records (e.g. a Kubernetes
  "headless" service)

## Demos

* [CockroachDB local-cluster balancing](./configs/crdb-local)
* Multi-region Kubernetes deployment (Coming soon)

## Future Work

Currently, Iaido only acts on raw TCP streams.  Direct support for HTTP,
PostgreSQL, and other application protocols would enable more seamless
traffic distribution and failover semantics.

TLS-stripping/wrapping would allow incremental adoption of secured,
end-to-end network flows.
