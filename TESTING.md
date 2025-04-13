# WebSocket Connection Testing Guide

This guide explains how to use the simplified WebSocket Connection Exporter to monitor WebSocket endpoint availability and connection latency.

## Overview

The WebSocket Connection Exporter focuses exclusively on testing:

1. Whether WebSocket endpoints are accessible
2. How long it takes to establish a connection

This approach is ideal for blockchain nodes or other WebSocket endpoints that may not support ping-pong frames or specific message patterns.

## Key Metrics

The exporter provides four critical metrics:

- **`probe_websocket_up`**: 1 if connection succeeded, 0 if failed
- **`probe_websocket_connection_duration_seconds`**: Time to establish the connection
- **`probe_success`**: Overall success of the probe (same as `probe_websocket_up` in this simplified version)
- **`probe_duration_seconds`**: Total probe duration

## How It Works

The exporter:

1. Attempts to establish a WebSocket connection to the specified URL
2. Measures the time it takes to establish the connection
3. Closes the connection immediately after establishing it
4. Reports metrics about the connection attempt

This is similar to what the `wscat` tool does when connecting to a WebSocket endpoint, but in a format suitable for monitoring and alerting.

## Deployment with VMProbe

Create a VMProbe resource to monitor your WebSocket endpoints:

```yaml
apiVersion: operator.victoriametrics.com/v1beta1
kind: VMProbe
metadata:
  name: websocket-connection-probe
  namespace: monitoring
spec:
  jobName: websocket-connection-monitoring
  vmProberSpec:
    url: websocket-exporter.monitoring.svc:9095
    path: /probe
  interval: 30s
  targets:
    staticConfig:
      targets:
      - wss://endpoint1.example.com/path/token
      - wss://endpoint2.example.com/path/token
      - wss://endpoint3.example.com/path/token
```

## Manual Testing

You can manually test your WebSocket endpoints:

```bash
curl "http://localhost:9095/probe?target=wss://your-endpoint.example.com/path/token"
```

Example response:

```
# HELP probe_success Displays whether or not the probe was a success
# TYPE probe_success gauge
probe_success 1
# HELP probe_websocket_up Displays whether the WebSocket connection was successful
# TYPE probe_websocket_up gauge
probe_websocket_up 1
# HELP probe_websocket_connection_duration_seconds Duration of the WebSocket connection establishment
# TYPE probe_websocket_connection_duration_seconds gauge
probe_websocket_connection_duration_seconds 0.156
# HELP probe_duration_seconds Returns how long the probe took to complete in seconds
# TYPE probe_duration_seconds gauge
probe_duration_seconds 0.158
```

## Example Alerting Rules

Create alerts for WebSocket connection issues:

```yaml
apiVersion: operator.victoriametrics.com/v1beta1
kind: VMRule
metadata:
  name: websocket-connection-alerts
  namespace: monitoring
spec:
  groups:
  - name: websocket
    rules:
    - alert: WebSocketEndpointDown
      expr: probe_websocket_up{job="websocket-connection-monitoring"} == 0
      for: 2m
      labels:
        severity: critical
      annotations:
        summary: "WebSocket endpoint {{ $labels.instance }} is down"

    - alert: WebSocketHighConnectionLatency
      expr: probe_websocket_connection_duration_seconds{job="websocket-connection-monitoring"} > 1
      for: 5m
      labels:
        severity: warning
      annotations:
        summary: "WebSocket connection latency high for {{ $labels.instance }}"
```

## Debugging

Enable debug logging with the `--debug` flag:

```bash
./websocket-connection-exporter --debug
```

This will provide detailed logs about connection attempts.

## Benefits of This Approach

- **Simplicity**: Focuses only on connection establishment
- **Universal Compatibility**: Works with any WebSocket endpoint, regardless of protocol specifics
- **Low Overhead**: Minimal impact on endpoints being monitored
- **Clear Metrics**: Easy to understand and act upon
