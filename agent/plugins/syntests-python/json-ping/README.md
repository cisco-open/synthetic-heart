# JSON Ping Test

JsonPingTest is a test plugin that fetches a JSON response from a URL and validates using [JMESPath](https://jmespath.org/) queries.

## Example Configuration

```
url: https://api64.ipify.org/?format=json   # url to fetch
queries:  # jmes path queries and expected regex
- query: "ip"
  expected: "^\\d+\\.\\d+\\.\\d+\\.\\d+$"
```
