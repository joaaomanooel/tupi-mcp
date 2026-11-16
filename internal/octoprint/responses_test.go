package octoprint

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
)

func TestConnectionResponseMapping(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"current":{"state":"Operational","port":"/dev/ttyUSB0","baudrate":115200,"printerProfile":"_default"},"options":{"ports":[]},"future":true}`)
	}))
	defer server.Close()
	client := newTestClient(t, server.URL, 2048)

	response, err := client.Connection(context.Background())

	if err != nil {
		t.Fatal(err)
	}
	if response.Current == nil || response.Current.State == nil || *response.Current.State != "Operational" {
		t.Fatalf("unexpected connection state: %+v", response)
	}
	if response.Current.Port == nil || *response.Current.Port != "/dev/ttyUSB0" || response.Current.Baudrate == nil || *response.Current.Baudrate != 115200 || response.Current.PrinterProfile == nil || *response.Current.PrinterProfile != "_default" {
		t.Fatalf("missing connection settings: %+v", response.Current)
	}
}

func TestDisconnectedConnectionPreservesAbsentSettings(t *testing.T) {
	client := responseClient(t, `{"current":{"state":"Closed","port":null,"baudrate":null}}`)

	response, err := client.Connection(context.Background())

	if err != nil {
		t.Fatal(err)
	}
	if response.Current == nil || response.Current.State == nil || *response.Current.State != "Closed" || response.Current.Port != nil || response.Current.Baudrate != nil || response.Current.PrinterProfile != nil {
		t.Fatalf("unexpected disconnected connection: %+v", response)
	}
}

func TestPrintingPrinterResponse(t *testing.T) {
	client := responseClient(t, `{"state":{"text":"Printing","flags":{"operational":true,"printing":true,"paused":false,"ready":false,"error":false,"closedOrError":false,"future":[]}},"temperature":{"tool0":{"actual":201.25,"target":205,"offset":0},"tool2":{"actual":null,"target":0},"bed":{"actual":59.5,"target":60},"chamber":{"actual":31},"history":[],"future":true},"sd":{"ready":true}}`)

	response, err := client.Printer(context.Background())

	if err != nil {
		t.Fatal(err)
	}
	if response.State == nil || response.State.Text == nil || *response.State.Text != "Printing" || response.State.Flags == nil {
		t.Fatalf("missing printer state: %+v", response)
	}
	flags := response.State.Flags
	if flags.Operational == nil || !*flags.Operational || flags.Printing == nil || !*flags.Printing || flags.Paused == nil || *flags.Paused || flags.Ready == nil || *flags.Ready || flags.Error == nil || *flags.Error || flags.ClosedOrError == nil || *flags.ClosedOrError {
		t.Fatalf("unexpected state flags: %+v", flags)
	}
	tool := response.Temperature["tool0"]
	if len(response.Temperature) != 4 || tool.Actual == nil || *tool.Actual != 201.25 || tool.Target == nil || *tool.Target != 205 || tool.Offset == nil || *tool.Offset != 0 {
		t.Fatalf("unexpected temperatures: %+v", response.Temperature)
	}
	if response.Temperature["tool2"].Actual != nil || response.Temperature["tool2"].Target == nil || *response.Temperature["tool2"].Target != 0 || response.Temperature["bed"].Actual == nil || *response.Temperature["bed"].Actual != 59.5 || response.Temperature["chamber"].Target != nil {
		t.Fatalf("temperature availability was lost: %+v", response.Temperature)
	}
}

func TestPrintingJobResponse(t *testing.T) {
	client := responseClient(t, `{"job":{"file":{"name":"part.gcode","display":"Part","path":"parts/part.gcode","origin":"local","size":1468987,"date":1378847754},"estimatedPrintTime":1188.5,"averagePrintTime":1200,"lastPrintTime":1210,"filament":{"tool0":{"length":810,"volume":5.36}}},"progress":{"completion":42.5,"filepos":12345,"printTime":500,"printTimeLeft":688,"printTimeLeftOrigin":"analysis"},"state":"Printing"}`)

	response, err := client.Job(context.Background())

	if err != nil {
		t.Fatal(err)
	}
	if response.State == nil || *response.State != "Printing" || response.Job == nil || response.Job.File == nil || response.Progress == nil {
		t.Fatalf("missing job details: %+v", response)
	}
	file := response.Job.File
	if file.Name == nil || *file.Name != "part.gcode" || file.Display == nil || *file.Display != "Part" || file.Path == nil || *file.Path != "parts/part.gcode" || file.Origin == nil || *file.Origin != "local" || file.Size == nil || *file.Size != 1468987 || file.Date == nil || *file.Date != 1378847754 {
		t.Fatalf("unexpected job file: %+v", file)
	}
	if response.Job.EstimatedPrintTime == nil || *response.Job.EstimatedPrintTime != 1188.5 || response.Job.AveragePrintTime == nil || *response.Job.AveragePrintTime != 1200 || response.Job.LastPrintTime == nil || *response.Job.LastPrintTime != 1210 {
		t.Fatalf("missing estimates: %+v", response.Job)
	}
	progress := response.Progress
	if progress.Completion == nil || *progress.Completion != 42.5 || progress.FilePosition == nil || *progress.FilePosition != 12345 || progress.PrintTime == nil || *progress.PrintTime != 500 || progress.PrintTimeLeft == nil || *progress.PrintTimeLeft != 688 || progress.PrintTimeLeftOrigin == nil || *progress.PrintTimeLeftOrigin != "analysis" {
		t.Fatalf("unexpected progress: %+v", progress)
	}
}

func TestIdlePrinterPreservesMissingTemperatures(t *testing.T) {
	client := responseClient(t, `{"state":{"text":"Operational","flags":{"printing":false}}}`)

	response, err := client.Printer(context.Background())

	if err != nil {
		t.Fatal(err)
	}
	if response.Temperature != nil || response.State == nil || response.State.Flags == nil || response.State.Flags.Printing == nil || *response.State.Flags.Printing || response.State.Flags.Ready != nil {
		t.Fatalf("unexpected idle printer details: %+v", response)
	}
}

func TestIdleJobWithoutActiveFile(t *testing.T) {
	for _, body := range []string{
		`{"job":null,"progress":null,"state":"Operational"}`,
		`{"job":{"file":null,"estimatedPrintTime":null},"progress":{"completion":null,"filepos":null,"printTime":null,"printTimeLeft":null},"state":"Operational"}`,
		`{"job":{"file":{"name":null,"origin":null,"size":null,"date":null}},"state":"Operational"}`,
	} {
		t.Run(body, func(t *testing.T) {
			client := responseClient(t, body)

			response, err := client.Job(context.Background())

			if err != nil {
				t.Fatal(err)
			}
			if response.State == nil || *response.State != "Operational" {
				t.Fatalf("unexpected idle state: %+v", response)
			}
			if response.Job != nil && (response.Job.EstimatedPrintTime != nil || response.Job.File != nil && response.Job.File.Name != nil) {
				t.Fatalf("fabricated job details: %+v", response.Job)
			}
			if response.Progress != nil && (response.Progress.Completion != nil || response.Progress.FilePosition != nil || response.Progress.PrintTime != nil || response.Progress.PrintTimeLeft != nil) {
				t.Fatalf("fabricated progress: %+v", response.Progress)
			}
		})
	}
}

func TestPopulatedFileList(t *testing.T) {
	client := responseClient(t, `{"files":[{"name":"part.gcode","display":"Part","path":"part.gcode","type":"machinecode","origin":"local","size":1468987,"date":1378847754,"refs":{"resource":"http://example.com/api/files/local/part.gcode"}},{"name":"part.gco","path":"part.gco","type":"machinecode","origin":"sdcard"},{"name":"parts","path":"parts","type":"folder","origin":"local","children":[{"name":"nested.gcode","path":"parts/nested.gcode","type":"machinecode"}]}],"free":12345,"total":45678}`)

	response, err := client.Files(context.Background())

	if err != nil {
		t.Fatal(err)
	}
	if len(response.Files) != 3 {
		t.Fatalf("unexpected file list: %+v", response)
	}
	file := response.Files[0]
	if file.Name == nil || *file.Name != "part.gcode" || file.Display == nil || *file.Display != "Part" || file.Path == nil || *file.Path != "part.gcode" || file.Type == nil || *file.Type != "machinecode" || file.Origin == nil || *file.Origin != "local" || file.Size == nil || *file.Size != 1468987 || file.Date == nil || *file.Date != 1378847754 {
		t.Fatalf("unexpected local file: %+v", file)
	}
	if response.Files[1].Size != nil || response.Files[1].Date != nil || response.Files[1].Origin == nil || *response.Files[1].Origin != "sdcard" {
		t.Fatalf("missing file metadata was fabricated: %+v", response.Files[1])
	}
	if len(response.Files[2].Children) != 1 || response.Files[2].Children[0].Path == nil || *response.Files[2].Children[0].Path != "parts/nested.gcode" {
		t.Fatalf("missing folder child: %+v", response.Files[2])
	}
}

func TestEmptyFileListRemainsPresent(t *testing.T) {
	client := responseClient(t, `{"files":[]}`)

	response, err := client.Files(context.Background())

	if err != nil {
		t.Fatal(err)
	}
	if response.Files == nil || len(response.Files) != 0 {
		t.Fatalf("unexpected empty file list: %+v", response.Files)
	}
}

func TestMissingOptionalResponseFields(t *testing.T) {
	client := responseClient(t, `{"future":{"anything":true}}`)

	connection, connectionErr := client.Connection(context.Background())
	printer, printerErr := client.Printer(context.Background())
	job, jobErr := client.Job(context.Background())
	files, filesErr := client.Files(context.Background())

	if connectionErr != nil || printerErr != nil || jobErr != nil || filesErr != nil {
		t.Fatalf("missing optional fields rejected: %v %v %v %v", connectionErr, printerErr, jobErr, filesErr)
	}
	if connection.Current != nil || printer.State != nil || printer.Temperature != nil || job.State != nil || job.Job != nil || job.Progress != nil || files.Files != nil {
		t.Fatalf("missing response fields were fabricated: %+v %+v %+v %+v", connection, printer, job, files)
	}
}

func TestIncompatibleResponseFieldsAreSafe(t *testing.T) {
	for _, tc := range []struct {
		endpoint, body string
	}{
		{"connection", `{"current":{"state":123}}`},
		{"connection", `{"current":{"baudrate":"private detail"}}`},
		{"printer", `{"state":{"flags":{"printing":"private detail"}}}`},
		{"printer", `{"temperature":{"tool0":{"actual":"private detail"}}}`},
		{"printer", `{"temperature":[]}`},
		{"job", `{"job":{"file":[]}}`},
		{"job", `{"progress":{"completion":"private detail"}}`},
		{"job", `{"job":{"estimatedPrintTime":true}}`},
		{"files", `{"files":{}}`},
		{"files", `{"files":[{"size":"private detail"}]}`},
		{"files", `{"files":[{"children":false}]}`},
	} {
		t.Run(tc.endpoint+tc.body, func(t *testing.T) {
			client := responseClient(t, tc.body)

			response, err := readEndpoint(client, tc.endpoint, context.Background())

			assertKind(t, err, ErrorInvalidJSON)
			if !reflect.ValueOf(response).IsNil() || errors.Unwrap(err) != nil || strings.Contains(err.Error(), "private detail") {
				t.Fatalf("unsafe decoding error: %v", err)
			}
		})
	}
}

func TestEndpointResponsesRequireObjects(t *testing.T) {
	for _, endpoint := range []string{"connection", "printer", "job", "files"} {
		for _, body := range []string{`null`, `[]`, `true`, `123e10000`, `"private detail"`} {
			t.Run(endpoint+body, func(t *testing.T) {
				client := responseClient(t, body)

				response, err := readEndpoint(client, endpoint, context.Background())

				assertKind(t, err, ErrorInvalidJSON)
				if !reflect.ValueOf(response).IsNil() {
					t.Fatalf("incompatible response returned data: %+v", response)
				}
			})
		}
	}
}

func TestEndpointResponseErrorsPreserveSafeSemantics(t *testing.T) {
	for _, endpoint := range []string{"connection", "printer", "job", "files"} {
		for _, tc := range []struct {
			name, body string
			status     int
			kind       ErrorKind
		}{
			{"malformed", `{"private detail":`, http.StatusOK, ErrorInvalidJSON},
			{"disconnected", "private upstream detail", http.StatusConflict, ErrorHTTPStatus},
			{"unauthorized", "private upstream detail", http.StatusUnauthorized, ErrorHTTPStatus},
			{"secret", `{"unknown":"test-\u0073ecret"}`, http.StatusOK, ErrorSensitiveResponse},
		} {
			t.Run(endpoint+tc.name, func(t *testing.T) {
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(tc.status)
					fmt.Fprint(w, tc.body)
				}))
				defer server.Close()
				client := newTestClient(t, server.URL, 8192)

				response, err := readEndpoint(client, endpoint, context.Background())

				assertKind(t, err, tc.kind)
				if !reflect.ValueOf(response).IsNil() || errors.Unwrap(err) != nil || strings.Contains(err.Error(), "private") || strings.Contains(err.Error(), "test-secret") {
					t.Fatalf("unsafe endpoint response: %v", err)
				}
				var typed *Error
				errors.As(err, &typed)
				if tc.kind == ErrorHTTPStatus && typed.StatusCode != tc.status {
					t.Fatalf("missing HTTP status: %d", typed.StatusCode)
				}
			})
		}
	}
}

func responseClient(t *testing.T, body string) *Client {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, body)
	}))
	t.Cleanup(server.Close)
	return newTestClient(t, server.URL, 8192)
}

func readEndpoint(client *Client, endpoint string, ctx context.Context) (any, error) {
	switch endpoint {
	case "connection":
		return client.Connection(ctx)
	case "printer":
		return client.Printer(ctx)
	case "job":
		return client.Job(ctx)
	default:
		return client.Files(ctx)
	}
}
