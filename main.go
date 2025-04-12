package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"time"

	"github.com/gorilla/websocket"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Configuration options
var (
	listenAddress = flag.String("web.listen-address", ":9095", "Address to listen on for telemetry")
	metricsPath   = flag.String("web.telemetry-path", "/metrics", "Path under which to expose metrics")
	probePath     = flag.String("web.probe-path", "/probe", "Path under which to expose the probe endpoint")
	timeout       = flag.Duration("timeout", 10*time.Second, "Timeout for probe")
	debug         = flag.Bool("debug", false, "Enable debug logging")
)

// Prometheus metrics
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
	// Register metrics
	prometheus.MustRegister(websocketUp)
	prometheus.MustRegister(websocketConnectionDuration)
	prometheus.MustRegister(probeDuration)
	prometheus.MustRegister(probeSuccess)
}

// debugLog only logs if debug flag is enabled
func debugLog(format string, v ...interface{}) {
	if *debug {
		log.Printf(format, v...)
	}
}

func probeWebSocket(ctx context.Context, target string) bool {
	probeStart := time.Now()
	success := false
	defer func() {
		probeDuration.Set(time.Since(probeStart).Seconds())
		probeSuccess.Set(boolToFloat64(success))
	}()

	// Reset metrics for this probe
	websocketUp.Set(0)
	websocketConnectionDuration.Set(0)

	// Parse URL
	targetURL, err := url.Parse(target)
	if err != nil {
		log.Printf("Invalid target URL %s: %v", target, err)
		return false
	}

	// Set default scheme if missing
	if targetURL.Scheme == "" {
		targetURL.Scheme = "ws"
	}

	// Ensure URL uses ws:// or wss:// scheme
	if targetURL.Scheme != "ws" && targetURL.Scheme != "wss" {
		log.Printf("Invalid URL scheme %s, must be ws or wss", targetURL.Scheme)
		return false
	}

	// Create context with timeout
	ctxTimeout, cancel := context.WithTimeout(ctx, *timeout)
	defer cancel()

	// Set up connection
	dialer := websocket.Dialer{
		HandshakeTimeout: *timeout,
	}

	// Log connection attempt
	debugLog("Connecting to %s", targetURL.String())

	// Measure connection time
	connectStart := time.Now()

	// Connect to WebSocket server
	c, resp, err := dialer.DialContext(ctxTimeout, targetURL.String(), nil)
	if err != nil {
		if resp != nil {
			log.Printf("Failed to connect to %s: %v (HTTP status: %d)", targetURL.String(), err, resp.StatusCode)
		} else {
			log.Printf("Failed to connect to %s: %v", targetURL.String(), err)
		}
		return false
	}
	defer c.Close()

	// Record connection metrics
	connectionDuration := time.Since(connectStart)
	websocketConnectionDuration.Set(connectionDuration.Seconds())
	websocketUp.Set(1)
	debugLog("Connected to %s in %s", targetURL.String(), connectionDuration)

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

	debugLog("Received probe request for target: %s", target)

	// Create a fresh registry for this probe
	registry := prometheus.NewRegistry()
	registry.MustRegister(websocketUp)
	registry.MustRegister(websocketConnectionDuration)
	registry.MustRegister(probeDuration)
	registry.MustRegister(probeSuccess)

	// Probe the target
	start := time.Now()
	success := probeWebSocket(r.Context(), target)
	duration := time.Since(start)

	debugLog("Probe of %s completed in %s, success: %v", target, duration, success)

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

	if *debug {
		log.Println("Debug logging enabled")
	}

	// Set up HTTP server
	http.HandleFunc(*probePath, probeHandler)
	http.Handle(*metricsPath, promhttp.Handler())
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`<html>
			<head><title>WebSocket Connection Exporter</title></head>
			<body>
			<h1>WebSocket Connection Exporter</h1>
			<p>Visit <a href="` + *metricsPath + `">Metrics</a> to see metrics.</p>
			<p>Visit <a href="` + *probePath + `?target=wss://example.com/path/token">Probe</a> to probe a WebSocket endpoint.</p>
			<p>This exporter tests WebSocket connection establishment and measures connection latency.</p>
			</body>
			</html>`))
	})

	// Start server
	fmt.Fprintf(os.Stdout, "Starting WebSocket Connection Exporter on %s\n", *listenAddress)
	fmt.Fprintf(os.Stdout, "Probe endpoint: %s?target=wss://example.com/path/token\n", *probePath)
	log.Fatal(http.ListenAndServe(*listenAddress, nil))
}
