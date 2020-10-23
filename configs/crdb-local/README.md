# Local CockroachDB Cluster

The example in this directory will start a local, three-node CockroachDB
cluster and configure Iaido to act as a proxy for both the SQL and Admin
UI endpoints.

* Execute the `start-cockroach.sh` to initialize a 3-node cluster
* Run `iaido --config iaido.yaml --diagAddr 127.0.0.1:6060`
* Navigate to http://127.0.0.1:6060/statusz to see the Iaido status
* Execute `cockroach workload init tpcc --warehouses 10 --drop`
* Navigate to http://127.0.0.1:8080/#/metrics/sql/cluster to see the SQL
  connections spread across the different nodes.