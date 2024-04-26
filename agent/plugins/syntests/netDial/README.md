# Net Dial Test
Dials on a port and address to check if its open

## Test Details map
No extra info

## Example Configuration
```yaml
  config : |
    addresses:
      - addr: 1.1.1.1:53
        net: tcp
        timeout: 5 # Seconds, default = 3
      - addr: 127.0.0.1:51230
        net: tcp
```
