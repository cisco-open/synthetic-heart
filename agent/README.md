# Synthetic heart Agent

The agent is the component responsible for running the synthetic tests. The agent has a plugin architecture using
[Hashicorp's go-plugin](https://github.com/hashicorp/go-plugin).

## Configuration

This section is for the configuration of the agent.
A sample config file agent  is provided below:

```yaml
gracePeriod: 3s         # When the agent is exiting, how long to wait to process/export any pending test results
syncFrequency: 30s      # How often to poll external storage for new syntest configs
printPluginLogs: onFail # Whether to print logs from plugin to stdout (always, never, onFail)
storage:                    # External storage configuration
   type: redis               # Type of external storage
   address: redis.{{ .Release.Namespace }}.svc:6379
   bufferSize: 1000          # The size on import buffer (approximately: no_of_nodes * no_of_tests)
   exportRate: {{ .Values.agent.exportRate }}
   pollRate: 60s             # How often to poll for new test runs

prometheus:                 # Whether to run prometheus exporter
  address: :2112            # Address at which to run the prometheus server
  labels:
     <prometheus-label>: <value> # Any labels to add to the prometheus metrics for the tests it runs
     nodeName: {{.Agent.NodeName}}
     podName: {{.Agent.PodName}}
     agentNamespace: {{.Agent.AgentNamespace}}
     plugin: {{.TestConfig.PluginName}}
     label-1: {{index .Agent.PodLabels "label-1"}}
     
matchTestNamespaces: # The agent will only run SyntheticTest that match these namespace(s) (empty list means all)
   - synthetic-heart-system
   
matchTestLabels:     # The agent will only run SyntheticTest that match these labels (empty list means any)
    infra: "true"
    
enabledPlugins:      # Location of the plugin binaries/scripts and how to run them
   - path: "./plugins/*"
   - path: "./plugins-python/*/*.py"
     cmd: "python3"
```

## Metrics

By default the agent export the test runtimes and the test marks.

If a test wants to export custom metrics, it needs to add the following to `TestResult.Details` map:

- `key`: `_prometheus`
- `value`: YAML representation of `PrometheusMetrics` struct in [models.go](https://github.com/cisco-open/synthetic-heart/blob/master/common/models.go#L32)

Note: At the moment only Prometheus Gauges are supported

## Testing

Run the tests, do: `make test`

## Images

There are three container images for the agent:
 - `synthetic-heart-agent:<version>-no-plugins`: Image with no plugins (intended to be used as a base image)
 - `synthetic-heart-agent:<version>`: Image with golang plugins
 - `synthetic-heart-agent:<version>-with-py`: Image with python interpreter and python plugins

## Development

### Debugging the agent

You can run the agent locally with the following commands from the root directory:

```shell
# build the binary
cd agent; SYNHEART_VERSION=1.2.0 make build-agent; cd .. 

# run the redis server
redis-server

# run the agent
NODE_NAME=localhost POD_NAME=localhost-pod NAMESPACE=localhost-ns LOG_LEVEL=trace ./agent/bin/agent ./testing/configs/agent-config.yaml

# run some tests
cd testing/test-client; go run testClient.go
```

### Writing a new Synthetic Test Plugin

All Synthetic-Heart tests are [hashicorp go-plugins](https://github.com/hashicorp/go-plugin), so it's relatively straightforward to write plugins with custom functionality, including exposing custom metrics to Prometheus.

Some comments on plugin development:

- Please try to keep plugins as generic as possible.
- Make the plugins as configurable as possible.
- Use worker pools to allow multiple instances to be run by one plugin. For example with http ping test, it's expensive to run 5 instances of the same plugins to test 5 domains, compared to 1 instance testing all 5 domains.
- Try exporting plugin specific metrics.

### To add a new synthetic test plugin:

For golang plugins:
  - Test name should be camelCase (in this example: `myTest`)
  - Run `make new-go-test name=myTest`. This will create a new directory `./plugins/syntests/myTest` with a sample plugin.

For python plugins:
  - Test name should be kebab-case or snake_case (in this example: `my-test`)
  - Run `make new-python-test name=my-test`. This will create a new directory `./plugins/syntests-python/my-test` with a sample plugin. 

NOTE: Please add a README.md in the new plugin directory with description of the plugin, and how to configure