# Curl Test

Does a curl on an url, then prints out the result, can also export the metrics to prometheus.
The `outputOptions` correspond to the `--write-out` options in curl command. [More info](https://ec.haxx.se/usingcurl/usingcurl-verbose/usingcurl-writeout)

## Test Details map

Populates the prometheus metrics that prometheus plugin supports

## Example Configuration

```yaml
    config: |
      url: https://google.com
      outputOptions:
      - name: time_namelookup
        prometheusMetric: true  # time_namelookup will be posted to prometheus as a Gauge
      - name: remote_ip
        prometheusLabel: true   # remote_ip will be added as label to all metrics posted
```
