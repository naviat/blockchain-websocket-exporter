# Blockchain WebSocket Connection Exporter

A Prometheus exporter for monitoring WebSocket connections to blockchain nodes. This exporter focuses on establishing connections to WebSocket endpoints and measuring connection success and latency.

## Overview

This WebSocket Connection Exporter:

1. Establishes WebSocket connections to blockchain node endpoints
2. Measures connection establishment time and success
3. Exposes a `/probe` endpoint compatible with VictoriaMetrics VMProbe
4. Provides focused metrics about connection success and latency

## Key Metrics

The exporter provides these metrics:

- `probe_success` - Overall success of the probe
- `probe_duration_seconds` - Total probe duration
- `probe_websocket_up` - Success of the WebSocket connection establishment
- `probe_websocket_connection_duration_seconds` - Time to establish WebSocket connection

## Implementation Details

### Exporter Architecture

The exporter implements a simple HTTP server with two main endpoints:

1. `/metrics` - Standard Prometheus metrics endpoint for the exporter itself
2. `/probe` - Endpoint that accepts a `target` parameter, probes the specified WebSocket URL, and returns metrics about the connection

The probe process:

1. Establishes a WebSocket connection to the target
2. Measures connection time
3. Closes the connection
4. Returns metrics about the connection attempt

### Key Code Components

- **Connection Handling**: Uses the Gorilla WebSocket library to establish connections
- **Prometheus Integration**: Uses the Prometheus client library to expose metrics
- **VMProbe Compatibility**: Follows the same pattern as the blackbox exporter

## Deployment

### Building the Docker Image

```bash
# Clone the repository
git clone https://github.com/naviat/blockchain-websocket-exporter.git
cd blockchain-websocket-exporter

# Build the Docker image
docker build -t naviat/blockchain-websocket-exporter:latest .
```

### Local Testing with Kind

For local testing with a kind Kubernetes cluster, see the [LOCAL-KIND.md](LOCAL-KIND.md) guide, which includes:

- Setting up a kind cluster
- Installing Victoria Metrics Operator
- Deploying the WebSocket exporter
- Configuring VMProbe for monitoring
- Setting up Grafana for visualization

### Deploying in Kubernetes

1. Apply the Kubernetes deployment:

```bash
kubectl apply -f websocket-exporter-deployment.yaml
```

2. Create a VMProbe to monitor your WebSocket endpoints:

```bash
kubectl apply -f vmprobe-config.yaml
```

## Usage

### Testing the Exporter

You can test the exporter by sending a request to the probe endpoint:

```bash
curl "http://localhost:9095/probe?target=wss://your-blockchain-node.example.com/token"
```

The response will include metrics about the probe:

```config
# HELP probe_success Displays whether or not the probe was a success
# TYPE probe_success gauge
probe_success 1
# HELP probe_websocket_up Displays whether the WebSocket connection was successful
# TYPE probe_websocket_up gauge
probe_websocket_up 1
# HELP probe_websocket_connection_duration_seconds Duration of the WebSocket connection establishment
# TYPE probe_websocket_connection_duration_seconds gauge
probe_websocket_connection_duration_seconds 0.123
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
- `--debug` - Enable debug logging (default: `false`)

Example:

```bash
./blockchain-websocket-exporter --timeout=5s --debug
```

## VMProbe Configuration

The VMProbe configuration specifies which endpoints to monitor:

```yaml
apiVersion: operator.victoriametrics.com/v1beta1
kind: VMProbe
metadata:
  name: websocket-connection-probe
  namespace: victoria-metrics
spec:
  jobName: websocket-connection-monitoring
  vmProberSpec:
    url: websocket-exporter.victoria-metrics.svc:9095
    path: /probe
  interval: 30s
  targets:
    staticConfig:
      targets:
      - wss://bsc-websocket-endpoint/token
      - wss://eth-websocket-endpoint/token
      - wss://polygon-websocket-endpoint/token
      labels:
        service: blockchain-nodes
```

## Example Alerting Rules

Create alerts for WebSocket connection issues:

```yaml
apiVersion: operator.victoriametrics.com/v1beta1
kind: VMRule
metadata:
  name: websocket-connection-alerts
  namespace: victoria-metrics
spec:
  groups:
  - name: websocket
    rules:
    - alert: WebSocketConnectionDown
      expr: probe_websocket_up{job="websocket-connection-monitoring"} == 0
      for: 2m
      labels:
        severity: critical
      annotations:
        summary: "WebSocket connection to {{ $labels.instance }} is down"

    - alert: WebSocketHighConnectionLatency
      expr: probe_websocket_connection_duration_seconds{job="websocket-connection-monitoring"} > 0.5
      for: 5m
      labels:
        severity: warning
      annotations:
        summary: "WebSocket connection latency high for {{ $labels.instance }}"
```

## License

MIT
