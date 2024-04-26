# Synthetic Heart Controller

This is a controller to handler SyntheticTest CRDs. It mirrors the CRD configs to redis, which
is watched by the synthetic-heart agents.

It also:

- any agents that are no longer correspond to a k8s node
- any tests that do not exist
- reschedules tests that are on non-active/non-existent nodes

The controller was built using [Kubebuilder v3.14.0](https://github.com/kubernetes-sigs/kubebuilder)

kubebuilder commands that were run to generate the project:

```sh
kubebuilder init --domain infra.webex.com --license none --repo "github.com/cisco-open/synthetic-heart/controller"
kubebuilder create api --group synheart.infra.webex.com --version v1 --kind SyntheticTest
```

# Synthetic Test CRD Example

```yaml
apiVersion: synheart.infra.webex.com/v1
kind: SyntheticTest
metadata:
  name: dns-external
spec:
  plugin: dns
  displayName: DNS (External)
  node: "*"
  repeat: 5m
  config: |
    domains: ["google.com"]
```

## Config

```
# Needs two environment variables
SYNHEART_STORE_ADDR="localhost:6379"  # the address of redis
AGENT_STATUS_DEADLINE="30s" # deadline for an agent before its considered not alive - to check whether tests need rescheduling
```

## Development

### Prerequisites

- go version v1.21.0+
- docker version 17.03+.
- kubectl version v1.11.3+.
- Access to a Kubernetes v1.11.3+ cluster.

In order to run:

- Run a local [redis server](https://redis.io/download)
- `export SYNHEART_STORE_ADDR=localhost:6379`
- `make install`
- `make run`

Useful make commands:

- `make manifests`: Builds api definitions
- `make install` : Installs the CRD to the cluster using local kubeconfig
- `make run` : Runs the controller using the local kubeconfig
