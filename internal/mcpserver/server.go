package mcpserver

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/joaaomanooel/tupi-mcp/internal/octoprint"
)

const (
	serverName    = "tupi-mcp"
	serverVersion = "0.1.0"
)

type Reader interface {
	Connection(ctx context.Context) (*octoprint.ConnectionResponse, error)
	Printer(ctx context.Context) (*octoprint.PrinterResponse, error)
	Job(ctx context.Context) (*octoprint.JobResponse, error)
	Files(ctx context.Context) (*octoprint.FilesResponse, error)
	UploadGCode(ctx context.Context, filename string, contents []byte) (*octoprint.UploadResponse, error)
}

func NewServer(reader Reader) *mcp.Server {
	server := mcp.NewServer(
		&mcp.Implementation{Name: serverName, Version: serverVersion},
		&mcp.ServerOptions{Instructions: "OctoPrint access provides read-only status tools and one bounded G-code upload tool. Uploads go to local storage with selection and printing disabled."},
	)
	mcp.AddTool(server, &mcp.Tool{
		Name:         "printer_get_status",
		Description:  "Read the OctoPrint connection state and the printer state reported by the printer.",
		OutputSchema: statusOutputSchema,
	}, statusHandler(reader))
	mcp.AddTool(server, &mcp.Tool{
		Name:         "printer_get_temperatures",
		Description:  "Read the current and target temperatures of the heaters reported by OctoPrint.",
		OutputSchema: temperaturesOutputSchema,
	}, temperaturesHandler(reader))
	mcp.AddTool(server, &mcp.Tool{
		Name:         "printer_get_job",
		Description:  "Read the current OctoPrint job, its progress and its print time estimates.",
		OutputSchema: jobOutputSchema,
	}, jobHandler(reader))
	mcp.AddTool(server, &mcp.Tool{
		Name:         "printer_list_files",
		Description:  "List the files and folders reported by OctoPrint.",
		OutputSchema: filesOutputSchema,
	}, filesHandler(reader))
	mcp.AddTool(server, &mcp.Tool{
		Name:         "printer_upload_gcode",
		Description:  "Upload a G-code file to OctoPrint local storage without selecting or printing it. Accepts a filename ending in .gcode and at most 8 MiB of contents.",
		OutputSchema: uploadOutputSchema,
	}, uploadHandler(reader))
	return server
}

type uploadInput struct {
	Filename string `json:"filename" jsonschema:"G-code filename ending in .gcode"`
	Contents string `json:"contents" jsonschema:"G-code file contents, maximum 8 MiB"`
}

func uploadHandler(reader Reader) mcp.ToolHandlerFor[uploadInput, uploadOutput] {
	return func(ctx context.Context, _ *mcp.CallToolRequest, input uploadInput) (*mcp.CallToolResult, uploadOutput, error) {
		response, err := reader.UploadGCode(ctx, input.Filename, []byte(input.Contents))
		if err != nil {
			return nil, uploadOutput{}, toolError(err)
		}
		return nil, newUploadOutput(response), nil
	}
}

func statusHandler(reader Reader) mcp.ToolHandlerFor[struct{}, statusOutput] {
	return func(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, statusOutput, error) {
		connection, err := reader.Connection(ctx)
		if err != nil {
			return nil, statusOutput{}, toolError(err)
		}
		printer, err := reader.Printer(ctx)
		if err != nil {
			if isPrinterNotOperational(err) {
				return nil, newStatusOutput(connection, nil), nil
			}
			return nil, statusOutput{}, toolError(err)
		}
		return nil, newStatusOutput(connection, printer), nil
	}
}

func isPrinterNotOperational(err error) bool {
	var operationError *octoprint.Error
	if !errors.As(err, &operationError) {
		return false
	}
	return operationError.Kind == octoprint.ErrorHTTPStatus && operationError.StatusCode == http.StatusConflict
}

func temperaturesHandler(reader Reader) mcp.ToolHandlerFor[struct{}, temperaturesOutput] {
	return func(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, temperaturesOutput, error) {
		printer, err := reader.Printer(ctx)
		if err != nil {
			return nil, temperaturesOutput{}, toolError(err)
		}
		return nil, newTemperaturesOutput(printer), nil
	}
}

func jobHandler(reader Reader) mcp.ToolHandlerFor[struct{}, jobOutput] {
	return func(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, jobOutput, error) {
		job, err := reader.Job(ctx)
		if err != nil {
			return nil, jobOutput{}, toolError(err)
		}
		return nil, newJobOutput(job), nil
	}
}

func filesHandler(reader Reader) mcp.ToolHandlerFor[struct{}, filesOutput] {
	return func(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, filesOutput, error) {
		files, err := reader.Files(ctx)
		if err != nil {
			return nil, filesOutput{}, toolError(err)
		}
		return nil, newFilesOutput(files), nil
	}
}

func toolError(err error) error {
	var operationError *octoprint.Error
	if errors.As(err, &operationError) {
		if operationError.Kind == octoprint.ErrorHTTPStatus {
			return fmt.Errorf("octoprint request failed: %s (status %d)", operationError.Kind, operationError.StatusCode)
		}
		if operationError.Kind != "" {
			return fmt.Errorf("octoprint request failed: %s", operationError.Kind)
		}
	}
	return errors.New("octoprint request failed")
}
