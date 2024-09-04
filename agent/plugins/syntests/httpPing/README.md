# HTTP Ping

Pings a HTTP endpoint and checks if its the expected response.

## Test Details map

 1. `key`: `_log`
    - `value`: details of a request

## Configuration Items

| Key                  | Description                                                                                                       | Required | Memo                                                                                                                         |
|----------------------|-------------------------------------------------------------------------------------------------------------------|----------|------------------------------------------------------------------------------------------------------------------------------|
| `address`            | The address of the endpoint to ping                                                                               | Yes      |                                                                                                                              |
| `expectedCodeRegex`  | The expected http response code regex                                                                             | Yes      |                                                                                                                              |
| `retries`            | The number of max retry times when http request failed                                                            | No       | The final result is successful within the retry attempts.                                                                    |
| `timeoutRetries`     | The number of max retry times only when http request exceeded timeout. Recommended value is small number, like 1. | No       | The final result is successful within the retry attempts.                                                                    |
| `repeatsWithoutFail` | The number of total repeated http ping  test times                                                                | No       | The final result is successful only if all repeated tests pass. It’s mutually exclusive with `retries` and `timeoutRetries`. |
| `waitBetweenRepeats` | The ping interval in the repeated http ping test                                                                  | No       | Default value is 5s.                                                                                                         |

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
    repeatsWithoutFail: 3
    waitBetweenRepeats: 3s
```

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
