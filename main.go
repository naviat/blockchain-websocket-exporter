package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"time"

	"github.com/gorilla/websocket"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	webListenAddress = flag.String("web.listen-address", ":9095", "Address to listen on")
	webTelemetryPath = flag.String("web.telemetry-path", "/metrics", "Path for exporter metrics")
	webProbePath     = flag.String("web.probe-path", "/probe", "Path for probe endpoint")
	timeout          = flag.Duration("timeout", 10*time.Second, "Probe timeout")
)

var (
	websocketUp = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "probe_websocket_up",
		Help: "Displays whether the WebSocket connection was successful",
	})

	websocketConnectionDuration = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "probe_websocket_connection_duration_seconds",
		Help: "Duration of the WebSocket connection establishment",
	})

	probeDuration = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "probe_duration_seconds",
		Help: "Returns how long the probe took to complete in seconds",
	})

	probeSuccess = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "probe_success",
		Help: "Displays whether or not the probe was a success",
	})
)

func init() {
	prometheus.MustRegister(websocketUp)
	prometheus.MustRegister(websocketConnectionDuration)
	prometheus.MustRegister(probeDuration)
	prometheus.MustRegister(probeSuccess)
}

func probeWebSocket(target string) bool {
	probeStart := time.Now()
	success := false
	defer func() {
		probeDuration.Set(time.Since(probeStart).Seconds())
		probeSuccess.Set(boolToFloat64(success))
	}()

	websocketUp.Set(0)
	websocketConnectionDuration.Set(0)

	targetURL, err := url.Parse(target)
	if err != nil {
		fmt.Printf("Invalid target URL %s: %v\n", target, err)
		return false
	}

	// Ensure URL uses ws:// or wss:// scheme
	if targetURL.Scheme != "ws" && targetURL.Scheme != "wss" {
		fmt.Printf("Invalid URL scheme %s, must be ws or wss\n", targetURL.Scheme)
		return false
	}

	// Create context with timeout
	ctxTimeout, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()

	dialer := websocket.Dialer{
		HandshakeTimeout: *timeout,
	}

	connectStart := time.Now()

	c, resp, err := dialer.DialContext(ctxTimeout, targetURL.String(), nil)
	if err != nil {
		if resp != nil {
			fmt.Printf("Failed to connect to %s: %v (HTTP status: %d)\n", targetURL.String(), err, resp.StatusCode)
		} else {
			fmt.Printf("Failed to connect to %s: %v\n", targetURL.String(), err)
		}
		return false
	}
	defer func() {
		err := c.Close()
		if err != nil {
			fmt.Printf("Error closing connection: %v\n", err)
		}
	}()

	// Record connection metrics
	connectionDuration := time.Since(connectStart)
	websocketConnectionDuration.Set(connectionDuration.Seconds())
	websocketUp.Set(1)
	fmt.Printf("Connected to %s in %s\n", targetURL.String(), connectionDuration)

	// Consider the probe successful if the connection was established
	success = true
	return success
}

func probeHandler(w http.ResponseWriter, r *http.Request) {
	target := r.URL.Query().Get("target")
	if target == "" {
		http.Error(w, "Target parameter is missing", http.StatusBadRequest)
		return
	}

	// Create a fresh registry for this probe
	registry := prometheus.NewRegistry()
	registry.MustRegister(websocketUp)
	registry.MustRegister(websocketConnectionDuration)
	registry.MustRegister(probeDuration)
	registry.MustRegister(probeSuccess)

	probeWebSocket(target)

	// Return metrics
	h := promhttp.HandlerFor(registry, promhttp.HandlerOpts{})
	h.ServeHTTP(w, r)
}

func boolToFloat64(b bool) float64 {
	if b {
		return 1
	}
	return 0
}

func main() {
	flag.Parse()

	// Setup HTTP server
	http.Handle(*webTelemetryPath, promhttp.Handler())
	http.HandleFunc(*webProbePath, probeHandler)
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if _, err := w.Write([]byte(`<html>
			<head><title>WebSocket Exporter</title></head>
			<body>
			<h1>WebSocket Exporter</h1>
			<p><a href="` + *webProbePath + `">Probe</a></p>
			<p><a href="` + *webTelemetryPath + `">Metrics</a></p>
			</body>
			</html>`)); err != nil {
			log.Printf("Error writing response: %v", err)
		}
	})

	log.Printf("Starting websocket exporter on %s", *webListenAddress)
	if err := http.ListenAndServe(*webListenAddress, nil); err != nil {
		log.Fatalf("Error starting HTTP server: %v", err)
	}
}
