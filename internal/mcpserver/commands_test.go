package mcpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"reflect"
	"slices"
	"sort"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func listTools(t *testing.T, session *mcp.ClientSession) map[string]*mcp.Tool {
	t.Helper()
	result, err := session.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatalf("list tools: %v", err)
	}
	tools := make(map[string]*mcp.Tool, len(result.Tools))
	for _, tool := range result.Tools {
		tools[tool.Name] = tool
	}
	return tools
}

func assertInputSchema(t *testing.T, tool *mcp.Tool, properties, required []string) {
	t.Helper()
	var schema struct {
		Type                 string                     `json:"type"`
		Properties           map[string]json.RawMessage `json:"properties"`
		AdditionalProperties *bool                      `json:"additionalProperties"`
		Required             []string                   `json:"required"`
	}
	encoded, err := json.Marshal(tool.InputSchema)
	if err != nil {
		t.Fatalf("marshal %s input schema: %v", tool.Name, err)
	}
	if err := json.Unmarshal(encoded, &schema); err != nil {
		t.Fatalf("unmarshal %s input schema: %v", tool.Name, err)
	}
	if schema.Type != "object" || len(schema.Properties) != len(properties) {
		t.Errorf("%s input schema = %s, want properties %v", tool.Name, encoded, properties)
	}
	for _, name := range properties {
		if schema.Properties[name] == nil {
			t.Errorf("%s input schema is missing %s: %s", tool.Name, name, encoded)
		}
	}
	if schema.AdditionalProperties == nil || *schema.AdditionalProperties {
		t.Errorf("%s input schema allows undeclared arguments: %s", tool.Name, encoded)
	}
	sort.Strings(schema.Required)
	wantRequired := slices.Clone(required)
	sort.Strings(wantRequired)
	if !slices.Equal(schema.Required, wantRequired) {
		t.Errorf("%s input required = %v, want %v", tool.Name, schema.Required, wantRequired)
	}
}

func TestConfigurableToolsAreDeniedByDefault(t *testing.T) {
	mock, _, session := newIntegrationServer(t)

	tools := listTools(t, session)
	for name := range configurableTools {
		if tools[name] != nil {
			t.Errorf("tool %s is published without configuration", name)
		}
	}

	_, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      ToolDeleteFile,
		Arguments: map[string]any{"path": "cube.gcode"},
	})
	if err == nil {
		t.Fatal("denied tool call succeeded")
	}
	if requests := mock.recordedRequests(); len(requests) != 0 {
		t.Errorf("denied tool call reached OctoPrint: %v", requests)
	}
}

func TestOnlyConfiguredToolsArePublished(t *testing.T) {
	_, _, session := newIntegrationServerWithTools(t, ToolDeleteFile)

	tools := listTools(t, session)
	if tools[ToolDeleteFile] == nil {
		t.Fatal("configured tool was not published")
	}
	for _, name := range append(slices.Clone(fileTools), jobTools...) {
		if name == ToolDeleteFile {
			continue
		}
		if tools[name] != nil {
			t.Errorf("tool %s is published without configuration", name)
		}
	}
	assertInputSchema(t, tools[ToolDeleteFile], []string{"path"}, []string{"path"})
	assertOutputSchema(t, tools[ToolDeleteFile], ToolDeleteFile)
}

func TestEnabledFileToolsArePublishedAndInvoked(t *testing.T) {
	mock, _, session := newIntegrationServerWithTools(t, fileTools...)

	tools := listTools(t, session)
	for _, name := range fileTools {
		if tools[name] == nil {
			t.Fatalf("configured tool %s was not published", name)
		}
		assertOutputSchema(t, tools[name], name)
	}
	assertInputSchema(t, tools[ToolCreateFolder], []string{"name", "parent"}, []string{"name"})
	assertInputSchema(t, tools[ToolCopyFile], []string{"source", "destination"}, []string{"source", "destination"})
	assertInputSchema(t, tools[ToolMoveFile], []string{"source", "destination"}, []string{"source", "destination"})
	assertInputSchema(t, tools[ToolSelectFile], []string{"path"}, []string{"path"})
	assertInputSchema(t, tools[ToolDeleteFile], []string{"path"}, []string{"path"})
	assertInputSchema(t, tools[ToolSliceFile], []string{"path", "destination"}, []string{"path", "destination"})

	mock.setRoute("/api/files/local", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("create folder method = %s, want POST", r.Method)
		}
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			t.Errorf("parse folder form: %v", err)
			return
		}
		if r.FormValue("foldername") != "prints" || r.FormValue("path") != "archive" {
			t.Errorf("folder form name=%q path=%q", r.FormValue("foldername"), r.FormValue("path"))
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		fmt.Fprint(w, `{"folder":{"name":"prints","path":"archive/prints","origin":"local"},"done":true}`)
	})
	mock.setRoute("/api/files/local/cube.gcode", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		if r.Method != http.MethodPost {
			t.Errorf("file command method = %s, want POST or DELETE", r.Method)
			return
		}
		var payload struct {
			Command     string `json:"command"`
			Destination string `json:"destination"`
			Select      *bool  `json:"select"`
			Print       *bool  `json:"print"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Errorf("decode file command: %v", err)
			return
		}
		switch payload.Command {
		case "copy":
			if payload.Destination != "copies/cube.gcode" {
				t.Errorf("copy destination = %q", payload.Destination)
			}
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{"name":"cube.gcode","path":"copies/cube.gcode","origin":"local"}`)
		case "move":
			if payload.Destination != "archive/cube.gcode" {
				t.Errorf("move destination = %q", payload.Destination)
			}
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{"path":"archive/cube.gcode"}`)
		case "select":
			if payload.Print == nil || *payload.Print {
				t.Errorf("select print flag = %v, want false", payload.Print)
			}
			w.WriteHeader(http.StatusNoContent)
		case "slice":
			if payload.Destination != "sliced/cube.gcode" || payload.Select == nil || *payload.Select || payload.Print == nil || *payload.Print {
				t.Errorf("slice payload = %+v", payload)
			}
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{"path":"sliced/cube.gcode"}`)
		default:
			t.Errorf("unexpected file command %q", payload.Command)
		}
	})

	folder := decodeOutput[folderOutput](t, callTool(t, session, ToolCreateFolder, map[string]any{"name": "prints", "parent": "archive"}))
	if folder.Folder == nil || folder.Folder.Path == nil || *folder.Folder.Path != "archive/prints" || folder.Done == nil || !*folder.Done {
		t.Errorf("unexpected folder output: %+v", folder)
	}
	copied := decodeOutput[fileCommandOutput](t, callTool(t, session, ToolCopyFile, map[string]any{"source": "cube.gcode", "destination": "copies/cube.gcode"}))
	if copied.Path == nil || *copied.Path != "copies/cube.gcode" || copied.Name == nil || *copied.Name != "cube.gcode" {
		t.Errorf("unexpected copy output: %+v", copied)
	}
	moved := decodeOutput[fileCommandOutput](t, callTool(t, session, ToolMoveFile, map[string]any{"source": "cube.gcode", "destination": "archive/cube.gcode"}))
	if moved.Path == nil || *moved.Path != "archive/cube.gcode" {
		t.Errorf("unexpected move output: %+v", moved)
	}
	selected := decodeOutput[commandOutput](t, callTool(t, session, ToolSelectFile, map[string]any{"path": "cube.gcode"}))
	if selected.Done == nil || !*selected.Done {
		t.Errorf("unexpected select output: %+v", selected)
	}
	deleted := decodeOutput[commandOutput](t, callTool(t, session, ToolDeleteFile, map[string]any{"path": "cube.gcode"}))
	if deleted.Done == nil || !*deleted.Done {
		t.Errorf("unexpected delete output: %+v", deleted)
	}
	sliced := decodeOutput[fileCommandOutput](t, callTool(t, session, ToolSliceFile, map[string]any{"path": "cube.gcode", "destination": "sliced/cube.gcode"}))
	if sliced.Path == nil || *sliced.Path != "sliced/cube.gcode" {
		t.Errorf("unexpected slice output: %+v", sliced)
	}

	expected := []string{
		"POST /api/files/local",
		"POST /api/files/local/cube.gcode",
		"POST /api/files/local/cube.gcode",
		"POST /api/files/local/cube.gcode",
		"DELETE /api/files/local/cube.gcode",
		"POST /api/files/local/cube.gcode",
	}
	requests := mock.recordedRequests()
	if !slices.Equal(requests, expected) {
		t.Fatalf("requests = %v, want %v", requests, expected)
	}
}

func TestEnabledJobToolsArePublishedAndInvoked(t *testing.T) {
	mock, _, session := newIntegrationServerWithTools(t, jobTools...)

	tools := listTools(t, session)
	for _, name := range jobTools {
		if tools[name] == nil {
			t.Fatalf("configured tool %s was not published", name)
		}
		assertNoInputArguments(t, tools[name])
		assertOutputSchema(t, tools[name], name)
	}

	var payloads []map[string]any
	mock.setRoute("/api/job", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("job command method = %s, want POST", r.Method)
		}
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Errorf("decode job command: %v", err)
			return
		}
		payloads = append(payloads, payload)
		w.WriteHeader(http.StatusNoContent)
	})

	for _, name := range jobTools {
		output := decodeOutput[commandOutput](t, callTool(t, session, name, nil))
		if output.Done == nil || !*output.Done {
			t.Errorf("%s output = %+v, want done", name, output)
		}
	}

	expected := []map[string]any{
		{"command": "start"},
		{"command": "pause", "action": "pause"},
		{"command": "pause", "action": "resume"},
		{"command": "restart"},
		{"command": "cancel"},
	}
	if len(payloads) != len(expected) {
		t.Fatalf("job payloads = %v, want %v", payloads, expected)
	}
	if !reflect.DeepEqual(payloads, expected) {
		t.Fatalf("job payloads = %v, want %v", payloads, expected)
	}
}

func TestConfiguredFileAndJobToolDescriptionsNameRequiredPermissions(t *testing.T) {
	enabled := make([]string, 0, len(configurableTools))
	for name := range configurableTools {
		enabled = append(enabled, name)
	}
	_, _, session := newIntegrationServerWithTools(t, enabled...)

	tools := listTools(t, session)
	requiredPermissions := map[string]string{
		ToolCreateFolder: "Requires the FILES_UPLOAD permission.",
		ToolCopyFile:     "Requires the FILES_UPLOAD permission.",
		ToolMoveFile:     "Requires the FILES_UPLOAD and FILES_DELETE permissions.",
		ToolSelectFile:   "Requires the FILES_SELECT permission.",
		ToolDeleteFile:   "Requires the FILES_DELETE permission.",
		ToolSliceFile:    "Requires the SLICE permission.",
		ToolStartJob:     "Requires the PRINT permission.",
		ToolPauseJob:     "Requires the PRINT permission.",
		ToolResumeJob:    "Requires the PRINT permission.",
		ToolRestartJob:   "Requires the PRINT permission.",
		ToolCancelJob:    "Requires the PRINT permission.",
	}
	if len(requiredPermissions) != len(enabled) {
		t.Fatalf("expected permission clauses cover %d tools, want %d", len(requiredPermissions), len(enabled))
	}
	for name, clause := range requiredPermissions {
		tool := tools[name]
		if tool == nil {
			t.Fatalf("configured tool %s was not published", name)
		}
		if !strings.HasSuffix(tool.Description, clause) {
			t.Errorf("%s description %q does not end with %q", name, tool.Description, clause)
		}
		if clauses := strings.Count(tool.Description, "Requires the"); clauses != 1 {
			t.Errorf("%s description %q has %d permission clauses, want 1", name, tool.Description, clauses)
		}
	}
}

func TestConfigurableToolErrorsAreFiltered(t *testing.T) {
	for _, tc := range []struct {
		name    string
		tool    string
		args    map[string]any
		path    string
		status  int
		message string
	}{
		{
			name:    "delete conflict",
			tool:    ToolDeleteFile,
			args:    map[string]any{"path": "cube.gcode"},
			path:    "/api/files/local/cube.gcode",
			status:  http.StatusConflict,
			message: "status 409",
		},
		{
			name:    "slice failure",
			tool:    ToolSliceFile,
			args:    map[string]any{"path": "cube.gcode", "destination": "sliced/cube.gcode"},
			path:    "/api/files/local/cube.gcode",
			status:  http.StatusInternalServerError,
			message: "status 500",
		},
		{
			name:    "start failure",
			tool:    ToolStartJob,
			path:    "/api/job",
			status:  http.StatusConflict,
			message: "status 409",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			mock, _, session := newIntegrationServerWithTools(t, tc.tool)
			mock.setRoute(tc.path, func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tc.status)
				fmt.Fprint(w, `{"error":"upstream-secret-detail","key":"`+testAPIKey+`"}`)
			})

			result := callTool(t, session, tc.tool, tc.args)
			if result == nil || !result.IsError {
				t.Fatalf("expected tool error, got %+v", result)
			}
			message := contentText(result)
			for _, forbidden := range []string{testAPIKey, "upstream-secret-detail", "cube.gcode", "/api/"} {
				if contains(message, forbidden) {
					t.Errorf("tool error message %q discloses %q", message, forbidden)
				}
			}
			if !contains(message, tc.message) {
				t.Errorf("tool error message %q does not mention %q", message, tc.message)
			}
		})
	}
}

func TestConfigurableToolRejectsInvalidPathWithoutRequest(t *testing.T) {
	mock, _, session := newIntegrationServerWithTools(t, ToolDeleteFile)

	result := callTool(t, session, ToolDeleteFile, map[string]any{"path": "../cube.gcode"})

	if result == nil || !result.IsError {
		t.Fatalf("expected tool error, got %+v", result)
	}
	message := contentText(result)
	if !contains(message, "invalid_path") || contains(message, "../cube.gcode") {
		t.Errorf("unexpected invalid path message: %q", message)
	}
	if requests := mock.recordedRequests(); len(requests) != 0 {
		t.Errorf("invalid path reached OctoPrint: %v", requests)
	}
}

func TestNewServerValidatesEnabledTools(t *testing.T) {
	if _, err := NewServer(readerStub{}, []string{"printer_unknown"}); err == nil {
		t.Fatal("unknown tool name was accepted")
	}
	if _, err := NewServer(readerStub{}, []string{ToolStartJob, ToolStartJob}); err == nil {
		t.Fatal("duplicate tool name was accepted")
	}
	if _, err := NewServer(readerStub{}, []string{ToolDeleteFile}); err == nil {
		t.Fatal("file tool was accepted without a file manager")
	}
	if _, err := NewServer(readerStub{}, []string{ToolStartJob}); err == nil {
		t.Fatal("job tool was accepted without a job controller")
	}
}
