package main

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestRunBindsBeforeLoggingListening(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve listener: %v", err)
	}
	defer listener.Close()

	octoprint := httptest.NewServer(http.NotFoundHandler())
	defer octoprint.Close()
	setRunEnvironment(t, listener.Addr().String(), octoprint.URL)

	var output bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&output, nil))
	if err := run(context.Background(), logger); err == nil {
		t.Fatal("expected bind error")
	}
	if strings.Contains(output.String(), "MCP server listening") {
		t.Fatalf("listening logged before bind succeeded: %s", output.String())
	}
}

func TestRunStopsAfterContextCancellation(t *testing.T) {
	octoprint := httptest.NewServer(http.NotFoundHandler())
	defer octoprint.Close()
	address := unusedLoopbackAddress(t)
	token := setRunEnvironment(t, address, octoprint.URL)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		done <- run(ctx, slog.New(slog.NewTextHandler(io.Discard, nil)))
	}()
	waitForListener(t, done, address)
	transport := &mcp.StreamableClientTransport{
		Endpoint:   "http://" + address + "/mcp",
		HTTPClient: &http.Client{Transport: runBearerTransport{token: token}},
	}
	client := mcp.NewClient(&mcp.Implementation{Name: "tupi-mcp-test", Version: "0.0.0"}, nil)
	session, err := client.Connect(context.Background(), transport, nil)
	if err != nil {
		t.Fatalf("connect MCP session: %v", err)
	}
	defer session.Close()
	cancel()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("run returned an error during graceful shutdown: %v", err)
		}
	case <-time.After(shutdownTimeout + time.Second):
		t.Fatal("run did not stop after context cancellation")
	}
}

func setRunEnvironment(t *testing.T, address, octoprintURL string) string {
	t.Helper()
	token := strings.Repeat("a", 32)
	t.Setenv("TUPI_MCP_TOKEN", token)
	t.Setenv("TUPI_MCP_LISTEN_ADDRESS", address)
	t.Setenv("TUPI_OCTOPRINT_BASE_URL", octoprintURL)
	t.Setenv("TUPI_OCTOPRINT_API_KEY", "test-api-key")
	return token
}

type runBearerTransport struct {
	token string
}

func (t runBearerTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	clone := request.Clone(request.Context())
	clone.Header.Set("Authorization", "Bearer "+t.token)
	return http.DefaultTransport.RoundTrip(clone)
}

func unusedLoopbackAddress(t *testing.T) string {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve listener: %v", err)
	}
	address := listener.Addr().String()
	if err := listener.Close(); err != nil {
		t.Fatalf("close reserved listener: %v", err)
	}
	return address
}

func waitForListener(t *testing.T, done <-chan error, address string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		connection, err := net.DialTimeout("tcp", address, 50*time.Millisecond)
		if err == nil {
			_ = connection.Close()
			return
		}
		select {
		case err := <-done:
			t.Fatalf("run exited before listening: %v", err)
		case <-time.After(10 * time.Millisecond):
		}
	}
	t.Fatal("server did not start listening")
}
