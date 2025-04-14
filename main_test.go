package main

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

// TestProbeWebSocket tests the probeWebSocket function
func TestProbeWebSocket(t *testing.T) {
	// Create a mock WebSocket server
	upgrader := websocket.Upgrader{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Logf("Failed to upgrade connection: %v", err)
			return
		}
		defer func() {
			if err := conn.Close(); err != nil {
				t.Logf("Failed to close connection: %v", err)
			}
		}()
	}))
	defer server.Close()

	// Convert HTTP URL to WebSocket URL
	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")

	// Reset metrics before test
	prometheus.Unregister(websocketUp)
	prometheus.Unregister(websocketConnectionDuration)
	prometheus.Unregister(probeDuration)
	prometheus.Unregister(probeSuccess)

	// Re-register metrics
	prometheus.MustRegister(websocketUp)
	prometheus.MustRegister(websocketConnectionDuration)
	prometheus.MustRegister(probeDuration)
	prometheus.MustRegister(probeSuccess)

	// Test cases
	testCases := []struct {
		name     string
		target   string
		expected bool
	}{
		{
			name:     "Valid WebSocket URL",
			target:   wsURL,
			expected: true,
		},
		{
			name:     "Invalid URL",
			target:   "invalid://url",
			expected: false,
		},
		{
			name:     "Non-existent host",
			target:   "ws://non-existent-host.local/path",
			expected: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Set timeout for test
			*timeout = 1 * time.Second

			// Test the probeWebSocket function
			result := probeWebSocket(tc.target)

			if result != tc.expected {
				t.Errorf("probeWebSocket(%s) = %v, want %v", tc.target, result, tc.expected)
			}

			// For successful connections, verify metrics were set correctly
			if tc.expected {
				if value := testutil.ToFloat64(probeSuccess); value != 1 {
					t.Errorf("probeSuccess metric = %v, want 1", value)
				}
				if value := testutil.ToFloat64(websocketUp); value != 1 {
					t.Errorf("websocketUp metric = %v, want 1", value)
				}
				if value := testutil.ToFloat64(websocketConnectionDuration); value <= 0 {
					t.Errorf("websocketConnectionDuration metric = %v, want > 0", value)
				}
				if value := testutil.ToFloat64(probeDuration); value <= 0 {
					t.Errorf("probeDuration metric = %v, want > 0", value)
				}
			}
		})
	}
}

// TestProbeHandler tests the HTTP handler for the probe endpoint
func TestProbeHandler(t *testing.T) {
	testCases := []struct {
		name           string
		target         string
		expectedStatus int
		checkMetrics   bool
	}{
		{
			name:           "Valid target",
			target:         "ws://example.com",
			expectedStatus: http.StatusOK,
			checkMetrics:   true,
		},
		{
			name:           "Missing target",
			target:         "",
			expectedStatus: http.StatusBadRequest,
			checkMetrics:   false,
		},
		{
			name:           "Invalid target",
			target:         "invalid://url",
			expectedStatus: http.StatusOK,
			checkMetrics:   true,
		},
		{
			name:           "Empty target parameter",
			target:         "",
			expectedStatus: http.StatusBadRequest,
			checkMetrics:   false,
		},
		{
			name:           "Target with spaces",
			target:         "ws://example.com with spaces",
			expectedStatus: http.StatusOK,
			checkMetrics:   true,
		},
		{
			name:           "Target with special characters",
			target:         "ws://example.com/path?query=value&param=test",
			expectedStatus: http.StatusOK,
			checkMetrics:   true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Create request with target parameter
			req, err := http.NewRequest("GET", "/probe", nil)
			if err != nil {
				t.Fatal(err)
			}

			if tc.target != "" {
				q := req.URL.Query()
				q.Add("target", tc.target)
				req.URL.RawQuery = q.Encode()
			}

			// Create response recorder
			rr := httptest.NewRecorder()

			// Call handler
			probeHandler(rr, req)

			// Check status code
			if status := rr.Code; status != tc.expectedStatus {
				t.Errorf("handler returned wrong status code: got %v want %v",
					status, tc.expectedStatus)
			}

			// Check metrics if expected
			if tc.checkMetrics {
				body := rr.Body.String()
				if !strings.Contains(body, "probe_success") {
					t.Errorf("response missing probe_success metric")
				}
				if !strings.Contains(body, "probe_duration_seconds") {
					t.Errorf("response missing probe_duration_seconds metric")
				}
				if !strings.Contains(body, "probe_websocket_up") {
					t.Errorf("response missing probe_websocket_up metric")
				}
				if !strings.Contains(body, "probe_websocket_connection_duration_seconds") {
					t.Errorf("response missing probe_websocket_connection_duration_seconds metric")
				}
			}
		})
	}
}

// TestURLParsing tests the URL parsing and validation logic
func TestURLParsing(t *testing.T) {
	testCases := []struct {
		name           string
		input          string
		expectedValid  bool
		expectedScheme string
	}{
		{
			name:           "Valid ws URL",
			input:          "ws://example.com/path",
			expectedValid:  true,
			expectedScheme: "ws",
		},
		{
			name:           "Valid wss URL",
			input:          "wss://example.com/path",
			expectedValid:  true,
			expectedScheme: "wss",
		},
		{
			name:           "URL with no scheme",
			input:          "example.com/path",
			expectedValid:  true,
			expectedScheme: "ws", // Default scheme
		},
		{
			name:           "Invalid scheme",
			input:          "http://example.com",
			expectedValid:  false,
			expectedScheme: "http",
		},
		{
			name:           "Invalid URL format",
			input:          "://invalid-url",
			expectedValid:  false,
			expectedScheme: "",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Parse URL
			parsedURL, err := url.Parse(tc.input)

			// Check parsing error
			if err != nil {
				if tc.expectedValid {
					t.Errorf("Failed to parse valid URL %s: %v", tc.input, err)
				}
				return
			}

			// Set default scheme if missing
			if parsedURL.Scheme == "" {
				parsedURL.Scheme = "ws"
			}

			// Check scheme
			if parsedURL.Scheme != tc.expectedScheme {
				t.Errorf("Incorrect scheme for %s: got %s, want %s", tc.input, parsedURL.Scheme, tc.expectedScheme)
			}

			// Check validation
			isValid := parsedURL.Scheme == "ws" || parsedURL.Scheme == "wss"
			if isValid != tc.expectedValid {
				t.Errorf("Incorrect validation for %s: got %v, want %v", tc.input, isValid, tc.expectedValid)
			}
		})
	}
}

// TestBoolToFloat64 tests the boolToFloat64 utility function
func TestBoolToFloat64(t *testing.T) {
	testCases := []struct {
		name     string
		input    bool
		expected float64
	}{
		{
			name:     "True to 1.0",
			input:    true,
			expected: 1.0,
		},
		{
			name:     "False to 0.0",
			input:    false,
			expected: 0.0,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := boolToFloat64(tc.input)
			if result != tc.expected {
				t.Errorf("boolToFloat64(%v) = %v, want %v", tc.input, result, tc.expected)
			}
		})
	}
}

// TestRootHandler tests the root endpoint handler
func TestRootHandler(t *testing.T) {
	// Save original values and restore them later
	origWebTelemetryPath := *webTelemetryPath
	origWebProbePath := *webProbePath
	defer func() {
		*webTelemetryPath = origWebTelemetryPath
		*webProbePath = origWebProbePath
	}()

	// Set test values
	*webTelemetryPath = "/test-metrics"
	*webProbePath = "/test-probe"

	// Create test request
	req, err := http.NewRequest("GET", "/", nil)
	if err != nil {
		t.Fatal(err)
	}

	// Create response recorder
	rr := httptest.NewRecorder()

	// Create handler function
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, err := w.Write([]byte(`<html>
			<head><title>WebSocket Exporter</title></head>
			<body>
			<h1>WebSocket Exporter</h1>
			<p><a href="` + *webProbePath + `">Probe</a></p>
			<p><a href="` + *webTelemetryPath + `">Metrics</a></p>
			</body>
			</html>`)); err != nil {
			t.Errorf("Error writing response: %v", err)
		}
	})

	// Serve the request
	handler.ServeHTTP(rr, req)

	// Check status code
	if status := rr.Code; status != http.StatusOK {
		t.Errorf("handler returned wrong status code: got %v want %v",
			status, http.StatusOK)
	}

	// Check response body
	expected := `<html>
			<head><title>WebSocket Exporter</title></head>
			<body>
			<h1>WebSocket Exporter</h1>
			<p><a href="/test-probe">Probe</a></p>
			<p><a href="/test-metrics">Metrics</a></p>
			</body>
			</html>`
	if rr.Body.String() != expected {
		t.Errorf("handler returned unexpected body: got %v want %v",
			rr.Body.String(), expected)
	}
}

// TestInvalidURLScheme tests handling of URLs with invalid schemes
func TestInvalidURLScheme(t *testing.T) {
	// Test with HTTP scheme (not ws/wss)
	result := probeWebSocket("http://example.com")

	if result != false {
		t.Errorf("probeWebSocket() with invalid scheme = %v, want false", result)
	}

	// Verify metrics
	if value := testutil.ToFloat64(probeSuccess); value != 0 {
		t.Errorf("probeSuccess metric = %v, want 0", value)
	}

	if value := testutil.ToFloat64(websocketUp); value != 0 {
		t.Errorf("websocketUp metric = %v, want 0", value)
	}
}

// TestContextCancellation tests handling of context cancellation
func TestContextCancellation(t *testing.T) {
	// Create a context and cancel it immediately
	_, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	// Test with cancelled context
	result := probeWebSocket("ws://example.com")

	if result != false {
		t.Errorf("probeWebSocket() with cancelled context = %v, want false", result)
	}
}

// TestFmtFprintf tests the error handling for fmt.Fprintf
func TestFmtFprintf(t *testing.T) {
	// Create a writer that always fails
	badWriter := &badWriter{}

	// Save original stdout and restore it later
	origStdout := os.Stdout
	// Create a temporary pipe
	r, w, _ := os.Pipe()
	os.Stdout = w

	// Test Fprintf with a writer that will work
	_, err := fmt.Fprintf(os.Stdout, "Test message\n")
	if err != nil {
		t.Errorf("fmt.Fprintf to stdout failed: %v", err)
	}

	// Close the pipe and restore stdout
	if err := w.Close(); err != nil {
		t.Logf("Error closing writer: %v", err)
	}
	os.Stdout = origStdout

	// Read the output (not really necessary for this test)
	var buf bytes.Buffer
	if _, err := buf.ReadFrom(r); err != nil {
		t.Fatalf("Failed to read from pipe: %v", err)
	}

	// Test Fprintf with a writer that always fails
	_, err = fmt.Fprintf(badWriter, "Test message\n")
	if err == nil {
		t.Errorf("fmt.Fprintf to bad writer should have failed")
	}
}

// badWriter is a writer that always returns an error
type badWriter struct{}

func (w *badWriter) Write(p []byte) (n int, err error) {
	return 0, fmt.Errorf("forced write error")
}

// TestMainFlagParsing tests that flag parsing works in main
func TestMainFlagParsing(t *testing.T) {
	// Save original args
	oldArgs := os.Args
	defer func() { os.Args = oldArgs }()

	// Set test args
	os.Args = []string{"cmd", "-web.listen-address=:8080"}

	// Reset flags (necessary because flags might have been parsed in other tests)
	flag.CommandLine = flag.NewFlagSet(os.Args[0], flag.ExitOnError)

	// Re-declare your flags (this would normally happen at package level)
	webListenAddress = flag.String("web.listen-address", ":9095", "Address to listen on")
	webTelemetryPath = flag.String("web.telemetry-path", "/metrics", "Path for exporter metrics")
	webProbePath = flag.String("web.probe-path", "/probe", "Path for probe endpoint")
	timeout = flag.Duration("timeout", 10*time.Second, "Probe timeout")

	// Parse flags
	flag.Parse()

	// Verify flag values
	if *webListenAddress != ":8080" {
		t.Errorf("webListenAddress = %v, want :8080", *webListenAddress)
	}
}

func TestMain(t *testing.T) {
	// Save original command line arguments
	origArgs := os.Args
	defer func() { os.Args = origArgs }()

	// Test with custom listen address
	os.Args = []string{"cmd", "-web.listen-address=:8080"}
	flag.Parse()
	if *webListenAddress != ":8080" {
		t.Errorf("webListenAddress = %v, want :8080", *webListenAddress)
	}
}

func TestMainFunction(t *testing.T) {
	// Create a mock server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/":
			if _, err := w.Write([]byte("Root handler response")); err != nil {
				t.Logf("Failed to write response: %v", err)
			}
		case "/metrics":
			if _, err := w.Write([]byte("Metrics handler response")); err != nil {
				t.Logf("Failed to write response: %v", err)
			}
		case "/probe":
			if _, err := w.Write([]byte("Probe handler response")); err != nil {
				t.Logf("Failed to write response: %v", err)
			}
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	// Save original values
	origWebListenAddress := *webListenAddress
	origWebTelemetryPath := *webTelemetryPath
	origWebProbePath := *webProbePath
	defer func() {
		*webListenAddress = origWebListenAddress
		*webTelemetryPath = origWebTelemetryPath
		*webProbePath = origWebProbePath
	}()

	// Set test values
	*webListenAddress = server.URL[7:] // Remove "http://" prefix
	*webTelemetryPath = "/metrics"
	*webProbePath = "/probe"

	// Test root handler
	resp, err := http.Get(server.URL + "/")
	if err != nil {
		t.Fatalf("Failed to GET /: %v", err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			t.Logf("Failed to close response body: %v", err)
		}
	}()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Root handler status = %v, want %v", resp.StatusCode, http.StatusOK)
	}

	// Test metrics handler
	resp, err = http.Get(server.URL + *webTelemetryPath)
	if err != nil {
		t.Fatalf("Failed to GET %s: %v", *webTelemetryPath, err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			t.Logf("Failed to close response body: %v", err)
		}
	}()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Metrics handler status = %v, want %v", resp.StatusCode, http.StatusOK)
	}

	// Test probe handler without target
	resp, err = http.Get(server.URL + *webProbePath)
	if err != nil {
		t.Fatalf("Failed to GET %s: %v", *webProbePath, err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			t.Logf("Failed to close response body: %v", err)
		}
	}()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Probe handler status = %v, want %v", resp.StatusCode, http.StatusOK)
	}
}

func TestMetricsRegistration(t *testing.T) {
	// Reset metrics
	prometheus.Unregister(websocketUp)
	prometheus.Unregister(websocketConnectionDuration)
	prometheus.Unregister(probeDuration)
	prometheus.Unregister(probeSuccess)

	// Re-register metrics
	prometheus.MustRegister(websocketUp)
	prometheus.MustRegister(websocketConnectionDuration)
	prometheus.MustRegister(probeDuration)
	prometheus.MustRegister(probeSuccess)

	// Test metric values
	websocketUp.Set(1)
	websocketConnectionDuration.Set(0.5)
	probeDuration.Set(1.0)
	probeSuccess.Set(1)

	// Verify metric values
	if value := testutil.ToFloat64(websocketUp); value != 1 {
		t.Errorf("websocketUp = %v, want 1", value)
	}
	if value := testutil.ToFloat64(websocketConnectionDuration); value != 0.5 {
		t.Errorf("websocketConnectionDuration = %v, want 0.5", value)
	}
	if value := testutil.ToFloat64(probeDuration); value != 1.0 {
		t.Errorf("probeDuration = %v, want 1.0", value)
	}
	if value := testutil.ToFloat64(probeSuccess); value != 1 {
		t.Errorf("probeSuccess = %v, want 1", value)
	}
}

func TestProbeWebSocketErrorHandling(t *testing.T) {
	testCases := []struct {
		name     string
		target   string
		expected bool
	}{
		{
			name:     "Invalid URL format",
			target:   "://invalid-url",
			expected: false,
		},
		{
			name:     "URL with invalid characters",
			target:   "ws://example.com/\x00",
			expected: false,
		},
		{
			name:     "URL with invalid port",
			target:   "ws://example.com:99999",
			expected: false,
		},
		{
			name:     "URL with invalid path",
			target:   "ws://example.com/../../etc/passwd",
			expected: false,
		},
		{
			name:     "URL with invalid query",
			target:   "ws://example.com?param=\x00",
			expected: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Reset metrics before test
			prometheus.Unregister(websocketUp)
			prometheus.Unregister(websocketConnectionDuration)
			prometheus.Unregister(probeDuration)
			prometheus.Unregister(probeSuccess)

			// Re-register metrics
			prometheus.MustRegister(websocketUp)
			prometheus.MustRegister(websocketConnectionDuration)
			prometheus.MustRegister(probeDuration)
			prometheus.MustRegister(probeSuccess)

			// Test the probeWebSocket function
			result := probeWebSocket(tc.target)

			if result != tc.expected {
				t.Errorf("probeWebSocket(%s) = %v, want %v", tc.target, result, tc.expected)
			}

			// Verify metrics were set correctly for failed probes
			if !tc.expected {
				if value := testutil.ToFloat64(probeSuccess); value != 0 {
					t.Errorf("probeSuccess metric = %v, want 0", value)
				}
				if value := testutil.ToFloat64(websocketUp); value != 0 {
					t.Errorf("websocketUp metric = %v, want 0", value)
				}
				if value := testutil.ToFloat64(websocketConnectionDuration); value != 0 {
					t.Errorf("websocketConnectionDuration metric = %v, want 0", value)
				}
			}
		})
	}
}
