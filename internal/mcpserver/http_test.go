package mcpserver

import (
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/joaaomanooel/tupi-mcp/internal/config"
)

const initializeBody = `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-11-25","capabilities":{},"clientInfo":{"name":"test","version":"0"}}}`

func TestClientRequestsNeedNoCredential(t *testing.T) {
	setup := startMCPServer(t, readerStub{}, nil, 64<<10)
	response := postInitialize(t, setup, func(*http.Request) {})
	if response.StatusCode != http.StatusOK {
		t.Fatalf("status without client credential = %d, want %d", response.StatusCode, http.StatusOK)
	}
}

func TestHostValidation(t *testing.T) {
	setup := startMCPServer(t, readerStub{}, nil, 64<<10)
	for _, tc := range []struct {
		name       string
		host       string
		wantStatus int
	}{
		{name: "allowed", host: setup.host, wantStatus: http.StatusOK},
		{name: "foreign", host: "evil.example", wantStatus: http.StatusForbidden},
		{name: "loopback alias", host: "localhost:1234", wantStatus: http.StatusForbidden},
	} {
		t.Run(tc.name, func(t *testing.T) {
			response := postInitialize(t, setup, func(r *http.Request) {
				r.Host = tc.host
			})

			if response.StatusCode != tc.wantStatus {
				t.Fatalf("status = %d, want %d", response.StatusCode, tc.wantStatus)
			}
			if tc.wantStatus == http.StatusForbidden {
				assertBodyDoesNotDisclose(t, response, tc.host)
			}
		})
	}
}

func TestHostValidationProtectsEveryPath(t *testing.T) {
	setup := startMCPServer(t, readerStub{}, nil, 64<<10)

	response := get(t, setup, "/", func(r *http.Request) {
		r.Host = "evil.example"
	})

	if response.StatusCode != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", response.StatusCode, http.StatusForbidden)
	}
}

func TestOriginValidation(t *testing.T) {
	setup := startMCPServer(t, readerStub{}, []string{"https://client.example.com"}, 64<<10)
	for _, tc := range []struct {
		name       string
		addOrigin  func(*http.Request)
		wantStatus int
	}{
		{name: "absent", addOrigin: func(*http.Request) {}, wantStatus: http.StatusOK},
		{name: "allowed", addOrigin: func(r *http.Request) {
			r.Header.Set("Origin", "https://client.example.com")
		}, wantStatus: http.StatusOK},
		{name: "allowed with different case", addOrigin: func(r *http.Request) {
			r.Header.Set("Origin", "HTTPS://CLIENT.EXAMPLE.COM")
		}, wantStatus: http.StatusOK},
		{name: "foreign", addOrigin: func(r *http.Request) {
			r.Header.Set("Origin", "https://evil.example")
		}, wantStatus: http.StatusForbidden},
		{name: "null", addOrigin: func(r *http.Request) {
			r.Header.Set("Origin", "null")
		}, wantStatus: http.StatusForbidden},
		{name: "with path", addOrigin: func(r *http.Request) {
			r.Header.Set("Origin", "https://client.example.com/path")
		}, wantStatus: http.StatusForbidden},
		{name: "with scheme mismatch", addOrigin: func(r *http.Request) {
			r.Header.Set("Origin", "http://client.example.com")
		}, wantStatus: http.StatusForbidden},
		{name: "duplicate headers", addOrigin: func(r *http.Request) {
			r.Header.Set("Origin", "https://client.example.com")
			r.Header.Add("Origin", "https://client.example.com")
		}, wantStatus: http.StatusForbidden},
	} {
		t.Run(tc.name, func(t *testing.T) {
			response := postInitialize(t, setup, tc.addOrigin)

			if response.StatusCode != tc.wantStatus {
				t.Fatalf("status = %d, want %d", response.StatusCode, tc.wantStatus)
			}
		})
	}
}

func TestOriginDeniedWithoutAllowlist(t *testing.T) {
	setup := startMCPServer(t, readerStub{}, nil, 64<<10)

	response := postInitialize(t, setup, func(r *http.Request) {
		r.Header.Set("Origin", "https://client.example.com")
	})

	if response.StatusCode != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", response.StatusCode, http.StatusForbidden)
	}
}

func TestUnknownPathIsNotFound(t *testing.T) {
	setup := startMCPServer(t, readerStub{}, nil, 64<<10)

	for _, path := range []string{"/", "/mcp/", "/other"} {
		t.Run(path, func(t *testing.T) {
			response := get(t, setup, path, func(*http.Request) {})

			if response.StatusCode != http.StatusNotFound {
				t.Fatalf("status = %d, want %d", response.StatusCode, http.StatusNotFound)
			}
		})
	}
}

func TestRequestWithoutCredentialReachesMCPHandler(t *testing.T) {
	_, _, session := newPhysicalIntegrationServer(t, ToolReconnect)

	result := callTool(t, session, ToolReconnect, nil)

	if result == nil || result.IsError {
		t.Fatalf("physical tool call without a bearer token failed: %s", contentText(result))
	}

	setup := startMCPServer(t, readerStub{}, nil, 64<<10)
	response := postInitialize(t, setup, func(*http.Request) {})

	if response.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", response.StatusCode, http.StatusOK)
	}
}

func TestRequestBodyLimit(t *testing.T) {
	setup := startMCPServer(t, readerStub{}, nil, 512)
	payload := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-11-25","clientInfo":{"name":"` + strings.Repeat("a", 2048) + `","version":"0"}}}`

	response := post(t, setup, "/mcp", strings.NewReader(payload), func(r *http.Request) {
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("Accept", "application/json, text/event-stream")
	})

	if response.StatusCode != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, want %d", response.StatusCode, http.StatusRequestEntityTooLarge)
	}
	if recovered := postInitialize(t, setup, func(*http.Request) {}); recovered.StatusCode != http.StatusOK {
		t.Fatalf("server stopped serving after oversized request: %d", recovered.StatusCode)
	}
}

func TestOptionsValidation(t *testing.T) {
	server, err := NewServer(readerStub{}, nil, config.PhysicalLimits{})
	if err != nil {
		t.Fatalf("new server: %v", err)
	}
	for _, tc := range []struct {
		name    string
		options Options
	}{
		{name: "missing server", options: Options{AllowedHosts: []string{"127.0.0.1:8080"}, MaxRequestBodyBytes: 1024}},
		{name: "missing hosts", options: Options{Server: server, MaxRequestBodyBytes: 1024}},
		{name: "empty host", options: Options{Server: server, AllowedHosts: []string{""}, MaxRequestBodyBytes: 1024}},
		{name: "wildcard host", options: Options{Server: server, AllowedHosts: []string{"*.example.com"}, MaxRequestBodyBytes: 1024}},
		{name: "host with scheme", options: Options{Server: server, AllowedHosts: []string{"https://example.com"}, MaxRequestBodyBytes: 1024}},
		{name: "host with invalid port", options: Options{Server: server, AllowedHosts: []string{"example.com:0"}, MaxRequestBodyBytes: 1024}},
		{name: "unbracketed ipv6", options: Options{Server: server, AllowedHosts: []string{"::1:8080"}, MaxRequestBodyBytes: 1024}},
		{name: "wildcard origin", options: Options{Server: server, AllowedHosts: []string{"127.0.0.1:8080"}, AllowedOrigins: []string{"*"}, MaxRequestBodyBytes: 1024}},
		{name: "origin with path", options: Options{Server: server, AllowedHosts: []string{"127.0.0.1:8080"}, AllowedOrigins: []string{"https://example.com/path"}, MaxRequestBodyBytes: 1024}},
		{name: "origin without scheme", options: Options{Server: server, AllowedHosts: []string{"127.0.0.1:8080"}, AllowedOrigins: []string{"example.com"}, MaxRequestBodyBytes: 1024}},
		{name: "missing body limit", options: Options{Server: server, AllowedHosts: []string{"127.0.0.1:8080"}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			handler, err := NewHandler(tc.options)

			if err == nil || handler != nil {
				t.Fatalf("expected configuration error, got %v %v", handler, err)
			}
		})
	}
}

func TestOptionsAcceptsExactAllowlists(t *testing.T) {
	server, err := NewServer(readerStub{}, nil, config.PhysicalLimits{})
	if err != nil {
		t.Fatalf("new server: %v", err)
	}
	handler, err := NewHandler(Options{
		Server:              server,
		AllowedHosts:        []string{"127.0.0.1:8080", "localhost:8080", "mcp.example.com", "[::1]"},
		AllowedOrigins:      []string{"https://client.example.com"},
		MaxRequestBodyBytes: 1024,
	})

	if err != nil || handler == nil {
		t.Fatalf("unexpected result: %v %v", handler, err)
	}
}

func TestCanonicalOrigin(t *testing.T) {
	for _, tc := range []struct {
		value string
		want  string
		ok    bool
	}{
		{value: "https://client.example.com", want: "https://client.example.com", ok: true},
		{value: "https://client.example.com:443", want: "https://client.example.com", ok: true},
		{value: "http://client.example.com:80", want: "http://client.example.com", ok: true},
		{value: "HTTPS://Client.Example.Com:8443", want: "https://client.example.com:8443", ok: true},
		{value: "http://127.0.0.1:3000", want: "http://127.0.0.1:3000", ok: true},
		{value: "null", ok: false},
		{value: "https://client.example.com/", ok: false},
		{value: "https://client.example.com/path", ok: false},
		{value: "https://client.example.com?query=1", ok: false},
		{value: "https://client.example.com:", ok: false},
		{value: "https://:443", ok: false},
		{value: "ftp://client.example.com", ok: false},
		{value: "https://user:pass@client.example.com", ok: false},
		{value: "*", ok: false},
		{value: "", ok: false},
	} {
		t.Run(tc.value, func(t *testing.T) {
			origin, ok := canonicalOrigin(tc.value)

			if ok != tc.ok || (ok && origin != tc.want) {
				t.Fatalf("canonicalOrigin(%q) = %q, %v", tc.value, origin, ok)
			}
		})
	}
}

func postInitialize(t *testing.T, setup testServer, mutate func(*http.Request)) *http.Response {
	t.Helper()
	return post(t, setup, "/mcp", strings.NewReader(initializeBody), func(r *http.Request) {
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("Accept", "application/json, text/event-stream")
		if mutate != nil {
			mutate(r)
		}
	})
}

func post(t *testing.T, setup testServer, path string, body io.Reader, mutate func(*http.Request)) *http.Response {
	t.Helper()
	request, err := http.NewRequest(http.MethodPost, setup.url+path, body)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	if mutate != nil {
		mutate(request)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	t.Cleanup(func() {
		_ = response.Body.Close()
	})
	return response
}

func get(t *testing.T, setup testServer, path string, mutate func(*http.Request)) *http.Response {
	t.Helper()
	request, err := http.NewRequest(http.MethodGet, setup.url+path, nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	mutate(request)
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	t.Cleanup(func() {
		_ = response.Body.Close()
	})
	return response
}

func assertBodyDoesNotDisclose(t *testing.T, response *http.Response, secret string) {
	t.Helper()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	if strings.Contains(string(body), secret) {
		t.Errorf("response body discloses %q: %s", secret, body)
	}
}
