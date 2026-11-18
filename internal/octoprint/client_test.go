package octoprint

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/http/httptrace"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"testing/iotest"
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

			body, err := readEndpoint(client, endpoint, context.Background())

			if err != nil || body == nil {
				t.Fatalf("unexpected result: %+v %v", body, err)
			}
		})
	}
}

func TestUploadGCode(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/files/local" || r.Header.Get("X-Api-Key") != "test-secret" {
			t.Errorf("unexpected upload request: %s %s", r.Method, r.URL.Path)
		}
		if r.ContentLength <= 0 || !strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/form-data; boundary=") {
			t.Errorf("upload request missing length or multipart content type")
		}
		if r.FormValue("select") != "false" || r.FormValue("print") != "false" || r.FormValue("noOverwrite") != "true" {
			t.Errorf("upload flags select=%q print=%q noOverwrite=%q", r.FormValue("select"), r.FormValue("print"), r.FormValue("noOverwrite"))
		}
		file, header, err := r.FormFile("file")
		if err != nil {
			t.Errorf("upload file: %v", err)
			return
		}
		defer file.Close()
		contents, err := io.ReadAll(file)
		if err != nil || header.Filename != "first.gcode" || string(contents) != "G28\nM84\n" {
			t.Errorf("unexpected file %q: %q (%v)", header.Filename, contents, err)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		fmt.Fprint(w, `{"files":{"local":{"name":"first.gcode","path":"first.gcode","origin":"local"}},"done":true,"effectiveSelect":false,"effectivePrint":false}`)
	}))
	defer server.Close()

	result, err := newTestClient(t, server.URL, 1024).UploadGCode(context.Background(), "first.gcode", []byte("G28\nM84\n"))
	if err != nil || result == nil || result.Files["local"] == nil || result.EffectivePrint == nil || *result.EffectivePrint {
		t.Fatalf("unexpected upload response: %+v %v", result, err)
	}
}

func TestUploadGCodeReturnsSafeConflict(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.FormValue("noOverwrite") != "true" {
			t.Error("upload did not request overwrite protection")
		}
		w.WriteHeader(http.StatusConflict)
		fmt.Fprint(w, "existing-file-secret")
	}))
	defer server.Close()

	_, err := newTestClient(t, server.URL, 1024).UploadGCode(context.Background(), "first.gcode", []byte("G28"))
	assertKind(t, err, ErrorHTTPStatus)
	var typed *Error
	if !errors.As(err, &typed) || typed.StatusCode != http.StatusConflict || strings.Contains(err.Error(), "existing-file-secret") {
		t.Fatalf("unexpected conflict error: %v", err)
	}
}

func TestUploadGCodeRejectsInvalidInputWithoutRequest(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { requests.Add(1); fmt.Fprint(w, `{}`) }))
	defer server.Close()
	client := newTestClient(t, server.URL, 1024)

	for _, filename := range []string{"", "first", ".gcode", ".GCODE", "..gcode", "../first.gcode", `..\\first.gcode`, "first.stl", " first.gcode", "first.gcode\n", strings.Repeat("a", 250) + ".gcode"} {
		_, err := client.UploadGCode(context.Background(), filename, []byte("G28"))
		assertKind(t, err, ErrorInvalidUpload)
	}
	_, err := client.UploadGCode(context.Background(), "first.gcode", nil)
	assertKind(t, err, ErrorInvalidUpload)
	_, err = client.UploadGCode(context.Background(), "first.gcode", []byte("G28\x00M84"))
	assertKind(t, err, ErrorInvalidUpload)
	_, err = client.UploadGCode(context.Background(), "first.gcode", make([]byte, MaxGCodeUploadBytes+1))
	assertKind(t, err, ErrorRequestTooLarge)
	if requests.Load() != 0 {
		t.Fatalf("invalid input sent %d requests", requests.Load())
	}
}

func TestUploadGCodeFilenameLengthBoundary(t *testing.T) {
	if !validGCodeFilename(strings.Repeat("a", 249) + ".gcode") {
		t.Error("255-byte filename rejected")
	}
	if validGCodeFilename(strings.Repeat("a", 250) + ".gcode") {
		t.Error("256-byte filename accepted")
	}
}

func TestConfigurationValidation(t *testing.T) {
	for _, base := range []string{"", "localhost", "ftp://127.0.0.1", "http://", "http://user:pass@127.0.0.1", "http://127.0.0.1?x=1", "http://127.0.0.1#fragment", "http://127.0.0.1?", "http://127.0.0.1/#"} {
		t.Run(base, func(t *testing.T) {
			_, err := NewClient(base, "test-secret", &http.Client{Timeout: time.Second}, 128, false)
			assertKind(t, err, ErrorConfig)
		})
	}
	for _, key := range []string{"", "  "} {
		_, err := NewClient("http://127.0.0.1", key, &http.Client{Timeout: time.Second}, 128, false)
		assertKind(t, err, ErrorConfig)
	}
	for _, transport := range []*http.Client{nil, {}, {Timeout: -time.Second}} {
		_, err := NewClient("http://127.0.0.1", "test-secret", transport, 128, false)
		assertKind(t, err, ErrorConfig)
	}
	for _, cap := range []int64{0, -1} {
		_, err := NewClient("http://127.0.0.1", "test-secret", &http.Client{Timeout: time.Second}, cap, false)
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
	client, err := NewClient(server.URL, "test-secret", &http.Client{Timeout: 30 * time.Millisecond}, 128, false)
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
	client, err := NewClient(server.URL+"/octoprint/", "test-secret", transport, 128, false)
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
	client, err := NewClient(base, "test-secret", &http.Client{Timeout: time.Second}, cap, false)
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
		assertKind(t, err, ErrorSensitiveResponse)
		if strings.Contains(err.Error(), "test-secret") {
			t.Error("credential in error")
		}
	}
}

func TestBaseURLTransportPolicy(t *testing.T) {
	for _, tc := range []struct {
		base    string
		allowed bool
	}{
		{"http://127.0.0.1", true},
		{"http://127.42.0.1:5000/octoprint", true},
		{"http://[::1]:5000", true},
		{"http://[::ffff:127.0.0.1]", true},
		{"http://localhost", false},
		{"http://localhost.", false},
		{"http://octoprint.example", false},
		{"http://192.0.2.10:5000", false},
		{"http://0.0.0.0", false},
		{"http://[::]", false},
		{"http://[2001:db8::1]", false},
		{"http://127.1", false},
		{"http://2130706433", false},
		{"http://0177.0.0.1", false},
		{"http://[::1%25lo0]", false},
		{"https://octoprint.example:5000/octoprint", true},
		{"https://192.0.2.10", true},
		{"https://[2001:db8::1]", true},
	} {
		t.Run(tc.base, func(t *testing.T) {
			var requests atomic.Int32
			transport := roundTripFunc(func(r *http.Request) (*http.Response, error) {
				requests.Add(1)
				return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{}`))}, nil
			})
			client, err := NewClient(tc.base, "test-secret", &http.Client{Timeout: time.Second, Transport: transport}, 128, false)
			if tc.allowed {
				if err != nil || client == nil {
					t.Fatalf("allowed URL rejected: %v", err)
				}
				if _, err := client.Connection(context.Background()); err != nil {
					t.Fatal(err)
				}
				if requests.Load() != 1 {
					t.Fatal("allowed URL did not send one request")
				}
				return
			}
			if client != nil {
				_, _ = client.Connection(context.Background())
				t.Error("rejected URL created a client")
			}
			if requests.Load() != 0 {
				t.Error("rejected URL sent a request")
			}
			assertKind(t, err, ErrorConfig)
		})
	}
}

func TestInsecureHTTPRequiresExplicitOptIn(t *testing.T) {
	var requests atomic.Int32
	transport := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		requests.Add(1)
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{}`))}, nil
	})
	httpClient := &http.Client{Timeout: time.Second, Transport: transport}

	client, err := NewClient("http://octoprint.example:5000", "test-secret", httpClient, 128, false)
	if client != nil {
		t.Fatal("non-loopback HTTP URL created a client by default")
	}
	assertKind(t, err, ErrorConfig)
	if requests.Load() != 0 {
		t.Fatal("rejected URL sent a request")
	}

	client, err = NewClient("http://octoprint.example:5000", "test-secret", httpClient, 128, true)
	if err != nil || client == nil {
		t.Fatalf("explicit insecure HTTP opt-in was rejected: %v", err)
	}
	if _, err := client.Connection(context.Background()); err != nil {
		t.Fatal(err)
	}
	if requests.Load() != 1 {
		t.Fatalf("request count = %d, want 1", requests.Load())
	}
}

func TestHTTPSRead(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.TLS == nil || r.Method != http.MethodGet || r.URL.Path != "/api/connection" || r.Header.Get("X-Api-Key") != "test-secret" {
			t.Error("unexpected HTTPS request")
		}
		fmt.Fprint(w, `{}`)
	}))
	defer server.Close()
	httpClient := server.Client()
	httpClient.Timeout = time.Second
	client, err := NewClient(server.URL, "test-secret", httpClient, 128, false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.Connection(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestHTTPStatusBodyAllowsConnectionReuse(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if requests.Add(1) == 1 {
			w.WriteHeader(http.StatusServiceUnavailable)
			fmt.Fprint(w, "temporarily unavailable")
			return
		}
		fmt.Fprint(w, `{}`)
	}))
	defer server.Close()
	transport := &http.Transport{}
	defer transport.CloseIdleConnections()
	client, err := NewClient(server.URL, "test-secret", &http.Client{Timeout: time.Second, Transport: transport}, 128, false)
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.Job(context.Background())
	assertKind(t, err, ErrorHTTPStatus)
	var reused bool
	trace := &httptrace.ClientTrace{GotConn: func(info httptrace.GotConnInfo) { reused = info.Reused }}
	ctx := httptrace.WithClientTrace(context.Background(), trace)
	if _, err := client.Job(ctx); err != nil {
		t.Fatal(err)
	}
	if !reused || requests.Load() != 2 {
		t.Fatal("small error response prevented connection reuse")
	}
}

func TestHTTPStatusBodyDrainIsBoundedAndBestEffort(t *testing.T) {
	for _, tc := range []struct {
		name     string
		reader   io.Reader
		wantRead int
	}{
		{"small", strings.NewReader("upstream detail"), 15},
		{"large", strings.NewReader(strings.Repeat("x", 1<<20)), 32},
		{"read failure", io.MultiReader(strings.NewReader("detail"), iotest.ErrReader(errors.New("test-secret upstream detail"))), 6},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := &trackedResponseBody{Reader: tc.reader}
			transport := roundTripFunc(func(*http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: http.StatusServiceUnavailable, Body: body}, nil
			})
			client, err := NewClient("http://127.0.0.1", "test-secret", &http.Client{Timeout: time.Second, Transport: transport}, 32, false)
			if err != nil {
				t.Fatal(err)
			}
			result, err := client.Job(context.Background())
			assertKind(t, err, ErrorHTTPStatus)
			var typed *Error
			errors.As(err, &typed)
			if result != nil || typed.StatusCode != http.StatusServiceUnavailable || errors.Unwrap(err) != nil || err.Error() != "octoprint: http_status" {
				t.Fatalf("unsafe status result: %v", err)
			}
			if body.bytesRead != tc.wantRead || !body.closed {
				t.Fatalf("read %d bytes, want %d; closed: %t", body.bytesRead, tc.wantRead, body.closed)
			}
		})
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

type trackedResponseBody struct {
	io.Reader
	bytesRead int
	closed    bool
}

func (b *trackedResponseBody) Read(p []byte) (int, error) {
	n, err := b.Reader.Read(p)
	b.bytesRead += n
	return n, err
}

func (b *trackedResponseBody) Close() error {
	b.closed = true
	return nil
}

func TestConcurrentApprovedReadsShareClient(t *testing.T) {
	const readsPerEndpoint = 8
	endpoints := []string{"connection", "printer", "job", "files"}
	var requests [4]atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("unexpected method: %s", r.Method)
		}
		if r.Header.Get("X-Api-Key") != "test-secret" || r.URL.RawQuery != "" {
			t.Error("unexpected request credentials or query")
		}
		for index, endpoint := range endpoints {
			if r.URL.Path == "/api/"+endpoint {
				requests[index].Add(1)
				fmt.Fprint(w, `{}`)
				return
			}
		}
		t.Errorf("unexpected endpoint: %s", r.URL.Path)
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()
	client := newTestClient(t, server.URL, 128)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	start := make(chan struct{})
	var workers sync.WaitGroup
	for _, endpoint := range endpoints {
		for range readsPerEndpoint {
			workers.Go(func() {
				<-start
				response, err := readEndpoint(client, endpoint, ctx)
				if err != nil || response == nil {
					t.Errorf("concurrent %s read failed: %v", endpoint, err)
				}
			})
		}
	}
	close(start)
	workers.Wait()
	for index, endpoint := range endpoints {
		if got := requests[index].Load(); got != readsPerEndpoint {
			t.Errorf("%s received %d requests, want %d", endpoint, got, readsPerEndpoint)
		}
	}
}

type failingTransport struct{}

func (failingTransport) RoundTrip(*http.Request) (*http.Response, error) {
	return nil, errors.New("test-secret transport detail")
}

func TestTransportErrorsAreRedacted(t *testing.T) {
	client, err := NewClient("http://127.0.0.1", "test-secret", &http.Client{Timeout: time.Second, Transport: failingTransport{}}, 128, false)
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
		result, err := newTestClient(t, server.URL, 128).get(context.Background(), "/api/files")
		server.Close()
		if err != nil || string(result) != body {
			t.Fatalf("valid JSON rejected: %v", err)
		}
	}
}

func TestCredentialCannotAppearInBaseURL(t *testing.T) {
	for _, base := range []string{"http://127.0.0.1/test-secret", "https://test-secret.example", "http://127.0.0.1/test-%73ecret"} {
		_, err := NewClient(base, "test-secret", &http.Client{Timeout: time.Second}, 128, false)
		assertKind(t, err, ErrorConfig)
		if strings.Contains(err.Error(), "test-secret") {
			t.Error("credential in configuration error")
		}
	}
}

func TestCancellationDuringClosedBodyRead(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	client, err := NewClient("http://127.0.0.1", "test-secret", &http.Client{
		Timeout:   time.Second,
		Transport: closedBodyTransport{cancel: cancel},
	}, 128, false)
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

func TestCreateFolder(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/files/local" || r.Header.Get("X-Api-Key") != "test-secret" {
			t.Errorf("unexpected folder request: %s %s", r.Method, r.URL.Path)
		}
		if r.URL.RawQuery != "" || strings.Contains(r.URL.String(), "test-secret") {
			t.Error("credential in URL")
		}
		if !strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/form-data; boundary=") {
			t.Errorf("folder request missing multipart content type")
		}
		if err := r.ParseMultipartForm(1024); err != nil {
			t.Errorf("folder form: %v", err)
			return
		}
		if r.FormValue("foldername") != "parts" || r.FormValue("path") != "projects" {
			t.Errorf("folder fields foldername=%q path=%q", r.FormValue("foldername"), r.FormValue("path"))
		}
		if _, present := r.MultipartForm.File["file"]; present {
			t.Error("folder request included a file")
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		fmt.Fprint(w, `{"folder":{"name":"parts","path":"projects/parts","origin":"local","refs":{"resource":"https://octoprint.internal/api/files/local/projects/parts"}},"done":true}`)
	}))
	defer server.Close()

	result, err := newTestClient(t, server.URL, 1024).CreateFolder(context.Background(), "parts", "projects")
	if err != nil || result == nil || result.Folder == nil || result.Folder.Path == nil || *result.Folder.Path != "projects/parts" {
		t.Fatalf("unexpected folder response: %+v %v", result, err)
	}
	encoded, err := json.Marshal(result)
	if err != nil || strings.Contains(string(encoded), "https://octoprint.internal") {
		t.Fatalf("upstream URL was not filtered: %s (%v)", encoded, err)
	}
}

func TestCreateFolderWithoutParentOmitsPath(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseMultipartForm(1024); err != nil {
			t.Errorf("folder form: %v", err)
			return
		}
		if values := r.MultipartForm.Value["path"]; len(values) != 0 {
			t.Errorf("path field sent without parent: %q", values)
		}
		if r.FormValue("foldername") != "parts" {
			t.Errorf("foldername=%q", r.FormValue("foldername"))
		}
		w.WriteHeader(http.StatusCreated)
		fmt.Fprint(w, `{"folder":{"path":"parts"},"done":true}`)
	}))
	defer server.Close()

	result, err := newTestClient(t, server.URL, 1024).CreateFolder(context.Background(), "parts", "")
	if err != nil || result == nil || result.Folder == nil || result.Folder.Path == nil || *result.Folder.Path != "parts" {
		t.Fatalf("unexpected folder response: %+v %v", result, err)
	}
}

func TestCopyFile(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/files/local/projects/part one.gcode" || r.URL.EscapedPath() != "/api/files/local/projects/part%20one.gcode" {
			t.Errorf("unexpected copy request: %s %s", r.Method, r.URL.EscapedPath())
		}
		if r.Header.Get("X-Api-Key") != "test-secret" || r.URL.RawQuery != "" {
			t.Error("unexpected credentials")
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("content type = %q", r.Header.Get("Content-Type"))
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("copy body: %v", err)
			return
		}
		var command map[string]string
		if err := json.Unmarshal(body, &command); err != nil || len(command) != 2 || command["command"] != "copy" || command["destination"] != "archive/part one.gcode" {
			t.Errorf("unexpected copy body: %s", body)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		fmt.Fprint(w, `{"name":"part one.gcode","path":"archive/part one.gcode","origin":"local","refs":{"resource":"https://octoprint.internal/api/files/local/archive/part%20one.gcode","download":"https://octoprint.internal/downloads/files/local/archive/part%20one.gcode"}}`)
	}))
	defer server.Close()

	result, err := newTestClient(t, server.URL, 1024).CopyFile(context.Background(), "projects/part one.gcode", "archive/part one.gcode")
	if err != nil || result == nil || result.Path == nil || *result.Path != "archive/part one.gcode" {
		t.Fatalf("unexpected copy response: %+v %v", result, err)
	}
	encoded, err := json.Marshal(result)
	if err != nil || strings.Contains(string(encoded), "https://octoprint.internal") {
		t.Fatalf("upstream URL was not filtered: %s (%v)", encoded, err)
	}
}

func TestMoveFileToExistingFolderUsesEffectivePath(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/files/local/projects/part.gcode" || r.Header.Get("X-Api-Key") != "test-secret" {
			t.Errorf("unexpected move request: %s %s", r.Method, r.URL.Path)
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("move body: %v", err)
			return
		}
		var command map[string]string
		if err := json.Unmarshal(body, &command); err != nil || len(command) != 2 || command["command"] != "move" || command["destination"] != "archive" {
			t.Errorf("unexpected move body: %s", body)
		}
		w.WriteHeader(http.StatusCreated)
		fmt.Fprint(w, `{"name":"part.gcode","path":"archive/part.gcode","origin":"local","refs":{"resource":"https://octoprint.internal/api/files/local/archive/part.gcode"}}`)
	}))
	defer server.Close()

	result, err := newTestClient(t, server.URL, 1024).MoveFile(context.Background(), "projects/part.gcode", "archive")
	if err != nil || result == nil || result.Path == nil || *result.Path != "archive/part.gcode" {
		t.Fatalf("unexpected move response: %+v %v", result, err)
	}
}

func TestFileMutationsRejectInvalidPathsWithoutRequest(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		fmt.Fprint(w, `{}`)
	}))
	defer server.Close()
	client := newTestClient(t, server.URL, 1024)

	for _, name := range []string{"", ".", "..", "a/b", `a\b`, "a\x01b", "\xff"} {
		_, err := client.CreateFolder(context.Background(), name, "")
		assertKind(t, err, ErrorInvalidPath)
	}
	for _, parent := range []string{".", "..", "a//b", "/a", "a/", `a\b`, "a\x00b", "a/../b"} {
		_, err := client.CreateFolder(context.Background(), "parts", parent)
		assertKind(t, err, ErrorInvalidPath)
	}
	for _, invalid := range []string{"", ".", "..", "../a.gcode", "a//b.gcode", "/a.gcode", "a/", `dir\file.gcode`, "dir/\x01a.gcode", "dir/./a.gcode"} {
		_, err := client.CopyFile(context.Background(), invalid, "archive/a.gcode")
		assertKind(t, err, ErrorInvalidPath)
		_, err = client.CopyFile(context.Background(), "projects/a.gcode", invalid)
		assertKind(t, err, ErrorInvalidPath)
		_, err = client.MoveFile(context.Background(), invalid, "archive/a.gcode")
		assertKind(t, err, ErrorInvalidPath)
		_, err = client.MoveFile(context.Background(), "projects/a.gcode", invalid)
		assertKind(t, err, ErrorInvalidPath)
	}
	if requests.Load() != 0 {
		t.Fatalf("invalid paths sent %d requests", requests.Load())
	}
}

func TestFileMutationsPreserveUpstreamConflicts(t *testing.T) {
	for _, tc := range []struct {
		name string
		call func(*Client) error
	}{
		{"copy destination exists", func(client *Client) error {
			_, err := client.CopyFile(context.Background(), "projects/a.gcode", "archive/a.gcode")
			return err
		}},
		{"move source in use", func(client *Client) error {
			_, err := client.MoveFile(context.Background(), "projects/a.gcode", "archive/a.gcode")
			return err
		}},
		{"folder cannot be created", func(client *Client) error {
			_, err := client.CreateFolder(context.Background(), "a.gcode", "archive")
			return err
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusConflict)
				fmt.Fprint(w, "upstream-secret-detail")
			}))
			defer server.Close()

			err := tc.call(newTestClient(t, server.URL, 1024))

			assertKind(t, err, ErrorHTTPStatus)
			var typed *Error
			if !errors.As(err, &typed) || typed.StatusCode != http.StatusConflict {
				t.Fatalf("missing conflict status: %v", err)
			}
			if strings.Contains(err.Error(), "upstream-secret-detail") {
				t.Fatal("upstream detail leaked")
			}
		})
	}
}

func TestFileMutationsReturnSafeErrorsWithoutUpstreamBody(t *testing.T) {
	for _, tc := range []struct {
		name string
		call func(*Client) error
	}{
		{"copy", func(client *Client) error {
			_, err := client.CopyFile(context.Background(), "projects/a.gcode", "archive/a.gcode")
			return err
		}},
		{"move", func(client *Client) error {
			_, err := client.MoveFile(context.Background(), "projects/a.gcode", "archive/a.gcode")
			return err
		}},
		{"folder", func(client *Client) error {
			_, err := client.CreateFolder(context.Background(), "parts", "projects")
			return err
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusInternalServerError)
			}))
			defer server.Close()

			err := tc.call(newTestClient(t, server.URL, 1024))

			assertKind(t, err, ErrorHTTPStatus)
			var typed *Error
			if !errors.As(err, &typed) || typed.StatusCode != http.StatusInternalServerError {
				t.Fatalf("missing upstream status: %v", err)
			}
			if err.Error() != "octoprint: http_status" {
				t.Fatalf("unsafe error: %v", err)
			}
		})
	}
}

func TestFileMutationResponsesRejectInvalidPaths(t *testing.T) {
	for _, tc := range []struct {
		name string
		body string
		call func(*Client) error
	}{
		{"copy missing path", `{}`, func(client *Client) error {
			_, err := client.CopyFile(context.Background(), "projects/a.gcode", "archive/a.gcode")
			return err
		}},
		{"move missing path", `{"name":"a.gcode"}`, func(client *Client) error {
			_, err := client.MoveFile(context.Background(), "projects/a.gcode", "archive/a.gcode")
			return err
		}},
		{"copy URL path", `{"path":"https://evil.example/a.gcode"}`, func(client *Client) error {
			_, err := client.CopyFile(context.Background(), "projects/a.gcode", "archive/a.gcode")
			return err
		}},
		{"folder missing path", `{"done":true}`, func(client *Client) error {
			_, err := client.CreateFolder(context.Background(), "parts", "projects")
			return err
		}},
		{"folder URL path", `{"folder":{"path":"https://evil.example/parts"}}`, func(client *Client) error {
			_, err := client.CreateFolder(context.Background(), "parts", "projects")
			return err
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusCreated)
				fmt.Fprint(w, tc.body)
			}))
			defer server.Close()

			err := tc.call(newTestClient(t, server.URL, 1024))

			assertKind(t, err, ErrorInvalidJSON)
		})
	}
}

func TestPaddedAPIKeyIsRejectedBeforeRequest(t *testing.T) {
	for _, key := range []string{" test-secret ", " test-secret", "test-secret ", "\ttest-secret\t"} {
		t.Run(fmt.Sprintf("%q", key), func(t *testing.T) {
			var requests atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests.Add(1)
				fmt.Fprint(w, `{}`)
			}))
			defer server.Close()
			client, err := NewClient(server.URL, key, &http.Client{Timeout: time.Second}, 128, false)
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
