# Synthetic heart Rest API

Simple REST endpoint to query redis for synthetic test results, agent details etc.

## Config

```yaml
address: "0.0.0.0:51230"                                          # Address at which the rest api would run
storageAddress: "redis:6379"                                      # Address at which the storage is running
uiAddress: "http://localhost:51230?server=http://localhost:51230" # Address to redirect to when user requests /ui
```