package mcpserver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/joaaomanooel/tupi-mcp/internal/config"
	"github.com/joaaomanooel/tupi-mcp/internal/octoprint"
)

const (
	testToken  = "test-mcp-token-with-at-least-thirty-two-bytes"
	testAPIKey = "test-api-key"
)

const (
	connectionBody = `{"current":{"state":"Operational","port":"/dev/ttyUSB0","baudrate":115200,"printerProfile":"ender3-v3-se"}}`
	printerBody    = `{"state":{"text":"Operational","flags":{"operational":true,"printing":false,"paused":false,"ready":true,"error":false,"closedOrError":false}},"temperature":{"bed":{"actual":21.5,"target":0},"tool0":{"actual":22.25,"target":200}}}`
	jobBody        = `{"job":{"file":{"name":"cube.gcode","display":"cube","path":"cube.gcode","origin":"local","size":1024,"date":1700000000},"estimatedPrintTime":120.5},"progress":{"completion":50,"filepos":512,"printTime":60,"printTimeLeft":60.5,"printTimeLeftOrigin":"analysis"},"state":"Printing"}`
	filesBody      = `{"files":[{"name":"cube.gcode","display":"cube","path":"cube.gcode","type":"machine","origin":"local","size":1024,"date":1700000000},{"name":"folder","type":"folder","children":[{"name":"inner.gcode","path":"folder/inner.gcode","type":"machine"}]}]}`
)

type printerMock struct {
	server   *httptest.Server
	mu       sync.Mutex
	requests []string
	routes   map[string]http.HandlerFunc
}

func newPrinterMock(t *testing.T) *printerMock {
	t.Helper()
	mock := &printerMock{routes: make(map[string]http.HandlerFunc)}
	mock.setRoute("/api/connection", jsonRoute(connectionBody))
	mock.setRoute("/api/printer", jsonRoute(printerBody))
	mock.setRoute("/api/job", jsonRoute(jobBody))
	mock.setRoute("/api/files", jsonRoute(filesBody))
	mock.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Api-Key") != testAPIKey {
			t.Errorf("unexpected OctoPrint credential header")
		}
		if r.Header.Get("Authorization") != "" {
			t.Error("MCP authorization header reached OctoPrint")
		}
		mock.mu.Lock()
		mock.requests = append(mock.requests, r.Method+" "+r.URL.Path)
		route := mock.routes[r.URL.Path]
		mock.mu.Unlock()
		if route == nil {
			http.NotFound(w, r)
			return
		}
		route(w, r)
	}))
	t.Cleanup(mock.server.Close)
	return mock
}

func jsonRoute(body string) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, body)
	}
}

func (m *printerMock) setRoute(path string, handler http.HandlerFunc) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.routes[path] = handler
}

func (m *printerMock) route(path string) http.HandlerFunc {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.routes[path]
}

func (m *printerMock) recordedRequests() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]string(nil), m.requests...)
}

func (m *printerMock) resetRequests() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.requests = nil
}

type readerStub struct {
	err error
}

type uploadRecorder struct {
	readerStub
	called bool
}

func (r *uploadRecorder) UploadGCode(context.Context, string, []byte) (*octoprint.UploadResponse, error) {
	r.called = true
	return &octoprint.UploadResponse{}, nil
}

func (r readerStub) Connection(context.Context) (*octoprint.ConnectionResponse, error) {
	return &octoprint.ConnectionResponse{}, r.err
}

func (r readerStub) Printer(context.Context) (*octoprint.PrinterResponse, error) {
	return &octoprint.PrinterResponse{}, r.err
}

func (r readerStub) Job(context.Context) (*octoprint.JobResponse, error) {
	return &octoprint.JobResponse{}, r.err
}

func (r readerStub) Files(context.Context) (*octoprint.FilesResponse, error) {
	return &octoprint.FilesResponse{}, r.err
}

func (r readerStub) UploadGCode(context.Context, string, []byte) (*octoprint.UploadResponse, error) {
	return &octoprint.UploadResponse{}, r.err
}

type testServer struct {
	url   string
	host  string
	token string
}

func startMCPServer(t *testing.T, reader Reader, origins []string, maxRequestBodyBytes int64) testServer {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	host := listener.Addr().String()
	handler, err := NewHandler(Options{
		Server:              NewServer(reader),
		Token:               testToken,
		AllowedHosts:        []string{host},
		AllowedOrigins:      origins,
		MaxRequestBodyBytes: maxRequestBodyBytes,
	})
	if err != nil {
		t.Fatalf("new handler: %v", err)
	}
	httpServer := &httptest.Server{Listener: listener, Config: &http.Server{Handler: handler}}
	httpServer.Start()
	t.Cleanup(httpServer.Close)
	return testServer{url: httpServer.URL, host: host, token: testToken}
}

func connectSession(t *testing.T, setup testServer) *mcp.ClientSession {
	t.Helper()
	transport := &mcp.StreamableClientTransport{
		Endpoint:             setup.url + "/mcp",
		HTTPClient:           &http.Client{Transport: &bearerTransport{token: setup.token}},
		DisableStandaloneSSE: true,
	}
	client := mcp.NewClient(&mcp.Implementation{Name: "tupi-mcp-test", Version: "0.0.0"}, nil)
	session, err := client.Connect(context.Background(), transport, nil)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() {
		_ = session.Close()
	})
	return session
}

type bearerTransport struct {
	token string
}

func (t *bearerTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	clone := request.Clone(request.Context())
	clone.Header.Set("Authorization", bearerPrefix+t.token)
	return http.DefaultTransport.RoundTrip(clone)
}

func newIntegrationServer(t *testing.T) (*printerMock, testServer, *mcp.ClientSession) {
	t.Helper()
	mock := newPrinterMock(t)
	client, err := octoprint.NewClient(mock.server.URL, testAPIKey, &http.Client{Timeout: 2 * time.Second}, 4096, false)
	if err != nil {
		t.Fatalf("octoprint client: %v", err)
	}
	setup := startMCPServer(t, client, nil, config.DefaultMaxRequestBodyBytes)
	return mock, setup, connectSession(t, setup)
}

func decodeOutput[T any](t *testing.T, result *mcp.CallToolResult) T {
	t.Helper()
	if result == nil || result.IsError {
		t.Fatalf("unexpected tool error: %s", contentText(result))
	}
	encoded, err := json.Marshal(result.StructuredContent)
	if err != nil {
		t.Fatalf("marshal structured content: %v", err)
	}
	var output T
	if err := json.Unmarshal(encoded, &output); err != nil {
		t.Fatalf("unmarshal structured content: %v", err)
	}
	return output
}

func contentText(result *mcp.CallToolResult) string {
	if result == nil {
		return "no result"
	}
	text := ""
	for _, content := range result.Content {
		if value, ok := content.(*mcp.TextContent); ok {
			text += value.Text
		}
	}
	return text
}

func callTool(t *testing.T, session *mcp.ClientSession, name string, arguments map[string]any) *mcp.CallToolResult {
	t.Helper()
	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: name, Arguments: arguments})
	if err != nil {
		t.Fatalf("call %s: %v", name, err)
	}
	return result
}

func TestToolDiscovery(t *testing.T) {
	setup := startMCPServer(t, readerStub{}, nil, 64<<10)
	session := connectSession(t, setup)

	result, err := session.ListTools(context.Background(), nil)

	if err != nil {
		t.Fatalf("list tools: %v", err)
	}
	expected := []string{"printer_get_status", "printer_get_temperatures", "printer_get_job", "printer_list_files", "printer_upload_gcode"}
	if len(result.Tools) != len(expected) {
		t.Fatalf("tools = %d, want %d", len(result.Tools), len(expected))
	}
	returned := make(map[string]*mcp.Tool, len(result.Tools))
	for _, tool := range result.Tools {
		returned[tool.Name] = tool
	}
	for _, name := range expected {
		tool, ok := returned[name]
		if !ok {
			t.Fatalf("missing tool %s", name)
		}
		if name == "printer_upload_gcode" {
			assertUploadInputSchema(t, tool)
		} else {
			assertNoInputArguments(t, tool)
		}
		assertOutputSchema(t, tool, name)
	}
}

func assertUploadInputSchema(t *testing.T, tool *mcp.Tool) {
	t.Helper()
	var schema struct {
		Properties           map[string]json.RawMessage `json:"properties"`
		AdditionalProperties *bool                      `json:"additionalProperties"`
		Required             []string                   `json:"required"`
	}
	encoded, err := json.Marshal(tool.InputSchema)
	if err != nil {
		t.Fatalf("marshal upload input schema: %v", err)
	}
	if err := json.Unmarshal(encoded, &schema); err != nil {
		t.Fatalf("unmarshal upload input schema: %v", err)
	}
	if len(schema.Properties) != 2 || schema.Properties["filename"] == nil || schema.Properties["contents"] == nil {
		t.Errorf("unexpected upload input schema: %s", encoded)
	}
	if schema.AdditionalProperties == nil || *schema.AdditionalProperties {
		t.Errorf("upload input schema allows undeclared arguments: %s", encoded)
	}
	sort.Strings(schema.Required)
	if len(schema.Required) != 2 || schema.Required[0] != "contents" || schema.Required[1] != "filename" {
		t.Errorf("upload input required = %v, want [filename contents]", schema.Required)
	}
}

func TestUploadHandlerRejectsOversizeBeforeCallingReader(t *testing.T) {
	reader := &uploadRecorder{}
	handler := uploadHandler(reader)
	contents := strings.Repeat("G28\n", octoprint.MaxGCodeUploadBytes/4) + "G"
	_, _, err := handler(context.Background(), nil, uploadInput{Filename: "large.gcode", Contents: contents})
	if err == nil || !strings.Contains(err.Error(), string(octoprint.ErrorRequestTooLarge)) {
		t.Fatalf("unexpected oversized upload error: %v", err)
	}
	if reader.called {
		t.Error("reader received an oversized upload")
	}
}

func assertNoInputArguments(t *testing.T, tool *mcp.Tool) {
	t.Helper()
	var schema struct {
		Type                 string                     `json:"type"`
		Properties           map[string]json.RawMessage `json:"properties"`
		AdditionalProperties *bool                      `json:"additionalProperties"`
	}
	encoded, err := json.Marshal(tool.InputSchema)
	if err != nil {
		t.Fatalf("marshal input schema: %v", err)
	}
	if err := json.Unmarshal(encoded, &schema); err != nil {
		t.Fatalf("unmarshal input schema: %v", err)
	}
	if schema.Type != "object" || len(schema.Properties) != 0 {
		t.Errorf("%s input schema accepts arguments: %s", tool.Name, encoded)
	}
	if schema.AdditionalProperties == nil || *schema.AdditionalProperties {
		t.Errorf("%s input schema allows undeclared arguments: %s", tool.Name, encoded)
	}
}

func assertOutputSchema(t *testing.T, tool *mcp.Tool, name string) {
	t.Helper()
	var schema struct {
		Type       string                     `json:"type"`
		Properties map[string]json.RawMessage `json:"properties"`
		Required   []string                   `json:"required"`
	}
	encoded, err := json.Marshal(tool.OutputSchema)
	if err != nil {
		t.Fatalf("marshal output schema: %v", err)
	}
	if err := json.Unmarshal(encoded, &schema); err != nil {
		t.Fatalf("unmarshal output schema: %v", err)
	}
	if schema.Type != "object" || len(schema.Properties) == 0 {
		t.Fatalf("%s output schema is not an object with properties: %s", name, encoded)
	}
	if name == "printer_list_files" {
		if len(schema.Required) != 1 || schema.Required[0] != "files" {
			t.Errorf("files output schema required = %v, want [files]", schema.Required)
		}
		return
	}
	if len(schema.Required) != 0 {
		t.Errorf("%s output schema requires optional OctoPrint fields: %v", name, schema.Required)
	}
}

func TestToolInvocation(t *testing.T) {
	mock, _, session := newIntegrationServer(t)

	status := decodeOutput[statusOutput](t, callTool(t, session, "printer_get_status", nil))
	temperatures := decodeOutput[temperaturesOutput](t, callTool(t, session, "printer_get_temperatures", nil))
	job := decodeOutput[jobOutput](t, callTool(t, session, "printer_get_job", nil))
	files := decodeOutput[filesOutput](t, callTool(t, session, "printer_list_files", nil))

	if status.Connection == nil || status.Connection.State == nil || *status.Connection.State != "Operational" {
		t.Errorf("unexpected connection state: %+v", status.Connection)
	}
	if status.Printer == nil || status.Printer.State == nil || status.Printer.State.Flags == nil || !*status.Printer.State.Flags.Ready {
		t.Errorf("unexpected printer state: %+v", status.Printer)
	}
	bed, ok := temperatures.Temperatures["bed"]
	if !ok || bed.Actual == nil || *bed.Actual != 21.5 || bed.Target == nil || *bed.Target != 0 {
		t.Errorf("unexpected bed temperatures: %+v", temperatures)
	}
	tool0, ok := temperatures.Temperatures["tool0"]
	if !ok || tool0.Target == nil || *tool0.Target != 200 {
		t.Errorf("unexpected tool temperatures: %+v", temperatures)
	}
	if job.Progress == nil || job.Progress.FilePosition == nil || *job.Progress.FilePosition != 512 {
		t.Errorf("unexpected job progress: %+v", job.Progress)
	}
	if job.Job == nil || job.Job.File == nil || job.Job.File.Name == nil || *job.Job.File.Name != "cube.gcode" {
		t.Errorf("unexpected job file: %+v", job.Job)
	}
	if job.State == nil || *job.State != "Printing" {
		t.Errorf("unexpected job state: %+v", job.State)
	}
	if len(files.Files) != 2 {
		t.Fatalf("files = %d, want 2", len(files.Files))
	}
	if files.Files[0].Size == nil || *files.Files[0].Size != 1024 {
		t.Errorf("unexpected file size: %+v", files.Files[0])
	}
	if len(files.Files[1].Children) != 1 || files.Files[1].Children[0].Path == nil || *files.Files[1].Children[0].Path != "folder/inner.gcode" {
		t.Errorf("unexpected nested files: %+v", files.Files[1])
	}

	mock.setRoute("/api/files/local", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("upload method = %s, want POST", r.Method)
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
		content, err := io.ReadAll(file)
		if err != nil || header.Filename != "safe.gcode" || string(content) != "G28\nM84\n" {
			t.Errorf("unexpected uploaded file %q: %q (%v)", header.Filename, content, err)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		fmt.Fprint(w, `{"files":{"local":{"name":"safe.gcode","path":"safe.gcode","origin":"local"}},"done":true,"effectiveSelect":false,"effectivePrint":false}`)
	})
	uploadResult := decodeOutput[uploadOutput](t, callTool(t, session, "printer_upload_gcode", map[string]any{
		"filename": "safe.gcode",
		"contents": "G28\nM84\n",
	}))
	if uploadResult.File == nil || uploadResult.File.Name == nil || *uploadResult.File.Name != "safe.gcode" || uploadResult.EffectivePrint == nil || *uploadResult.EffectivePrint {
		t.Errorf("unexpected upload result: %+v", uploadResult)
	}

	expected := []string{"GET /api/connection", "GET /api/printer", "GET /api/printer", "GET /api/job", "GET /api/files", "POST /api/files/local"}
	requests := mock.recordedRequests()
	if len(requests) != len(expected) {
		t.Fatalf("requests = %v, want %v", requests, expected)
	}
	for index, request := range requests {
		if request != expected[index] {
			t.Fatalf("requests = %v, want %v", requests, expected)
		}
	}
}

func TestUploadGCodeAtMaximumSize(t *testing.T) {
	mock, _, session := newIntegrationServer(t)
	mock.setRoute("/api/files/local", func(w http.ResponseWriter, r *http.Request) {
		file, header, err := r.FormFile("file")
		if err != nil {
			t.Errorf("upload file: %v", err)
			return
		}
		defer file.Close()
		written, err := io.Copy(io.Discard, file)
		if err != nil || written != octoprint.MaxGCodeUploadBytes || header.Size != octoprint.MaxGCodeUploadBytes {
			t.Errorf("uploaded size = %d, header size = %d, error = %v", written, header.Size, err)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		fmt.Fprint(w, `{"files":{"local":{"name":"large.gcode","path":"large.gcode","origin":"local"}},"done":true,"effectiveSelect":false,"effectivePrint":false}`)
	})
	contents := strings.Repeat("G28\n", octoprint.MaxGCodeUploadBytes/4) + strings.Repeat("G", octoprint.MaxGCodeUploadBytes%4)
	result := callTool(t, session, "printer_upload_gcode", map[string]any{"filename": "large.gcode", "contents": contents})
	if result == nil || result.IsError {
		t.Fatalf("maximum-size upload failed: %s", contentText(result))
	}
	tooLargeSize := octoprint.MaxGCodeUploadBytes + 1
	tooLarge := strings.Repeat("G28\n", tooLargeSize/4) + strings.Repeat("G", tooLargeSize%4)
	rejected := callTool(t, session, "printer_upload_gcode", map[string]any{"filename": "large.gcode", "contents": tooLarge})
	if rejected == nil || !rejected.IsError || !strings.Contains(contentText(rejected), string(octoprint.ErrorRequestTooLarge)) {
		t.Fatalf("oversized upload was not rejected safely: %s", contentText(rejected))
	}
	if requests := mock.recordedRequests(); len(requests) != 1 {
		t.Fatalf("oversized upload reached OctoPrint: %v", requests)
	}
}

func TestStatusReportsConnectionWhenPrinterIsNotOperational(t *testing.T) {
	mock, _, session := newIntegrationServer(t)
	mock.setRoute("/api/connection", jsonRoute(`{"current":{"state":"Closed","port":"/dev/ttyUSB0","baudrate":115200,"printerProfile":"ender3-v3-se"}}`))
	mock.setRoute("/api/printer", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusConflict)
	})

	result := callTool(t, session, "printer_get_status", nil)
	status := decodeOutput[statusOutput](t, result)

	if status.Connection == nil || status.Connection.State == nil || *status.Connection.State != "Closed" {
		t.Errorf("unexpected connection state: %+v", status.Connection)
	}
	if status.Printer != nil {
		t.Errorf("printer details reported after OctoPrint returned 409: %+v", status.Printer)
	}
	encoded, err := json.Marshal(result.StructuredContent)
	if err != nil {
		t.Fatalf("marshal structured content: %v", err)
	}
	if contains(string(encoded), `"printer"`) {
		t.Errorf("printer data reported after OctoPrint returned 409: %s", encoded)
	}
	expected := []string{"GET /api/connection", "GET /api/printer"}
	requests := mock.recordedRequests()
	if len(requests) != len(expected) {
		t.Fatalf("requests = %v, want %v", requests, expected)
	}
	for index, request := range requests {
		if request != expected[index] {
			t.Fatalf("requests = %v, want %v", requests, expected)
		}
	}
}

func TestToolRejectsArguments(t *testing.T) {
	mock, _, session := newIntegrationServer(t)

	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "printer_get_status",
		Arguments: map[string]any{"action": "pause"},
	})

	if err == nil && (result == nil || !result.IsError) {
		t.Fatalf("expected argument rejection, got %+v", result)
	}
	if requests := mock.recordedRequests(); len(requests) != 0 {
		t.Errorf("rejected call reached OctoPrint: %v", requests)
	}
}

func TestToolErrorsAreSafeAndServerKeepsServing(t *testing.T) {
	for _, tc := range []struct {
		name    string
		tool    string
		path    string
		handler http.HandlerFunc
		want    string
	}{
		{
			name: "unavailable",
			tool: "printer_get_status",
			path: "/api/printer",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusServiceUnavailable)
				fmt.Fprint(w, `upstream-secret-detail `+testAPIKey)
			},
			want: "http_status",
		},
		{
			name: "malformed",
			tool: "printer_get_job",
			path: "/api/job",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				fmt.Fprint(w, `{"job":`)
			},
			want: "invalid_json",
		},
		{
			name: "sensitive",
			tool: "printer_list_files",
			path: "/api/files",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				fmt.Fprint(w, `{"files":[],"note":"`+testAPIKey+`"}`)
			},
			want: "sensitive_response",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			mock, _, session := newIntegrationServer(t)
			healthy := mock.route(tc.path)

			mock.setRoute(tc.path, tc.handler)
			result := callTool(t, session, tc.tool, nil)

			if result == nil || !result.IsError {
				t.Fatalf("expected tool error, got %+v", result)
			}
			message := contentText(result)
			for _, forbidden := range []string{testAPIKey, testToken, "upstream-secret-detail", "/api/", "/"} {
				if contains(message, forbidden) {
					t.Errorf("tool error message %q discloses %q", message, forbidden)
				}
			}
			if !contains(message, tc.want) {
				t.Errorf("tool error message %q does not mention %q", message, tc.want)
			}

			failed := callTool(t, session, tc.tool, nil)
			if failed == nil || !failed.IsError {
				t.Fatalf("expected repeated tool error, got %+v", failed)
			}
			listed, err := session.ListTools(context.Background(), nil)
			if err != nil || len(listed.Tools) != 5 {
				t.Fatalf("server stopped serving after tool error: %v", err)
			}

			mock.setRoute(tc.path, healthy)
			if recovered := callTool(t, session, tc.tool, nil); recovered == nil || recovered.IsError {
				t.Fatalf("tool did not recover: %s", contentText(recovered))
			}
		})
	}
}

func TestToolErrorMessagesDoNotDiscloseInternalDetails(t *testing.T) {
	for _, tc := range []struct {
		name   string
		err    error
		want   string
		forbid []string
	}{
		{
			name:   "transport",
			err:    &octoprint.Error{Kind: octoprint.ErrorTransport},
			want:   "transport",
			forbid: []string{"127.0.0.1", "http://"},
		},
		{
			name:   "status",
			err:    &octoprint.Error{Kind: octoprint.ErrorHTTPStatus, StatusCode: 409},
			want:   "status 409",
			forbid: []string{"/api/", "response"},
		},
		{
			name:   "unknown",
			err:    errors.New("dial tcp 127.0.0.1:5000: connect: connection refused"),
			want:   "octoprint request failed",
			forbid: []string{"127.0.0.1", "tcp", "refused"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			message := toolError(tc.err).Error()

			if !contains(message, tc.want) {
				t.Errorf("message %q does not mention %q", message, tc.want)
			}
			for _, forbidden := range tc.forbid {
				if contains(message, forbidden) {
					t.Errorf("message %q discloses %q", message, forbidden)
				}
			}
		})
	}
}

func TestOptionalFieldsStayOptional(t *testing.T) {
	temperatures := newTemperaturesOutput(&octoprint.PrinterResponse{})
	files := newFilesOutput(&octoprint.FilesResponse{})
	job := newJobOutput(&octoprint.JobResponse{})

	encodedTemperatures, err := json.Marshal(temperatures)
	if err != nil || string(encodedTemperatures) != `{}` {
		t.Errorf("empty temperatures = %s, %v", encodedTemperatures, err)
	}
	encodedJob, err := json.Marshal(job)
	if err != nil || string(encodedJob) != `{}` {
		t.Errorf("empty job = %s, %v", encodedJob, err)
	}
	encodedFiles, err := json.Marshal(files)
	if err != nil || string(encodedFiles) != `{"files":[]}` {
		t.Errorf("empty files = %s, %v", encodedFiles, err)
	}
}

func contains(value, substring string) bool {
	return strings.Contains(value, substring)
}
