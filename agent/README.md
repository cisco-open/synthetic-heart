# Synthetic heart Agent

The agent is the component responsible for running the synthetic tests. The agent has a plugin architecture using
[Hashicorp's go-plugin](https://github.com/hashicorp/go-plugin).

## Configuration
This section is for the configuration of the agent.
A sample config file agent  is provided below:
```yaml
gracePeriod: 3s         # When the agent is exiting, how long to wait to process/export any pending test results
syncFrequency: 30s      # How often to poll external storage for new syntest configs

storage:                    # External storage configuration
  type: redis               # Type of external storage
  address: redis:6379       # Address of the external storage
  buffer: 1000              # The size on import buffer (approximately: no_of_nodes)
  exportRate: 15s           # How often to export health status of plugins/agent
  pollRate: 60s             # How often to poll for new test runs

prometheus:                 # Whether to run prometheus exporter
  address: :2112            # Address at which to run the prometheus server

# Any run-time info e.g. cluster information
etc:
  clusterName: abc
```

## Metrics
By default the agent export the test runtimes and the test marks.

If a test wants to export custom metrics, it needs to add the following to `TestResult.Details` map:
- `key`: `_prometheus`
- `value`: YAML representation of `PrometheusMetrics` struct in [models.go](https://github.com/cisco-open/synthetic-heart/blob/master/common/models.go#L32)

Note: At the moment only Prometheus Gauges are supported


## Building proto files
Run `make proto`

## Testing
Run the tests, do: `make test`

## Development

### Writing a new Synthetic Test Plugin

All Synthetic-Heart tests are [hashicorp go-plugins](https://github.com/hashicorp/go-plugin), so it's relatively straightforward to write plugins with custom functionality, including exposing custom metrics to Prometheus.

Some comments on plugin development:

 - Please try to keep plugins as generic as possible.
 - Make the plugins as configurable as possible.
 - Use worker pools to allow multiple instances to be run by one plugin. For example with http ping test, it's expensive to run 5 instances of the same plugins to test 5 domains, compared to 1 instance testing all 5 domains.
 - Try exporting plugin specific metrics.

To add a new synthetic test, follow:

1. Run `make new-go-test name=myTest`<br>
   NOTE: Please use camel cased name like (`vaultAws` or `dns`)<br>

2. In `common/constants.go` file, Add your plugin name as a constant (with suffix `TestName`) e.g.
   ```go
        MyTestTestName = "myTest" // <- Add this
        DockerPullTestName = "dockerPull"
        DNSTestName = "dns"
        ...
   ```

   Make sure the value of the constant matches exactly with what was entered in Step 1 (in this case `myTest`).

3. In `pluginmanager/pluginRegistration.go` file, register your plugin by adding:
    - `RegisterSynTestPlugin(YourTestTestName, []string{BinPath + SyntestPrefix + MyTestTestName})`

