package main

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"io"
	"log"
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

// mockWebSocketServer creates a test WebSocket server for testing
func mockWebSocketServer(t *testing.T) *httptest.Server {
	var upgrader = websocket.Upgrader{}

	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Simulate delay for connection duration testing
		time.Sleep(10 * time.Millisecond)

		c, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Fatalf("Failed to upgrade connection: %v", err)
			return
		}
		defer func() {
			err := c.Close()
			if err != nil {
				t.Fatalf("Failed to close connection: %v", err)
			}
		}()

		// Keep connection open briefly
		time.Sleep(5 * time.Millisecond)
	}))
}

// TestProbeWebSocket tests the probeWebSocket function
func TestProbeWebSocket(t *testing.T) {
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

	// Start mock WebSocket server
	server := mockWebSocketServer(t)
	defer server.Close()

	// Convert HTTP URL to WebSocket URL
	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")

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
			result := probeWebSocket(context.Background(), tc.target)

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
	// Start mock WebSocket server
	server := mockWebSocketServer(t)
	defer server.Close()

	// Convert HTTP URL to WebSocket URL
	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")

	// Test cases
	testCases := []struct {
		name           string
		target         string
		expectedStatus int
		checkBody      bool
	}{
		{
			name:           "Valid target",
			target:         wsURL,
			expectedStatus: http.StatusOK,
			checkBody:      true,
		},
		{
			name:           "Missing target",
			target:         "",
			expectedStatus: http.StatusBadRequest,
			checkBody:      false,
		},
		{
			name:           "Invalid target",
			target:         "invalid://url",
			expectedStatus: http.StatusOK, // The handler returns OK even for failed probes
			checkBody:      true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Create request
			req, err := http.NewRequest("GET", "/probe", nil)
			if err != nil {
				t.Fatalf("Failed to create request: %v", err)
			}

			// Add target parameter if provided
			if tc.target != "" {
				q := req.URL.Query()
				q.Add("target", tc.target)
				req.URL.RawQuery = q.Encode()
			}

			// Create response recorder
			rr := httptest.NewRecorder()

			// Call the handler
			handler := http.HandlerFunc(probeHandler)
			handler.ServeHTTP(rr, req)

			// Check status code
			if status := rr.Code; status != tc.expectedStatus {
				t.Errorf("Handler returned wrong status code: got %v want %v", status, tc.expectedStatus)
			}

			// Check response body for metrics
			if tc.checkBody {
				body := rr.Body.String()
				expectedMetrics := []string{
					"probe_success",
					"probe_websocket_up",
					"probe_websocket_connection_duration_seconds",
					"probe_duration_seconds",
				}

				for _, metric := range expectedMetrics {
					if !strings.Contains(body, metric) {
						t.Errorf("Response body missing metric: %s", metric)
					}
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

// TestDebugLog tests the debug logging function
func TestDebugLog(t *testing.T) {
	// Test with debug enabled
	*debug = true
	debugLog("Test debug message")

	// Test with debug disabled
	*debug = false
	debugLog("This should not cause any issues")

	// No assertions needed, just checking that it doesn't panic
}

// TestRootHandler tests the root endpoint handler
func TestRootHandler(t *testing.T) {
	// Save original metrics and probe paths
	origMetricsPath := *metricsPath
	origProbePath := *probePath

	// Set paths for test
	*metricsPath = "/testmetrics"
	*probePath = "/testprobe"

	// Create a request to the root endpoint
	req, err := http.NewRequest("GET", "/", nil)
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}

	// Create response recorder
	rr := httptest.NewRecorder()

	// Get the handler
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, err := w.Write([]byte(`<html>
			<head><title>WebSocket Connection Exporter</title></head>
			<body>
			<h1>WebSocket Connection Exporter</h1>
			<p>Visit <a href="` + *metricsPath + `">Metrics</a> to see metrics.</p>
			<p>Visit <a href="` + *probePath + `?target=wss://example.com/path/token">Probe</a> to probe a WebSocket endpoint.</p>
			<p>This exporter tests WebSocket connection establishment and measures connection latency.</p>
			</body>
			</html>`))
		if err != nil {
			log.Printf("Error writing response: %v", err)
		}
	})

	// Call the handler
	handler.ServeHTTP(rr, req)

	// Check status code
	if status := rr.Code; status != http.StatusOK {
		t.Errorf("Handler returned wrong status code: got %v want %v", status, http.StatusOK)
	}

	// Check response body
	body := rr.Body.String()
	if !strings.Contains(body, "WebSocket Connection Exporter") {
		t.Errorf("Response body missing expected content")
	}
	if !strings.Contains(body, *metricsPath) {
		t.Errorf("Response body doesn't contain metrics path")
	}
	if !strings.Contains(body, *probePath) {
		t.Errorf("Response body doesn't contain probe path")
	}

	// Restore original paths
	*metricsPath = origMetricsPath
	*probePath = origProbePath
}

// TestDebugLoggingEnabled tests debug logging when enabled
func TestDebugLoggingEnabled(t *testing.T) {
	// Save original debug value and restore it later
	origDebug := *debug
	defer func() { *debug = origDebug }()

	// Enable debug logging
	*debug = true

	// Capture log output
	var buf bytes.Buffer
	log.SetOutput(&buf)
	defer log.SetOutput(os.Stderr) // Restore default output

	// Call debug logging
	debugLog("Test debug message %d", 123)

	// Check log output
	if !strings.Contains(buf.String(), "Test debug message 123") {
		t.Errorf("Debug log didn't output expected message")
	}
}

// TestInvalidURLScheme tests handling of URLs with invalid schemes
func TestInvalidURLScheme(t *testing.T) {
	// Test with HTTP scheme (not ws/wss)
	result := probeWebSocket(context.Background(), "http://example.com")

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
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	// Test with cancelled context
	result := probeWebSocket(ctx, "ws://example.com")

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

func TestSetupServer(t *testing.T) {
	// Save original values
	origListenAddress := *listenAddress
	origMetricsPath := *metricsPath
	origProbePath := *probePath
	origDebug := *debug

	// Set test values
	*listenAddress = ":9999"
	*metricsPath = "/test-metrics"
	*probePath = "/test-probe"
	*debug = true

	// Capture stdout
	r, w, _ := os.Pipe()
	oldStdout := os.Stdout
	os.Stdout = w

	// Setup server
	server := setupServer()

	// Close stdout capture and restore original
	if err := w.Close(); err != nil {
		t.Logf("Error closing writer: %v", err)
	}
	os.Stdout = oldStdout

	// Read captured output
	var buf bytes.Buffer
	if _, err := buf.ReadFrom(r); err != nil {
		t.Fatalf("Failed to read from pipe: %v", err)
	}
	output := buf.String()

	// Verify server setup
	if server.Addr != *listenAddress {
		t.Errorf("Server address = %v, want %v", server.Addr, *listenAddress)
	}

	// Check stdout output
	if !strings.Contains(output, "Starting WebSocket Connection Exporter on "+*listenAddress) {
		t.Errorf("Missing expected server start message in output")
	}
	if !strings.Contains(output, "Probe endpoint: "+*probePath) {
		t.Errorf("Missing expected probe endpoint message in output")
	}

	// Test handlers by making requests to them
	testServer := httptest.NewServer(server.Handler)
	defer testServer.Close()

	// Test root handler
	resp, err := http.Get(testServer.URL + "/")
	if err != nil {
		t.Fatalf("Failed to GET /: %v", err)
	}
	defer func() {
		err := resp.Body.Close()
		if err != nil {
			t.Logf("Error closing response body: %v", err)
		}
	}()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Root handler status = %v, want %v", resp.StatusCode, http.StatusOK)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("Failed to read response body: %v", err)
	}

	if !strings.Contains(string(body), "WebSocket Connection Exporter") {
		t.Errorf("Root handler response missing expected content")
	}

	// Test metrics handler
	resp, err = http.Get(testServer.URL + *metricsPath)
	if err != nil {
		t.Fatalf("Failed to GET %s: %v", *metricsPath, err)
	}
	if err := resp.Body.Close(); err != nil {
		t.Logf("Error closing response body: %v", err)
	}

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Metrics handler status = %v, want %v", resp.StatusCode, http.StatusOK)
	}

	// Test probe handler without target (should return 400 Bad Request)
	resp, err = http.Get(testServer.URL + *probePath)
	if err != nil {
		t.Fatalf("Failed to GET %s: %v", *probePath, err)
	}
	if err := resp.Body.Close(); err != nil {
		t.Logf("Error closing response body: %v", err)
	}

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("Probe handler status for missing target = %v, want %v",
			resp.StatusCode, http.StatusBadRequest)
	}

	// Restore original values
	*listenAddress = origListenAddress
	*metricsPath = origMetricsPath
	*probePath = origProbePath
	*debug = origDebug
}

// TestMainFlagParsing tests that flag parsing works in main
func TestMainFlagParsing(t *testing.T) {
	// Save original args
	oldArgs := os.Args
	defer func() { os.Args = oldArgs }()

	// Set test args
	os.Args = []string{"cmd", "-web.listen-address=:8080", "-debug=true"}

	// Reset flags (necessary because flags might have been parsed in other tests)
	flag.CommandLine = flag.NewFlagSet(os.Args[0], flag.ExitOnError)

	// Re-declare your flags (this would normally happen at package level)
	listenAddress = flag.String("web.listen-address", ":9095", "Address to listen on for telemetry")
	metricsPath = flag.String("web.telemetry-path", "/metrics", "Path under which to expose metrics")
	probePath = flag.String("web.probe-path", "/probe", "Path under which to expose the probe endpoint")
	timeout = flag.Duration("timeout", 10*time.Second, "Timeout for probe")
	debug = flag.Bool("debug", false, "Enable debug logging")

	// Parse flags
	flag.Parse()

	// Verify flag values
	if *listenAddress != ":8080" {
		t.Errorf("listen address = %v, want :8080", *listenAddress)
	}

	if *debug != true {
		t.Errorf("debug = %v, want true", *debug)
	}
}
