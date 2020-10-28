# Iaido
> A lightweight sidecar proxy for multi-region applications

[![Build Status](https://travis-ci.com/bobvawter/iaido.svg?branch=main)](https://travis-ci.com/bobvawter/iaido)
[![codecov](https://codecov.io/gh/bobvawter/iaido/branch/main/graph/badge.svg?token=B8C5YFH25N)](https://codecov.io/gh/bobvawter/iaido)
[![Go Report Card](https://goreportcard.com/badge/github.com/bobvawter/iaido)](https://goreportcard.com/report/github.com/bobvawter/iaido)

Iaido is intended to ease the way for organizations to adopt
multi-region application architectures without needing to roll out a
full service mesh, global traffic direction, or other complicated
infrastructure.  Iaido provides a "just-works" approach with reasonable
defaults for getting started.

Because Iaido is lightweight, it can be run as a "sidecar" for each
instance of your application without significantly impacting your CPU,
memory, or latency budgets.  Configure Iaido with the desired proxy
configuration for an existing network service, and point your app at
`localhost`.

## Features

* Enhance existing application stacks without code changes
* In-region connection distribution and tiered fail-over/fail-back.
* Minimal CPU, memory, and latency footprint, using the
  [`splice`](https://en.wikipedia.org/wiki/Splice_(system_call)) system
  call where available.
* File-based configuration that can be updated without restarting.
  * Iaido will poll its configuration file for changes, or you can send
    a `kill -HUP` to force an immediate reload.
* Expands multi-A and multi-AAAA DNS records that may be published by an
  existing service mesh (e.g. a Kubernetes "headless" service)

## Quickstart

Iaido uses a configuration file, however it will generate one for you
with just a few inputs to specify a list of local ports and remote
backends to forward to:

```
iaido --config iaido.yaml \
       --bootstrapAddr 127.0.0.1 \
       --bootstrap 26257:127.0.0.1:26258 \
       --bootstrap 26257:127.0.0.1:26259 \
       --bootstrap 26257:127.0.0.1:26260 \
       --bootstrap 8080:127.0.0.1:8181 
wrote bootstrapped configuration file to iaido.yaml

iaido -c iaido.yaml 
listening on 127.0.0.1:26257
listening on 127.0.0.1:8080
```

The syntax for the binding is `<local port>:<remote host>:<remote port>`
and is similar to that used with SSH port-forwarding.

## Demos

* [CockroachDB local-cluster balancing](./configs/crdb-local)
* Multi-region Kubernetes deployment (Coming soon)

## Future Work

Currently, Iaido only acts on raw TCP streams.  Direct support for HTTP,
PostgreSQL, and other application protocols would enable more seamless
traffic distribution and failover semantics.

TLS-stripping/wrapping would allow incremental adoption of secured,
end-to-end network flows.
