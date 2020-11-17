# Local CockroachDB Cluster

The example in this directory will start a local, three-node CockroachDB
cluster and configure Iaido to act as a proxy for both the SQL and Admin
UI endpoints.

* Execute `docker-compose -f server.yaml up` and wait for the cluster to initialize
* Execute `docker-compose -f workload.yaml up` in another terminal to execute the YCSB workload tool

The [Iaido status page](http://127.0.0.1:6060/statusz) and the [SQL
metrics dashboard](http://127.0.0.1:8080/#/metrics/sql/cluster) will
show that connections are being forwarded across different nodes.

The `iaido.yaml` file can be edited without needing to restart the server.