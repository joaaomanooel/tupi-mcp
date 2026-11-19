package mcpserver

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/joaaomanooel/tupi-mcp/internal/config"
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

type ExtendedReader interface {
	Version(ctx context.Context) (*octoprint.VersionResponse, error)
	Server(ctx context.Context) (*octoprint.ServerResponse, error)
	ToolTemperatures(ctx context.Context) (*octoprint.ToolTemperaturesResponse, error)
	BedTemperature(ctx context.Context) (*octoprint.BedTemperatureResponse, error)
	ChamberTemperature(ctx context.Context) (*octoprint.ChamberTemperatureResponse, error)
	SDState(ctx context.Context) (*octoprint.SDStateResponse, error)
	LastPrinterError(ctx context.Context) (*octoprint.PrinterErrorResponse, error)
	Profiles(ctx context.Context) (*octoprint.ProfilesResponse, error)
	Slicing(ctx context.Context) (*octoprint.SlicingResponse, error)
	Timelapse(ctx context.Context) (*octoprint.TimelapseResponse, error)
	Settings(ctx context.Context) (*octoprint.SettingsResponse, error)
}

func NewServer(reader Reader, enabledTools []string, physicalLimits config.PhysicalLimits) (*mcp.Server, error) {
	enabled, err := enabledToolSet(enabledTools)
	if err != nil {
		return nil, err
	}
	publishedPhysical := availablePhysicalTools(enabled, physicalLimits)
	instructions := serverInstructions(enabled, publishedPhysical)
	server := mcp.NewServer(
		&mcp.Implementation{Name: serverName, Version: serverVersion},
		&mcp.ServerOptions{Instructions: instructions},
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
		Description:  "List files and folders and read the local uploads filesystem capacity reported by OctoPrint. Requires the FILES_LIST permission.",
		OutputSchema: filesOutputSchema,
	}, filesHandler(reader))
	if extended, ok := reader.(ExtendedReader); ok {
		mcp.AddTool(server, &mcp.Tool{
			Name:         "printer_get_version",
			Description:  "Read the OctoPrint API version and server version reported by OctoPrint. Requires the STATUS permission.",
			OutputSchema: versionOutputSchema,
		}, extendedHandler(extended, ExtendedReader.Version, newVersionOutput))
		mcp.AddTool(server, &mcp.Tool{
			Name:         "printer_get_server",
			Description:  "Read the OctoPrint server version and safe mode status. Requires the STATUS permission.",
			OutputSchema: serverOutputSchema,
		}, extendedHandler(extended, ExtendedReader.Server, newServerOutput))
		mcp.AddTool(server, &mcp.Tool{
			Name:         "printer_get_tool_temperatures",
			Description:  "Read tool temperatures reported by OctoPrint. Requires the STATUS permission.",
			OutputSchema: toolTemperaturesOutputSchema,
		}, extendedHandler(extended, ExtendedReader.ToolTemperatures, newToolTemperaturesOutput))
		mcp.AddTool(server, &mcp.Tool{
			Name:         "printer_get_bed_temperature",
			Description:  "Read bed temperatures reported by OctoPrint. Requires the STATUS permission.",
			OutputSchema: bedTemperatureOutputSchema,
		}, extendedHandler(extended, ExtendedReader.BedTemperature, newBedTemperatureOutput))
		mcp.AddTool(server, &mcp.Tool{
			Name:         "printer_get_chamber_temperature",
			Description:  "Read chamber temperatures reported by OctoPrint. Requires the STATUS permission.",
			OutputSchema: chamberTemperatureOutputSchema,
		}, extendedHandler(extended, ExtendedReader.ChamberTemperature, newChamberTemperatureOutput))
		mcp.AddTool(server, &mcp.Tool{
			Name:         "printer_get_sd_state",
			Description:  "Read whether the printer SD card is ready. Requires the STATUS permission.",
			OutputSchema: sdStateOutputSchema,
		}, extendedHandler(extended, ExtendedReader.SDState, newSDStateOutput))
		mcp.AddTool(server, &mcp.Tool{
			Name:         "printer_get_last_error",
			Description:  "Read the latest printer error without serial terminal logs. Requires the STATUS permission.",
			OutputSchema: printerErrorOutputSchema,
		}, extendedHandler(extended, ExtendedReader.LastPrinterError, newPrinterErrorOutput))
		mcp.AddTool(server, &mcp.Tool{
			Name:         "printer_get_profiles",
			Description:  "List printer profiles reported by OctoPrint. Requires the CONNECTION permission.",
			OutputSchema: profilesOutputSchema,
		}, extendedHandler(extended, ExtendedReader.Profiles, newProfilesOutput))
		mcp.AddTool(server, &mcp.Tool{
			Name:         "printer_get_slicing",
			Description:  "List the slicing profiles available in OctoPrint grouped by slicer. Requires the SLICE permission.",
			OutputSchema: slicingOutputSchema,
		}, extendedHandler(extended, ExtendedReader.Slicing, newSlicingOutput))
		mcp.AddTool(server, &mcp.Tool{
			Name:         "printer_get_timelapse",
			Description:  "List rendered and unrendered timelapse files reported by OctoPrint. Requires the TIMELAPSE_LIST permission.",
			OutputSchema: timelapseOutputSchema,
		}, extendedHandler(extended, ExtendedReader.Timelapse, newTimelapseOutput))
		mcp.AddTool(server, &mcp.Tool{
			Name:         "printer_get_settings",
			Description:  "Read the OctoPrint feature settings for the temperature graph, SD support, model size detection, print start and cancel confirmations, upload overwrite confirmation and file delete confirmation. Requires the SETTINGS_READ permission.",
			OutputSchema: settingsOutputSchema,
		}, extendedHandler(extended, ExtendedReader.Settings, newSettingsOutput))
	}
	mcp.AddTool(server, &mcp.Tool{
		Name:         "printer_upload_gcode",
		Description:  "Upload UTF-8 G-code text to OctoPrint local storage without selecting or printing it, and refuse to overwrite an existing file. Accepts a filename ending in .gcode and at most 8 MiB of contents.",
		OutputSchema: uploadOutputSchema,
	}, uploadHandler(reader))
	if manager, ok := reader.(FileManager); ok {
		addFileTools(server, manager, enabled)
	} else if anyEnabled(enabled, fileTools...) {
		return nil, errors.New("mcpserver: enabled file tools require a file manager")
	}
	if controller, ok := reader.(JobController); ok {
		addJobTools(server, controller, enabled)
	} else if anyEnabled(enabled, jobTools...) {
		return nil, errors.New("mcpserver: enabled job tools require a job controller")
	}
	if len(publishedPhysical) > 0 {
		physicalReader, readerOK := reader.(PhysicalReader)
		writer, writerOK := reader.(PhysicalWriter)
		if !readerOK || !writerOK {
			return nil, errors.New("mcpserver: enabled physical tools require a physical reader and writer")
		}
		addPhysicalTools(server, NewPhysicalCoordinator(physicalReader, writer, physicalLimits), publishedPhysical)
	}
	return server, nil
}

func serverInstructions(enabled, publishedPhysical map[string]struct{}) string {
	instructions := "OctoPrint access provides read-only status and diagnostic tools plus one bounded G-code upload tool. Uploads go to local storage with selection and printing disabled."
	if anyEnabled(enabled, fileTools...) || anyEnabled(enabled, jobTools...) || len(publishedPhysical) > 0 {
		instructions = "OctoPrint access provides status, temperature, job, file listing and diagnostic tools plus one bounded G-code upload tool. Uploads go to local storage with selection and printing disabled."
	}
	if anyEnabled(enabled, fileTools...) {
		instructions += " Enabled file-management tools operate on OctoPrint local storage and require the permissions stated in each tool description."
	}
	if anyEnabled(enabled, jobTools...) {
		instructions += " Enabled job-control tools act on the current job and require the PRINT permission."
	}
	if len(publishedPhysical) > 0 {
		instructions += " Enabled physical controls cover " + publishedPhysicalControls(publishedPhysical) + " and require the permissions stated in each tool description. Every physical command re-reads printer state, enforces deployment limits, and reports only that OctoPrint accepted the request or that the outcome is unknown. OctoPrint acceptance does not confirm physical execution."
	}
	return instructions
}

var physicalControlCategories = []struct {
	description string
	tools       []string
}{
	{"connection", []string{ToolConnect, ToolReconnect, ToolDisconnect}},
	{"printhead movement", []string{ToolJog, ToolHome}},
	{"feedrate", []string{ToolSetFeedrate}},
	{"heater targets and offsets", []string{ToolSetToolTemperature, ToolSetToolOffset, ToolSetBedTemperature, ToolSetBedOffset, ToolSetChamberTemperature, ToolSetChamberOffset}},
	{"tool selection", []string{ToolSelectTool}},
	{"extrusion", []string{ToolExtrude}},
	{"SD operations", []string{ToolInitSD, ToolRefreshSD, ToolReleaseSD}},
}

func publishedPhysicalControls(published map[string]struct{}) string {
	present := make([]string, 0, len(physicalControlCategories))
	for _, category := range physicalControlCategories {
		if anyEnabled(published, category.tools...) {
			present = append(present, category.description)
		}
	}
	if len(present) == 0 {
		return "only the physical tools published in this session"
	}
	if len(present) == 1 {
		return present[0]
	}
	return strings.Join(present[:len(present)-1], ", ") + " and " + present[len(present)-1]
}

type uploadInput struct {
	Filename string `json:"filename" jsonschema:"G-code filename ending in .gcode"`
	Contents string `json:"contents" jsonschema:"UTF-8 G-code text, maximum 8 MiB"`
}

func uploadHandler(reader Reader) mcp.ToolHandlerFor[uploadInput, uploadOutput] {
	return func(ctx context.Context, _ *mcp.CallToolRequest, input uploadInput) (*mcp.CallToolResult, uploadOutput, error) {
		if len(input.Contents) > octoprint.MaxGCodeUploadBytes {
			return nil, uploadOutput{}, toolError(&octoprint.Error{Kind: octoprint.ErrorRequestTooLarge})
		}
		contents := []byte(input.Contents)
		if !octoprint.ValidGCodeContents(contents) {
			return nil, uploadOutput{}, toolError(&octoprint.Error{Kind: octoprint.ErrorInvalidUpload})
		}
		response, err := reader.UploadGCode(ctx, input.Filename, contents)
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

func extendedHandler[T any, O any](reader ExtendedReader, read func(ExtendedReader, context.Context) (*T, error), newOutput func(*T) O) mcp.ToolHandlerFor[struct{}, O] {
	return func(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, O, error) {
		var zero O
		response, err := read(reader, ctx)
		if err != nil {
			return nil, zero, toolError(err)
		}
		return nil, newOutput(response), nil
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
