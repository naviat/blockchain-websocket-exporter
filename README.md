# WebSocket Ping-Pong Exporter Implementation Guide

This guide explains the implementation and usage of a custom WebSocket exporter for monitoring ping-pong latency with VictoriaMetrics VMProbe.

## Overview

This custom exporter:

1. Specifically measures WebSocket ping-pong latency
2. Works with URLs in the format `wss://<id>/<token>`
3. Exposes a `/probe` endpoint compatible with VMProbe
4. Provides focused metrics about connection success and latency

## Components

The implementation consists of:

1. **Exporter Code** - A Go application that:
   - Establishes WebSocket connections
   - Sends ping frames and measures time until pong responses
   - Exposes Prometheus metrics
   - Provides a `/probe` endpoint compatible with VMProbe

2. **Docker Image** - Packages the exporter for deployment

3. **Kubernetes Deployment** - Runs the exporter in your cluster

4. **VMProbe Configuration** - Configures monitoring of your WebSocket endpoints

## Key Metrics

The exporter provides these metrics:

- `probe_success` - Overall success of the probe
- `probe_duration_seconds` - Total probe duration
- `probe_websocket_up` - Success of the WebSocket connection establishment
- `probe_websocket_connection_duration_seconds` - Time to establish WebSocket connection
- `probe_websocket_ping_pong_success` - Success of the ping-pong exchange
- `probe_websocket_ping_pong_duration_seconds` - Time between ping and pong (latency)

## Implementation Details

### Exporter Architecture

The exporter implements a simple HTTP server with two main endpoints:

1. `/metrics` - Standard Prometheus metrics endpoint for the exporter itself
2. `/probe` - Endpoint that accepts a `target` parameter, probes the specified WebSocket URL, and returns metrics about the probe

The probe process:

1. Establishes a WebSocket connection to the target
2. Measures connection time
3. Sends a ping frame
4. Waits for pong response
5. Measures ping-pong latency
6. Returns all metrics

### Key Code Components

- **Connection Handling**: Uses the Gorilla WebSocket library to establish connections
- **Ping-Pong Measurement**: Uses WebSocket's ping/pong frames for latency measurement
- **Prometheus Integration**: Uses the Prometheus client library to expose metrics
- **VMProbe Compatibility**: Follows the same pattern as the blackbox exporter

## Deployment

### Building the Docker Image

```bash
# Clone the repository
git clone https://github.com/naviat/websocket-exporter
cd websocket-exporter

# Build and push the Docker image
docker build -t naviat/websocket-exporter:latest .
docker push naviat/websocket-exporter:latest
```

### Deploying in Kubernetes

1. Apply the Kubernetes deployment:

```bash
# Update the image reference in kubernetes-deployment.yaml
kubectl apply -f kubernetes-deployment.yaml
```

2. Create a VMProbe to monitor your WebSocket endpoints:

```bash
# Update the targets in vmprobe-config.yaml
kubectl apply -f vmprobe-config.yaml
```

## Usage

### Testing the Exporter

You can test the exporter by sending a request to the probe endpoint:

```bash
# From within the cluster
curl "http://websocket-exporter.monitoring.svc:9095/probe?target=wss://node1.example.com/token1"
```

The response will include metrics about the probe:

```
# HELP probe_success Displays whether or not the probe was a success
# TYPE probe_success gauge
probe_success 1
# HELP probe_websocket_up Displays whether the WebSocket connection was successful
# TYPE probe_websocket_up gauge
probe_websocket_up 1
# HELP probe_websocket_connection_duration_seconds Duration of the WebSocket connection establishment
# TYPE probe_websocket_connection_duration_seconds gauge
probe_websocket_connection_duration_seconds 0.123
# HELP probe_websocket_ping_pong_duration_seconds Duration between ping and pong messages
# TYPE probe_websocket_ping_pong_duration_seconds gauge
probe_websocket_ping_pong_duration_seconds 0.045
# HELP probe_websocket_ping_pong_success Displays whether the WebSocket ping-pong was successful
# TYPE probe_websocket_ping_pong_success gauge
probe_websocket_ping_pong_success 1
# HELP probe_duration_seconds Returns how long the probe took to complete in seconds
# TYPE probe_duration_seconds gauge
probe_duration_seconds 0.169
```

### Configuration Options

The exporter supports several command-line flags:

- `--web.listen-address` - Address to listen on (default: `:9095`)
- `--web.telemetry-path` - Path for exporter metrics (default: `/metrics`)
- `--web.probe-path` - Path for probe endpoint (default: `/probe`)
- `--timeout` - Probe timeout (default: `10s`)
- `--ping.interval` - Interval for continuous monitoring (default: `1s`)
- `--debug` - Enable debug logging (default: `false`)

Example:

```bash
./websocket-exporter --timeout=5s --debug
```

## VMProbe Configuration

The VMProbe configuration specifies which endpoints to monitor:

```yaml
apiVersion: operator.victoriametrics.com/v1beta1
kind: VMProbe
metadata:
  name: websocket-pingpong-probe
  namespace: monitoring
spec:
  jobName: websocket-pingpong-monitoring
  vmProberSpec:
    url: websocket-exporter.monitoring.svc:9095
    path: /probe
  interval: 30s
  targets:
    staticConfig:
      targets:
      - wss://node1.example.com/token1
      - wss://node2.example.com/token2
      - wss://node3.example.com/token3
```

## Example Alerting Rules

Create alerts for WebSocket issues:

```yaml
apiVersion: operator.victoriametrics.com/v1beta1
kind: VMRule
metadata:
  name: websocket-alerts
  namespace: monitoring
spec:
  groups:
  - name: websocket
    rules:
    - alert: WebSocketEndpointDown
      expr: probe_success{job="websocket-pingpong-monitoring"} == 0
      for: 2m
      labels:
        severity: critical
      annotations:
        summary: "WebSocket endpoint {{ $labels.instance }} is down"
        
    - alert: WebSocketHighPingPongLatency
      expr: probe_websocket_ping_pong_duration_seconds{job="websocket-pingpong-monitoring"} > 0.5
      for: 5m
      labels:
        severity: warning
      annotations:
        summary: "WebSocket ping-pong latency high for {{ $labels.instance }}"
```

## Extending the Exporter

The exporter can be extended in several ways:

1. **Adding Authentication** - For more complex authentication than URL tokens
2. **Custom Messages** - For testing specific WebSocket protocols
3. **TLS Configuration** - For special TLS requirements
4. **Multiple Pings** - For more accurate latency measurements

## Troubleshooting

### Common Issues

1. **Connection Failures**
   - Check network connectivity to the WebSocket endpoints
   - Verify the tokens in the URLs are correct
   - Check TLS certificate validity for secure WebSockets

2. **Timeouts**
   - Adjust the timeout value if endpoints are slow to respond
   - Check for firewall or proxy issues

3. **Ping-Pong Failures**
   - Some WebSocket servers might not support ping frames
   - Check if the server properly implements the WebSocket protocol

### Debugging

Enable debug logging with the `--debug` flag:

```bash
kubectl patch deployment websocket-exporter -n monitoring -p '{"spec":{"template":{"spec":{"containers":[{"name":"websocket-exporter","args":["--web.listen-address=:9095","--web.telemetry-path=/metrics","--web.probe-path=/probe","--timeout=5s","--debug"]}]}}}}'
```

## Conclusion

This custom WebSocket exporter provides focused, reliable monitoring of WebSocket ping-pong latency, which is a crucial metric for understanding the real-time performance of your WebSocket services. By integrating with VMProbe, it fits seamlessly into your existing monitoring infrastructure.
