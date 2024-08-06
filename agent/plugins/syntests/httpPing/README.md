# HTTP Ping

Pings a HTTP endpoint and checks if its the expected response.

## Test Details map

 1. `key`: `_log`
    - `value`: details of a request

## Example Configuration

```yaml
  config : |
    address: "localhost:9200"
    expectedCodeRegex : ^(202|472){1}
    retries: 3
    timeoutRetries: 1
```

```yaml
  config : |
    address: "localhost:9200"
    expectedCodeRegex : ^(202|472){1}
    consecutiveTestSuccessCount: 3
    consecutiveTestInterval: 3s
```

`retries` is max retry times when http request failed except exceeded timeout
`timeoutRetries` is optional, default value is 0. Max retry times only when http request exceeded timeout. Recommended value is small number, like 1.
`retries` and `timeoutRetries` retry the ping test until it http request succeed or reach the max retry times.

`consecutiveTestSuccessCount` is the total http ping test times. If any failure occurs, the final test result will be failed. It conflicts with `retries` and `timeoutRetries`.
`consecutiveTestInterval` is ping interval in the http ping test. Default value is 5s.
`retries`/`timeoutRetries` and `consecutiveTestSuccessCount` are mutually exclusive. They can't be larger than 0 at the same time.

For multiple endpoints (performs the tests in parallel):

```yaml
  config : |
    - address: "localhost:9200"
      expectedCodeRegex : ^(202|472){1}
      retries: 3

    - address: "localhost:6400"
      expectedCodeRegex : ^(202|472){1}
      retries: 3
```
