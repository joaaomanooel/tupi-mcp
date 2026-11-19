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
	"sync"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/joaaomanooel/tupi-mcp/internal/config"
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
	enabled := append(slices.Clone(fileTools), jobTools...)
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
		if slices.Contains([]string{ToolStartJob, ToolResumeJob, ToolRestartJob}, name) && strings.Contains(tool.Description, "physical: operation_busy") {
			t.Errorf("%s description %q advertises unavailable physical coordination", name, tool.Description)
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

type physicalWrite struct {
	method string
	path   string
	body   map[string]any
}

func decodePayload(t *testing.T, value string) map[string]any {
	t.Helper()
	var payload map[string]any
	if err := json.Unmarshal([]byte(value), &payload); err != nil {
		t.Fatalf("decode payload %s: %v", value, err)
	}
	return payload
}

func recordPhysicalRoute(t *testing.T, path, readBody string, writes *[]physicalWrite) http.HandlerFunc {
	t.Helper()
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, readBody)
			return
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode %s payload: %v", path, err)
			return
		}
		*writes = append(*writes, physicalWrite{method: r.Method, path: path, body: body})
		w.WriteHeader(http.StatusNoContent)
	}
}

func TestEnabledPhysicalToolsArePublishedAndInvoked(t *testing.T) {
	mock, _, session := newPhysicalIntegrationServer(t, physicalTools...)

	tools := listTools(t, session)
	for _, name := range physicalTools {
		if tools[name] == nil {
			t.Fatalf("configured tool %s was not published", name)
		}
		assertOutputSchema(t, tools[name], name)
	}
	for _, name := range []string{ToolConnect, ToolReconnect, ToolDisconnect, ToolInitSD, ToolRefreshSD, ToolReleaseSD} {
		assertNoInputArguments(t, tools[name])
	}
	assertInputSchema(t, tools[ToolJog], []string{"x", "y", "z", "speed"}, []string{"speed"})
	assertInputSchema(t, tools[ToolHome], []string{"axes"}, []string{"axes"})
	assertInputSchema(t, tools[ToolSetFeedrate], []string{"factor"}, []string{"factor"})
	assertInputSchema(t, tools[ToolSetToolTemperature], []string{"tool", "target"}, []string{"tool", "target"})
	assertInputSchema(t, tools[ToolSetToolOffset], []string{"tool", "offset"}, []string{"tool", "offset"})
	assertInputSchema(t, tools[ToolSelectTool], []string{"tool"}, []string{"tool"})
	assertInputSchema(t, tools[ToolExtrude], []string{"amount", "speed"}, []string{"amount", "speed"})
	assertInputSchema(t, tools[ToolSetBedTemperature], []string{"target"}, []string{"target"})
	assertInputSchema(t, tools[ToolSetBedOffset], []string{"offset"}, []string{"offset"})
	assertInputSchema(t, tools[ToolSetChamberTemperature], []string{"target"}, []string{"target"})
	assertInputSchema(t, tools[ToolSetChamberOffset], []string{"offset"}, []string{"offset"})

	var writes []physicalWrite
	mock.setRoute("/api/connection", recordPhysicalRoute(t, "/api/connection", connectionBody, &writes))
	mock.setRoute("/api/printer/tool", recordPhysicalRoute(t, "/api/printer/tool", physicalToolsBody, &writes))
	mock.setRoute("/api/printer/printhead", recordPhysicalRoute(t, "/api/printer/printhead", "", &writes))
	mock.setRoute("/api/printer/bed", recordPhysicalRoute(t, "/api/printer/bed", "", &writes))
	mock.setRoute("/api/printer/chamber", recordPhysicalRoute(t, "/api/printer/chamber", "", &writes))
	mock.setRoute("/api/printer/sd", recordPhysicalRoute(t, "/api/printer/sd", "", &writes))

	calls := []struct {
		tool string
		args map[string]any
	}{
		{ToolReconnect, nil},
		{ToolJog, map[string]any{"x": 10, "speed": 1200}},
		{ToolHome, map[string]any{"axes": []string{"x", "y"}}},
		{ToolSetFeedrate, map[string]any{"factor": 150}},
		{ToolSetToolTemperature, map[string]any{"tool": "tool0", "target": 200}},
		{ToolSetToolOffset, map[string]any{"tool": "tool0", "offset": -1}},
		{ToolSelectTool, map[string]any{"tool": "tool0"}},
		{ToolExtrude, map[string]any{"amount": 5, "speed": 120}},
		{ToolSetBedTemperature, map[string]any{"target": 60}},
		{ToolSetBedOffset, map[string]any{"offset": -1}},
		{ToolSetChamberTemperature, map[string]any{"target": 40}},
		{ToolSetChamberOffset, map[string]any{"offset": 1}},
		{ToolInitSD, nil},
		{ToolRefreshSD, nil},
		{ToolReleaseSD, nil},
		{ToolDisconnect, nil},
	}
	for _, call := range calls {
		output := decodeOutput[physicalCommandOutput](t, callTool(t, session, call.tool, call.args))
		if output.Outcome != PhysicalAccepted {
			t.Errorf("%s outcome = %q, want %q", call.tool, output.Outcome, PhysicalAccepted)
		}
	}

	expected := []physicalWrite{
		{http.MethodPost, "/api/connection", decodePayload(t, `{"command":"connect"}`)},
		{http.MethodPost, "/api/printer/printhead", decodePayload(t, `{"command":"jog","x":10,"speed":1200}`)},
		{http.MethodPost, "/api/printer/printhead", decodePayload(t, `{"command":"home","axes":["x","y"]}`)},
		{http.MethodPost, "/api/printer/printhead", decodePayload(t, `{"command":"feedrate","factor":150}`)},
		{http.MethodPost, "/api/printer/tool", decodePayload(t, `{"command":"target","targets":{"tool0":200}}`)},
		{http.MethodPost, "/api/printer/tool", decodePayload(t, `{"command":"offset","offsets":{"tool0":-1}}`)},
		{http.MethodPost, "/api/printer/tool", decodePayload(t, `{"command":"select","tool":"tool0"}`)},
		{http.MethodPost, "/api/printer/tool", decodePayload(t, `{"command":"extrude","amount":5,"speed":120}`)},
		{http.MethodPost, "/api/printer/bed", decodePayload(t, `{"command":"target","target":60}`)},
		{http.MethodPost, "/api/printer/bed", decodePayload(t, `{"command":"offset","offset":-1}`)},
		{http.MethodPost, "/api/printer/chamber", decodePayload(t, `{"command":"target","target":40}`)},
		{http.MethodPost, "/api/printer/chamber", decodePayload(t, `{"command":"offset","offset":1}`)},
		{http.MethodPost, "/api/printer/sd", decodePayload(t, `{"command":"init"}`)},
		{http.MethodPost, "/api/printer/sd", decodePayload(t, `{"command":"refresh"}`)},
		{http.MethodPost, "/api/printer/sd", decodePayload(t, `{"command":"release"}`)},
		{http.MethodPost, "/api/connection", decodePayload(t, `{"command":"disconnect"}`)},
	}
	if !reflect.DeepEqual(writes, expected) {
		t.Fatalf("physical writes = %v, want %v", writes, expected)
	}
}

func TestPhysicalConnectIsAcceptedFromClosedState(t *testing.T) {
	mock, _, session := newPhysicalIntegrationServer(t, ToolConnect)
	var writes []physicalWrite
	mock.setRoute("/api/connection", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{"current":{"state":"Closed"}}`)
			return
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode connect payload: %v", err)
			return
		}
		writes = append(writes, physicalWrite{method: r.Method, path: r.URL.Path, body: body})
		w.WriteHeader(http.StatusNoContent)
	})
	mock.setRoute("/api/printer", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusConflict)
	})
	mock.setRoute("/api/job", jsonRoute(`{"state":"Offline"}`))

	result := callTool(t, session, ToolConnect, nil)
	output := decodeOutput[physicalCommandOutput](t, result)

	if output.Outcome != PhysicalAccepted {
		t.Errorf("outcome = %q, want %q", output.Outcome, PhysicalAccepted)
	}
	encoded, err := json.Marshal(result.StructuredContent)
	if err != nil || string(encoded) != `{"outcome":"accepted"}` {
		t.Errorf("structured content = %s, %v, want only the accepted outcome", encoded, err)
	}
	expected := []physicalWrite{{http.MethodPost, "/api/connection", decodePayload(t, `{"command":"connect"}`)}}
	if !reflect.DeepEqual(writes, expected) {
		t.Fatalf("physical writes = %v, want %v", writes, expected)
	}
}

func TestPhysicalSessionsShareToolsAndOctoPrintIdentity(t *testing.T) {
	mock, setup, first := newPhysicalIntegrationServer(t, ToolReconnect)
	second := connectSession(t, setup)

	firstTools := listTools(t, first)
	secondTools := listTools(t, second)
	if len(firstTools) != len(secondTools) {
		t.Fatalf("first client sees %d tools, second sees %d", len(firstTools), len(secondTools))
	}
	for name := range firstTools {
		if secondTools[name] == nil {
			t.Errorf("second client does not see tool %s", name)
		}
	}
	if firstTools[ToolReconnect] == nil {
		t.Fatal("configured tool was not published to the first client")
	}

	for _, session := range []*mcp.ClientSession{first, second} {
		output := decodeOutput[physicalCommandOutput](t, callTool(t, session, ToolReconnect, nil))
		if output.Outcome != PhysicalAccepted {
			t.Errorf("outcome = %q, want %q", output.Outcome, PhysicalAccepted)
		}
	}

	var reconnects int
	for _, request := range mock.recordedRequests() {
		if request == http.MethodPost+" /api/connection" {
			reconnects++
		}
	}
	if reconnects != 2 {
		t.Errorf("OctoPrint reconnect requests = %d, want 2", reconnects)
	}
}

func TestPhysicalUnknownResultIsNotRetried(t *testing.T) {
	mock, _, session := newPhysicalIntegrationServer(t, ToolJog)
	attempts := 0
	mock.setRoute("/api/printer/printhead", func(w http.ResponseWriter, _ *http.Request) {
		attempts++
		w.WriteHeader(http.StatusGatewayTimeout)
		fmt.Fprint(w, `{"error":"upstream-secret-detail"}`)
	})

	result := callTool(t, session, ToolJog, map[string]any{"x": 10, "speed": 1200})

	if result == nil || !result.IsError {
		t.Fatalf("expected an unknown-result error, got %+v", result)
	}
	if message := contentText(result); message != "physical: unknown_result" {
		t.Errorf("error message = %q, want %q", message, "physical: unknown_result")
	}
	encoded, err := json.Marshal(result.StructuredContent)
	if err != nil || string(encoded) != `{"outcome":"unknown"}` {
		t.Errorf("structured content = %s, %v, want unknown outcome", encoded, err)
	}
	if attempts != 1 {
		t.Errorf("write attempts = %d, want exactly one", attempts)
	}
}

func TestPhysicalToolErrorsAreSanitized(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status int
		want   string
	}{
		{name: "rejected argument", status: http.StatusBadRequest, want: "physical: argument_rejected"},
		{name: "denied permission", status: http.StatusForbidden, want: "physical: permission_denied"},
		{name: "conflicting resource", status: http.StatusConflict, want: "physical: resource_conflict"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			mock, _, session := newPhysicalIntegrationServer(t, ToolJog)
			mock.setRoute("/api/printer/printhead", func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tc.status)
				fmt.Fprint(w, `{"error":"upstream-secret-detail","key":"`+testAPIKey+`"}`)
			})

			result := callTool(t, session, ToolJog, map[string]any{"x": 10, "speed": 1200})

			if result == nil || !result.IsError {
				t.Fatalf("expected a sanitized tool error, got %+v", result)
			}
			message := contentText(result)
			if message != tc.want {
				t.Errorf("error message = %q, want %q", message, tc.want)
			}
			for _, forbidden := range []string{testAPIKey, "upstream-secret-detail", "/api/"} {
				if strings.Contains(message, forbidden) {
					t.Errorf("error message %q discloses %q", message, forbidden)
				}
			}
		})
	}
}

func TestPhysicalConnectionPermissionDenialIsSanitized(t *testing.T) {
	mock, _, session := newPhysicalIntegrationServer(t, ToolReconnect)
	mock.setRoute("/api/connection", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			jsonRoute(connectionBody)(w, r)
			return
		}
		w.WriteHeader(http.StatusForbidden)
		fmt.Fprint(w, `{"error":"upstream-secret-detail","key":"`+testAPIKey+`"}`)
	})

	result := callTool(t, session, ToolReconnect, nil)

	if result == nil || !result.IsError {
		t.Fatalf("expected a sanitized permission error, got %+v", result)
	}
	message := contentText(result)
	if message != "physical: permission_denied" {
		t.Errorf("error message = %q, want %q", message, "physical: permission_denied")
	}
	for _, forbidden := range []string{testAPIKey, "upstream-secret-detail", "/api/"} {
		if strings.Contains(message, forbidden) {
			t.Errorf("error message %q discloses %q", message, forbidden)
		}
	}
}

func TestPhysicalToolsRejectInvalidArgumentsWithoutWriting(t *testing.T) {
	for _, tc := range []struct {
		name string
		tool string
		args map[string]any
	}{
		{name: "jog beyond travel", tool: ToolJog, args: map[string]any{"x": 101, "speed": 1200}},
		{name: "jog without axes", tool: ToolJog, args: map[string]any{"speed": 1200}},
		{name: "home unknown axis", tool: ToolHome, args: map[string]any{"axes": []string{"e"}}},
		{name: "home without axes", tool: ToolHome, args: map[string]any{"axes": []string{}}},
		{name: "extrusion beyond length", tool: ToolExtrude, args: map[string]any{"amount": 11, "speed": 120}},
		{name: "tool outside the active profile", tool: ToolSetToolTemperature, args: map[string]any{"tool": "tool1", "target": 200}},
		{name: "malformed tool name", tool: ToolSelectTool, args: map[string]any{"tool": "extruder"}},
		{name: "feedrate below the allowed factor", tool: ToolSetFeedrate, args: map[string]any{"factor": 49}},
		{name: "feedrate above the allowed factor", tool: ToolSetFeedrate, args: map[string]any{"factor": 201}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			mock, _, session := newPhysicalIntegrationServer(t, tc.tool)

			result := callTool(t, session, tc.tool, tc.args)

			if result == nil || !result.IsError {
				t.Fatalf("expected an invalid-argument error, got %+v", result)
			}
			if message := contentText(result); !strings.HasPrefix(message, "physical: ") {
				t.Errorf("error message = %q, want a sanitized physical category", message)
			}
			for _, request := range mock.recordedRequests() {
				if strings.HasPrefix(request, http.MethodPost+" ") {
					t.Errorf("invalid arguments reached OctoPrint: %v", mock.recordedRequests())
				}
			}
		})
	}
}

func TestPhysicalToolAvailabilityRequiresLimitsAndDependencies(t *testing.T) {
	complete := testPhysicalLimits()
	for _, name := range physicalTools {
		if !physicalToolAvailable(name, complete) {
			t.Errorf("%s is unavailable with complete limits", name)
		}
	}

	withoutLimits := config.PhysicalLimits{}
	for _, name := range []string{ToolConnect, ToolReconnect, ToolDisconnect, ToolSetFeedrate, ToolInitSD, ToolRefreshSD, ToolReleaseSD} {
		if !physicalToolAvailable(name, withoutLimits) {
			t.Errorf("%s should not require numeric limits", name)
		}
	}
	for _, name := range []string{ToolJog, ToolHome, ToolExtrude, ToolSetToolTemperature, ToolSetToolOffset, ToolSelectTool, ToolSetBedTemperature, ToolSetBedOffset, ToolSetChamberTemperature, ToolSetChamberOffset} {
		if physicalToolAvailable(name, withoutLimits) {
			t.Errorf("%s should be unavailable without limits", name)
		}
	}

	axesWithoutValues := config.PhysicalLimits{Configured: config.PhysicalDomains{Axes: true}}
	if physicalToolAvailable(ToolJog, axesWithoutValues) {
		t.Error("jog should require at least one axis limit")
	}

	bedOnly := config.PhysicalLimits{
		Heaters:    map[string]config.HeaterLimits{"bed": complete.Heaters["bed"]},
		Configured: config.PhysicalDomains{Heaters: true},
	}
	if !physicalToolAvailable(ToolSetBedTemperature, bedOnly) {
		t.Error("bed temperature should be available with bed limits")
	}
	for _, name := range []string{ToolSetChamberTemperature, ToolSetToolTemperature, ToolSetToolOffset, ToolSelectTool} {
		if physicalToolAvailable(name, bedOnly) {
			t.Errorf("%s should be unavailable without its heater limits", name)
		}
	}

	withoutExtrusion := testPhysicalLimits()
	withoutExtrusion.Configured.Extrusion = false
	if physicalToolAvailable(ToolExtrude, withoutExtrusion) {
		t.Error("extrusion should be unavailable without extrusion limits")
	}
}

func TestPhysicalToolsWithoutLimits(t *testing.T) {
	missing := PhysicalToolsWithoutLimits([]string{ToolDeleteFile, ToolJog, ToolConnect, ToolExtrude}, config.PhysicalLimits{})
	if !slices.Equal(missing, []string{ToolJog, ToolExtrude}) {
		t.Errorf("missing limits = %v, want [printer_jog printer_extrude]", missing)
	}
	if missing := PhysicalToolsWithoutLimits([]string{ToolDeleteFile, ToolConnect, ToolInitSD}, config.PhysicalLimits{}); len(missing) != 0 {
		t.Errorf("missing limits = %v, want none", missing)
	}
	if missing := PhysicalToolsWithoutLimits(physicalTools, testPhysicalLimits()); len(missing) != 0 {
		t.Errorf("missing limits = %v, want none", missing)
	}
}

func TestEnabledPhysicalToolsAreWithheldWithoutLimits(t *testing.T) {
	_, _, session := newIntegrationServerWithLimits(t, config.PhysicalLimits{}, physicalTools...)

	tools := listTools(t, session)
	limited := []string{
		ToolJog,
		ToolHome,
		ToolExtrude,
		ToolSetToolTemperature,
		ToolSetToolOffset,
		ToolSelectTool,
		ToolSetBedTemperature,
		ToolSetBedOffset,
		ToolSetChamberTemperature,
		ToolSetChamberOffset,
	}
	for _, name := range limited {
		if tools[name] != nil {
			t.Errorf("tool %s is published without valid limits", name)
		}
	}
	for _, name := range []string{ToolConnect, ToolReconnect, ToolDisconnect, ToolSetFeedrate, ToolInitSD, ToolRefreshSD, ToolReleaseSD} {
		if tools[name] == nil {
			t.Errorf("tool %s should not require numeric limits", name)
		}
	}
	if instructions := session.InitializeResult().Instructions; strings.Contains(strings.ToLower(instructions), "read-only") {
		t.Errorf("instructions describe published connection tools as read-only: %s", instructions)
	}
}

func TestPartialPhysicalLimitsPublishOnlySupportedTools(t *testing.T) {
	limits := testPhysicalLimits()
	limits.Configured = config.PhysicalDomains{Axes: true}
	_, _, session := newIntegrationServerWithLimits(t, limits, physicalTools...)

	tools := listTools(t, session)
	for _, name := range []string{ToolConnect, ToolReconnect, ToolDisconnect, ToolJog, ToolHome, ToolSetFeedrate, ToolInitSD, ToolRefreshSD, ToolReleaseSD} {
		if tools[name] == nil {
			t.Errorf("tool %s should be published with axis limits", name)
		}
	}
	for _, name := range []string{ToolExtrude, ToolSetToolTemperature, ToolSetToolOffset, ToolSelectTool, ToolSetBedTemperature, ToolSetBedOffset, ToolSetChamberTemperature, ToolSetChamberOffset} {
		if tools[name] != nil {
			t.Errorf("tool %s is published without its required limits", name)
		}
	}
}

func TestConfiguredPhysicalToolDescriptionsNameRequiredPermissions(t *testing.T) {
	_, _, session := newPhysicalIntegrationServer(t, physicalTools...)

	tools := listTools(t, session)
	controlClause := "Requires the CONTROL permission."
	requiredPermissions := map[string]string{
		ToolConnect:               "Requires the CONNECTION permission.",
		ToolReconnect:             "Requires the CONNECTION permission.",
		ToolDisconnect:            "Requires the CONNECTION permission.",
		ToolJog:                   controlClause,
		ToolHome:                  controlClause,
		ToolSetFeedrate:           controlClause,
		ToolSetToolTemperature:    controlClause,
		ToolSetToolOffset:         controlClause,
		ToolSelectTool:            controlClause,
		ToolExtrude:               controlClause,
		ToolSetBedTemperature:     controlClause,
		ToolSetBedOffset:          controlClause,
		ToolSetChamberTemperature: controlClause,
		ToolSetChamberOffset:      controlClause,
		ToolInitSD:                controlClause,
		ToolRefreshSD:             controlClause,
		ToolReleaseSD:             controlClause,
	}
	if len(requiredPermissions) != len(physicalTools) {
		t.Fatalf("expected permission clauses cover %d tools, want %d", len(requiredPermissions), len(physicalTools))
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

func TestInstructionsReflectPublishedControls(t *testing.T) {
	t.Run("read-only when no write tools are enabled", func(t *testing.T) {
		_, _, session := newIntegrationServer(t)

		instructions := session.InitializeResult().Instructions

		if !strings.Contains(instructions, "read-only") {
			t.Errorf("instructions do not describe read-only access: %s", instructions)
		}
		if strings.Contains(instructions, "Enabled physical controls") {
			t.Errorf("instructions describe physical controls without published tools: %s", instructions)
		}
	})

	t.Run("physical controls are never described as read-only", func(t *testing.T) {
		_, _, session := newPhysicalIntegrationServer(t, physicalTools...)

		instructions := session.InitializeResult().Instructions

		if strings.Contains(strings.ToLower(instructions), "read-only") {
			t.Errorf("instructions describe write access as read-only: %s", instructions)
		}
		if !strings.Contains(instructions, "Enabled physical controls") {
			t.Errorf("instructions do not describe the published physical controls: %s", instructions)
		}
		if !strings.Contains(instructions, "OctoPrint acceptance does not confirm physical execution") {
			t.Errorf("instructions do not limit physical confirmation: %s", instructions)
		}
	})

	t.Run("physical controls describe only the published subset", func(t *testing.T) {
		_, _, session := newIntegrationServerWithLimits(t, config.PhysicalLimits{}, ToolReconnect)

		tools := listTools(t, session)
		if tools[ToolReconnect] == nil {
			t.Fatalf("tool %s was not published", ToolReconnect)
		}

		instructions := session.InitializeResult().Instructions

		if !strings.Contains(instructions, "Enabled physical controls cover connection and require") {
			t.Errorf("instructions do not describe the published connection control: %s", instructions)
		}
		for _, absent := range []string{"printhead movement", "feedrate", "heater targets", "tool selection", "extrusion", "SD operations"} {
			if strings.Contains(instructions, absent) {
				t.Errorf("instructions list absent control %q: %s", absent, instructions)
			}
		}
	})
}

func TestNewServerValidatesEnabledTools(t *testing.T) {
	if _, err := NewServer(readerStub{}, []string{"printer_unknown"}, config.PhysicalLimits{}); err == nil {
		t.Fatal("unknown tool name was accepted")
	}
	if _, err := NewServer(readerStub{}, []string{ToolStartJob, ToolStartJob}, config.PhysicalLimits{}); err == nil {
		t.Fatal("duplicate tool name was accepted")
	}
	if _, err := NewServer(readerStub{}, []string{ToolDeleteFile}, config.PhysicalLimits{}); err == nil {
		t.Fatal("file tool was accepted without a file manager")
	}
	if _, err := NewServer(readerStub{}, []string{ToolStartJob}, config.PhysicalLimits{}); err == nil {
		t.Fatal("job tool was accepted without a job controller")
	}
	if _, err := NewServer(readerStub{}, []string{ToolConnect}, config.PhysicalLimits{}); err == nil {
		t.Fatal("physical tool was accepted without a physical reader and writer")
	}
	if _, err := NewServer(readerStub{}, []string{ToolJog}, config.PhysicalLimits{}); err != nil {
		t.Fatalf("physical tool without limits should be withheld instead of rejected: %v", err)
	}
}

func TestJobCommandRejectsBusyPhysicalCoordinator(t *testing.T) {
	coordinator := &PhysicalCoordinator{}
	release, reserved := coordinator.tryReserve()
	if !reserved {
		t.Fatal("failed to reserve physical coordinator")
	}
	defer release()
	called := false
	handler := jobCommandHandler(func(context.Context) error {
		called = true
		return nil
	}, coordinator)

	_, _, err := handler(context.Background(), nil, struct{}{})

	if err == nil || err.Error() != "physical: operation_busy" {
		t.Fatalf("busy error = %v, want physical: operation_busy", err)
	}
	if called {
		t.Fatal("job command reached OctoPrint while the physical coordinator was busy")
	}
}

func TestJobCommandRejectsCanceledContextBeforeOctoPrint(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	called := false
	handler := jobCommandHandler(func(context.Context) error {
		called = true
		return nil
	}, nil)

	_, _, err := handler(ctx, nil, struct{}{})

	if err == nil {
		t.Fatal("canceled job command did not return an error")
	}
	if called {
		t.Fatal("canceled job command reached OctoPrint")
	}
}

func TestJobCommandsCoordinateWithPhysicalOperations(t *testing.T) {
	mock, setup, physicalSession := newIntegrationServerWithLimits(t, testPhysicalLimits(), ToolSetBedTemperature, ToolStartJob, ToolResumeJob, ToolRestartJob, ToolPauseJob, ToolCancelJob)
	jobSession := connectSession(t, setup)
	writeStarted := make(chan struct{})
	releaseWrite := make(chan struct{})
	var releaseOnce sync.Once
	release := func() { releaseOnce.Do(func() { close(releaseWrite) }) }
	defer release()
	mock.setRoute("/api/printer/bed", func(w http.ResponseWriter, r *http.Request) {
		close(writeStarted)
		select {
		case <-releaseWrite:
			w.WriteHeader(http.StatusNoContent)
		case <-r.Context().Done():
			w.WriteHeader(http.StatusGatewayTimeout)
		}
	})
	jobCommands := make(chan string, 2)
	mock.setRoute("/api/job", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			fmt.Fprint(w, physicalJobBody)
			return
		}
		var payload struct {
			Command string `json:"command"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Errorf("decode job command: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		jobCommands <- payload.Command
		w.WriteHeader(http.StatusNoContent)
	})

	physicalDone := make(chan error, 1)
	physicalContext, cancelPhysical := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancelPhysical()
	go func() {
		_, err := physicalSession.CallTool(physicalContext, &mcp.CallToolParams{Name: ToolSetBedTemperature, Arguments: map[string]any{"target": 50}})
		physicalDone <- err
	}()
	select {
	case <-writeStarted:
	case <-time.After(time.Second):
		t.Fatal("physical write did not reach OctoPrint")
	}

	for _, tool := range []string{ToolStartJob, ToolResumeJob, ToolRestartJob} {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		result, err := jobSession.CallTool(ctx, &mcp.CallToolParams{Name: tool})
		cancel()
		if err != nil {
			t.Fatalf("%s call: %v", tool, err)
		}
		if result == nil || !result.IsError || contentText(result) != "physical: operation_busy" {
			t.Fatalf("%s result = %+v, want physical: operation_busy", tool, result)
		}
	}
	select {
	case command := <-jobCommands:
		t.Fatalf("start job reached OctoPrint during physical write as %q", command)
	default:
	}

	for _, tool := range []string{ToolPauseJob, ToolCancelJob} {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		result, err := jobSession.CallTool(ctx, &mcp.CallToolParams{Name: tool})
		cancel()
		if err != nil {
			t.Fatalf("%s call: %v", tool, err)
		}
		if result == nil || result.IsError {
			t.Fatalf("%s result = %+v, want success while physical write is in progress", tool, result)
		}
		select {
		case command := <-jobCommands:
			if tool == ToolPauseJob && command != "pause" || tool == ToolCancelJob && command != "cancel" {
				t.Fatalf("%s reached OctoPrint as command %q", tool, command)
			}
		case <-time.After(time.Second):
			t.Fatalf("%s did not reach OctoPrint while physical write was in progress", tool)
		}
	}

	release()
	select {
	case err := <-physicalDone:
		if err != nil {
			t.Fatalf("physical write: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("physical operation did not finish after releasing OctoPrint")
	}
}
