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

`retries` is max retry times when http request failed except exceeded timeout
`timeoutRetries` is optional, default value is 0. Max retry times only when http request exceeded timeout. Recommended value is small number, like 1.

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
