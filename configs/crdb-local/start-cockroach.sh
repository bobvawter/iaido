#!/usr/bin/env sh

set -e

cockroach start \
  --insecure \
  --store=node1 \
  --listen-addr=localhost:26258 \
  --http-addr=localhost:8081 \
  --join=localhost:26258,localhost:26259,localhost:26260 \
  --background

cockroach start \
  --insecure \
  --store=node2 \
  --listen-addr=localhost:26259 \
  --http-addr=localhost:8082 \
  --join=localhost:26258,localhost:26259,localhost:26260 \
  --background

cockroach start \
  --insecure \
  --store=node3 \
  --listen-addr=localhost:26260 \
  --http-addr=localhost:8083 \
  --join=localhost:26258,localhost:26259,localhost:26260 \
  --background

cockroach init --insecure --port 26258