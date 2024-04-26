# Ping Test

Sends an ICMP ping

## Test Details map

No extra info

## Example Configuration

```yaml
    config: |
      domain: 8.8.8.8    # Where to ping
      pings: 5           # How many pings to send
      privileged: true   # Whether to run as root
```
