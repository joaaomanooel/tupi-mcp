package octoprint

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestApprovedReads(t *testing.T) {
	for _, endpoint := range []string{"connection", "printer", "job", "files"} {
		t.Run(endpoint, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet || r.URL.Path != "/api/"+endpoint {
					t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
				}
				if r.Header.Get("X-Api-Key") != "test-secret" {
					t.Error("missing credential header")
				}
				if r.URL.RawQuery != "" || strings.Contains(r.URL.String(), "test-secret") {
					t.Error("credential in URL")
				}
				fmt.Fprint(w, `{"ready":true}`)
			}))
			defer server.Close()
			client := newTestClient(t, server.URL, 128)
			methods := map[string]func(context.Context) ([]byte, error){"connection": client.Connection, "printer": client.Printer, "job": client.Job, "files": client.Files}
			body, err := methods[endpoint](context.Background())
			if err != nil || string(body) != `{"ready":true}` {
				t.Fatalf("unexpected result: %q %v", body, err)
			}
		})
	}
}

func TestConfigurationValidation(t *testing.T) {
	for _, base := range []string{"", "localhost", "ftp://localhost", "http://", "http://user:pass@localhost", "http://localhost?x=1", "http://localhost#fragment", "http://localhost?", "http://localhost/#"} {
		t.Run(base, func(t *testing.T) {
			_, err := NewClient(base, "test-secret", &http.Client{Timeout: time.Second}, 128)
			assertKind(t, err, ErrorConfig)
		})
	}
	for _, key := range []string{"", "  "} {
		_, err := NewClient("http://localhost", key, &http.Client{Timeout: time.Second}, 128)
		assertKind(t, err, ErrorConfig)
	}
	for _, transport := range []*http.Client{nil, {}, {Timeout: -time.Second}} {
		_, err := NewClient("http://localhost", "test-secret", transport, 128)
		assertKind(t, err, ErrorConfig)
	}
	for _, cap := range []int64{0, -1} {
		_, err := NewClient("http://localhost", "test-secret", &http.Client{Timeout: time.Second}, cap)
		assertKind(t, err, ErrorConfig)
	}
}

func TestResponseValidation(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		status     int
		cap        int64
		kind       ErrorKind
	}{
		{"status", `test-secret upstream details`, 409, 128, ErrorHTTPStatus},
		{"malformed", `test-secret`, 200, 128, ErrorInvalidJSON},
		{"trailing", `{} {}`, 200, 128, ErrorInvalidJSON},
		{"empty", "", 204, 128, ErrorInvalidJSON},
		{"oversized", `{"long":"content"}`, 200, 4, ErrorResponseTooLarge},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(tc.status); fmt.Fprint(w, tc.body) }))
			defer server.Close()
			body, err := newTestClient(t, server.URL, tc.cap).Job(context.Background())
			if body != nil {
				t.Error("error returned body")
			}
			assertKind(t, err, tc.kind)
			if strings.Contains(err.Error(), "test-secret") || strings.Contains(err.Error(), tc.body) && tc.body != "" {
				t.Error("unsafe error detail")
			}
			if tc.kind == ErrorHTTPStatus {
				var typed *Error
				errors.As(err, &typed)
				if typed.StatusCode != 409 {
					t.Error("missing HTTP status")
				}
			}
		})
	}
}

func TestExactBodyLimit(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, `{}`) }))
	defer server.Close()
	if _, err := newTestClient(t, server.URL, 2).Files(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestRedirectDoesNotForwardCredentials(t *testing.T) {
	var reached atomic.Bool
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { reached.Store(true); fmt.Fprint(w, `{}`) }))
	defer target.Close()
	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, target.URL, http.StatusFound) }))
	defer source.Close()
	_, err := newTestClient(t, source.URL, 128).Connection(context.Background())
	assertKind(t, err, ErrorHTTPStatus)
	if reached.Load() {
		t.Error("redirect followed")
	}
}

func TestCancellationAndTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { <-r.Context().Done() }))
	defer server.Close()
	client, err := NewClient(server.URL, "test-secret", &http.Client{Timeout: 30 * time.Millisecond}, 128)
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.Printer(context.Background())
	assertKind(t, err, ErrorTimeout)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Error("deadline cause missing")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = client.Printer(ctx)
	assertKind(t, err, ErrorCanceled)
	if !errors.Is(err, context.Canceled) {
		t.Error("cancellation cause missing")
	}
}

func TestBasePathAndClientIsolation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/octoprint/api/files" {
			t.Error("base path not preserved")
		}
		fmt.Fprint(w, `{}`)
	}))
	defer server.Close()
	transport := &http.Client{Timeout: time.Second}
	client, err := NewClient(server.URL+"/octoprint/", "test-secret", transport, 128)
	if err != nil {
		t.Fatal(err)
	}
	if transport.CheckRedirect != nil {
		t.Error("caller client modified")
	}
	if _, err = client.Files(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func newTestClient(t *testing.T, base string, cap int64) *Client {
	t.Helper()
	client, err := NewClient(base, "test-secret", &http.Client{Timeout: time.Second}, cap)
	if err != nil {
		t.Fatal(err)
	}
	return client
}

func assertKind(t *testing.T, err error, kind ErrorKind) {
	t.Helper()
	var typed *Error
	if !errors.As(err, &typed) || typed.Kind != kind {
		t.Fatalf("expected %s error, got %v", kind, err)
	}
}

func TestContextDeadline(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { <-r.Context().Done() }))
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	_, err := newTestClient(t, server.URL, 128).Printer(ctx)
	assertKind(t, err, ErrorTimeout)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("missing deadline cause: %v", err)
	}
}

func TestCredentialReflectionIsRejected(t *testing.T) {
	for _, body := range []string{`{"value":"test-secret"}`, `{"test-secret":true}`, `{"value":"test-\u0073ecret"}`} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, body) }))
		result, err := newTestClient(t, server.URL, 128).Files(context.Background())
		server.Close()
		if result != nil || err == nil {
			t.Fatal("reflected credential was returned")
		}
		if strings.Contains(err.Error(), "test-secret") {
			t.Error("credential in error")
		}
	}
}

type failingTransport struct{}

func (failingTransport) RoundTrip(*http.Request) (*http.Response, error) {
	return nil, errors.New("test-secret transport detail")
}

func TestTransportErrorsAreRedacted(t *testing.T) {
	client, err := NewClient("http://localhost", "test-secret", &http.Client{Timeout: time.Second, Transport: failingTransport{}}, 128)
	if err != nil {
		t.Fatal(err)
	}
	body, err := client.Job(context.Background())
	assertKind(t, err, ErrorTransport)
	if body != nil || strings.Contains(fmt.Sprintf("%+v", err), "test-secret") {
		t.Error("unsafe error")
	}
	if errors.Unwrap(err) != nil {
		t.Error("unsafe underlying transport error")
	}
}

func TestCancellationWhileReadingBody(t *testing.T) {
	started := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"partial":`)
		w.(http.Flusher).Flush()
		close(started)
		<-r.Context().Done()
	}))
	defer server.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { <-started; cancel() }()
	_, err := newTestClient(t, server.URL, 128).Job(ctx)
	assertKind(t, err, ErrorCanceled)
	if !errors.Is(err, context.Canceled) {
		t.Error("missing body cancellation cause")
	}
}

func TestValidJSONForms(t *testing.T) {
	for _, body := range []string{`[null,true,123,"value"]`, `123e10000`, ` {"ok":true} `} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, body) }))
		result, err := newTestClient(t, server.URL, 128).Files(context.Background())
		server.Close()
		if err != nil || string(result) != body {
			t.Fatalf("valid JSON rejected: %v", err)
		}
	}
}

func TestCredentialCannotAppearInBaseURL(t *testing.T) {
	for _, base := range []string{"http://localhost/test-secret", "http://test-secret.example", "http://localhost/test-%73ecret"} {
		_, err := NewClient(base, "test-secret", &http.Client{Timeout: time.Second}, 128)
		assertKind(t, err, ErrorConfig)
		if strings.Contains(err.Error(), "test-secret") {
			t.Error("credential in configuration error")
		}
	}
}

func TestCancellationDuringClosedBodyRead(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	client, err := NewClient("http://localhost", "test-secret", &http.Client{
		Timeout:   time.Second,
		Transport: closedBodyTransport{cancel: cancel},
	}, 128)
	if err != nil {
		t.Fatal(err)
	}
	body, err := client.Job(ctx)
	assertKind(t, err, ErrorCanceled)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("missing cancellation cause: %v", err)
	}
	if body != nil {
		t.Error("canceled read returned a body")
	}
}

type closedBodyTransport struct{ cancel context.CancelFunc }

func (t closedBodyTransport) RoundTrip(*http.Request) (*http.Response, error) {
	return &http.Response{StatusCode: http.StatusOK, Body: closedResponseBody{cancel: t.cancel}}, nil
}

type closedResponseBody struct{ cancel context.CancelFunc }

func (b closedResponseBody) Read([]byte) (int, error) {
	b.cancel()
	return 0, net.ErrClosed
}

func (closedResponseBody) Close() error { return nil }

func TestPaddedAPIKeyIsRejectedBeforeRequest(t *testing.T) {
	for _, key := range []string{" test-secret ", " test-secret", "test-secret ", "\ttest-secret\t"} {
		t.Run(fmt.Sprintf("%q", key), func(t *testing.T) {
			var requests atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests.Add(1)
				fmt.Fprint(w, `{}`)
			}))
			defer server.Close()
			client, err := NewClient(server.URL, key, &http.Client{Timeout: time.Second}, 128)
			assertKind(t, err, ErrorConfig)
			if client != nil {
				t.Error("invalid credential created a client")
			}
			if requests.Load() != 0 {
				t.Error("invalid credential sent a request")
			}
		})
	}
}
