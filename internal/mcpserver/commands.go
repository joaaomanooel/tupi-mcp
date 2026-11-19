package mcpserver

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/joaaomanooel/tupi-mcp/internal/config"
	"github.com/joaaomanooel/tupi-mcp/internal/octoprint"
)

const (
	ToolCreateFolder = "printer_create_folder"
	ToolCopyFile     = "printer_copy_file"
	ToolMoveFile     = "printer_move_file"
	ToolSelectFile   = "printer_select_file"
	ToolDeleteFile   = "printer_delete_file"
	ToolSliceFile    = "printer_slice_file"
	ToolStartJob     = "printer_start_job"
	ToolPauseJob     = "printer_pause_job"
	ToolResumeJob    = "printer_resume_job"
	ToolRestartJob   = "printer_restart_job"
	ToolCancelJob    = "printer_cancel_job"

	ToolConnect               = "printer_connect"
	ToolReconnect             = "printer_reconnect"
	ToolDisconnect            = "printer_disconnect"
	ToolJog                   = "printer_jog"
	ToolHome                  = "printer_home"
	ToolSetFeedrate           = "printer_set_feedrate"
	ToolSetToolTemperature    = "printer_set_tool_temperature"
	ToolSetToolOffset         = "printer_set_tool_offset"
	ToolSelectTool            = "printer_select_tool"
	ToolExtrude               = "printer_extrude"
	ToolSetBedTemperature     = "printer_set_bed_temperature"
	ToolSetBedOffset          = "printer_set_bed_offset"
	ToolSetChamberTemperature = "printer_set_chamber_temperature"
	ToolSetChamberOffset      = "printer_set_chamber_offset"
	ToolInitSD                = "printer_init_sd"
	ToolRefreshSD             = "printer_refresh_sd"
	ToolReleaseSD             = "printer_release_sd"
)

var configurableTools = map[string]struct{}{
	ToolCreateFolder: {},
	ToolCopyFile:     {},
	ToolMoveFile:     {},
	ToolSelectFile:   {},
	ToolDeleteFile:   {},
	ToolSliceFile:    {},
	ToolStartJob:     {},
	ToolPauseJob:     {},
	ToolResumeJob:    {},
	ToolRestartJob:   {},
	ToolCancelJob:    {},

	ToolConnect:               {},
	ToolReconnect:             {},
	ToolDisconnect:            {},
	ToolJog:                   {},
	ToolHome:                  {},
	ToolSetFeedrate:           {},
	ToolSetToolTemperature:    {},
	ToolSetToolOffset:         {},
	ToolSelectTool:            {},
	ToolExtrude:               {},
	ToolSetBedTemperature:     {},
	ToolSetBedOffset:          {},
	ToolSetChamberTemperature: {},
	ToolSetChamberOffset:      {},
	ToolInitSD:                {},
	ToolRefreshSD:             {},
	ToolReleaseSD:             {},
}

var fileTools = []string{
	ToolCreateFolder,
	ToolCopyFile,
	ToolMoveFile,
	ToolSelectFile,
	ToolDeleteFile,
	ToolSliceFile,
}

var jobTools = []string{
	ToolStartJob,
	ToolPauseJob,
	ToolResumeJob,
	ToolRestartJob,
	ToolCancelJob,
}

var physicalTools = []string{
	ToolConnect,
	ToolReconnect,
	ToolDisconnect,
	ToolJog,
	ToolHome,
	ToolSetFeedrate,
	ToolSetToolTemperature,
	ToolSetToolOffset,
	ToolSelectTool,
	ToolExtrude,
	ToolSetBedTemperature,
	ToolSetBedOffset,
	ToolSetChamberTemperature,
	ToolSetChamberOffset,
	ToolInitSD,
	ToolRefreshSD,
	ToolReleaseSD,
}

func isPhysicalTool(name string) bool {
	return slices.Contains(physicalTools, name)
}

func physicalToolAvailable(name string, limits config.PhysicalLimits) bool {
	switch name {
	case ToolConnect, ToolReconnect, ToolDisconnect, ToolSetFeedrate, ToolInitSD, ToolRefreshSD, ToolReleaseSD:
		return true
	case ToolJog, ToolHome:
		return limits.Configured.Axes && len(limits.Axes) > 0
	case ToolExtrude:
		return limits.Configured.Extrusion
	case ToolSetToolTemperature, ToolSetToolOffset, ToolSelectTool:
		return limits.Configured.Heaters && hasToolHeater(limits)
	case ToolSetBedTemperature, ToolSetBedOffset:
		return limits.Configured.Heaters && hasHeater(limits, "bed")
	case ToolSetChamberTemperature, ToolSetChamberOffset:
		return limits.Configured.Heaters && hasHeater(limits, "chamber")
	}
	return false
}

func availablePhysicalTools(enabled map[string]struct{}, limits config.PhysicalLimits) map[string]struct{} {
	available := make(map[string]struct{})
	for _, name := range physicalTools {
		if _, ok := enabled[name]; !ok {
			continue
		}
		if !physicalToolAvailable(name, limits) {
			continue
		}
		available[name] = struct{}{}
	}
	return available
}

func PhysicalToolsWithoutLimits(enabledTools []string, limits config.PhysicalLimits) []string {
	var missing []string
	for _, name := range enabledTools {
		if !isPhysicalTool(name) {
			continue
		}
		if physicalToolAvailable(name, limits) {
			continue
		}
		missing = append(missing, name)
	}
	return missing
}

func hasToolHeater(limits config.PhysicalLimits) bool {
	for name := range limits.Heaters {
		if _, ok := parseToolName(name); ok {
			return true
		}
	}
	return false
}

func hasHeater(limits config.PhysicalLimits, name string) bool {
	_, ok := limits.Heaters[name]
	return ok
}

type FileManager interface {
	CreateFolder(context.Context, string, string) (*octoprint.FolderResponse, error)
	CopyFile(context.Context, string, string) (*octoprint.FileCommandResponse, error)
	MoveFile(context.Context, string, string) (*octoprint.FileCommandResponse, error)
	SelectFile(context.Context, string) error
	DeleteFile(context.Context, string) error
	SliceFile(context.Context, string, string) (*octoprint.FileCommandResponse, error)
}

type JobController interface {
	StartJob(context.Context) error
	PauseJob(context.Context) error
	ResumeJob(context.Context) error
	RestartJob(context.Context) error
	CancelJob(context.Context) error
}

func anyEnabled(enabled map[string]struct{}, names ...string) bool {
	for _, name := range names {
		if _, ok := enabled[name]; ok {
			return true
		}
	}
	return false
}

func enabledToolSet(names []string) (map[string]struct{}, error) {
	enabled := make(map[string]struct{}, len(names))
	for _, name := range names {
		if _, known := configurableTools[name]; !known {
			return nil, fmt.Errorf("mcpserver: unknown tool %q", name)
		}
		if _, duplicate := enabled[name]; duplicate {
			return nil, fmt.Errorf("mcpserver: duplicate tool %q", name)
		}
		enabled[name] = struct{}{}
	}
	return enabled, nil
}

func addFileTools(server *mcp.Server, manager FileManager, enabled map[string]struct{}) {
	if _, ok := enabled[ToolCreateFolder]; ok {
		mcp.AddTool(server, &mcp.Tool{
			Name:         ToolCreateFolder,
			Description:  "Create a folder in OctoPrint local storage without selecting or printing anything. Requires the FILES_UPLOAD permission.",
			OutputSchema: folderOutputSchema,
		}, createFolderHandler(manager))
	}
	if _, ok := enabled[ToolCopyFile]; ok {
		mcp.AddTool(server, &mcp.Tool{
			Name:         ToolCopyFile,
			Description:  "Copy a file inside OctoPrint local storage without selecting or printing it. Requires the FILES_UPLOAD permission.",
			OutputSchema: fileCommandOutputSchema,
		}, copyFileHandler(manager))
	}
	if _, ok := enabled[ToolMoveFile]; ok {
		mcp.AddTool(server, &mcp.Tool{
			Name:         ToolMoveFile,
			Description:  "Move a file inside OctoPrint local storage without selecting or printing it. Requires the FILES_UPLOAD and FILES_DELETE permissions.",
			OutputSchema: fileCommandOutputSchema,
		}, moveFileHandler(manager))
	}
	if _, ok := enabled[ToolSelectFile]; ok {
		mcp.AddTool(server, &mcp.Tool{
			Name:         ToolSelectFile,
			Description:  "Select a local file in OctoPrint without starting a print. Requires the FILES_SELECT permission.",
			OutputSchema: commandOutputSchema,
		}, selectFileHandler(manager))
	}
	if _, ok := enabled[ToolDeleteFile]; ok {
		mcp.AddTool(server, &mcp.Tool{
			Name:         ToolDeleteFile,
			Description:  "Delete a local file or folder in OctoPrint without selecting or printing anything. Requires the FILES_DELETE permission.",
			OutputSchema: commandOutputSchema,
		}, deleteFileHandler(manager))
	}
	if _, ok := enabled[ToolSliceFile]; ok {
		mcp.AddTool(server, &mcp.Tool{
			Name:         ToolSliceFile,
			Description:  "Slice a local file into a destination inside OctoPrint local storage without selecting or printing the result. Requires the SLICE permission.",
			OutputSchema: fileCommandOutputSchema,
		}, sliceFileHandler(manager))
	}
}

func addJobTools(server *mcp.Server, controller JobController, enabled map[string]struct{}, coordinator *PhysicalCoordinator) {
	operationBusyDescription := ""
	if coordinator != nil {
		operationBusyDescription = " Returns physical: operation_busy during a physical operation; retry after it completes."
	}
	if _, ok := enabled[ToolStartJob]; ok {
		mcp.AddTool(server, &mcp.Tool{
			Name:         ToolStartJob,
			Description:  "Start the selected OctoPrint job. OctoPrint arbitrates the job state." + operationBusyDescription + " Requires the PRINT permission.",
			OutputSchema: commandOutputSchema,
		}, jobCommandHandler(controller.StartJob, coordinator))
	}
	if _, ok := enabled[ToolPauseJob]; ok {
		mcp.AddTool(server, &mcp.Tool{
			Name:         ToolPauseJob,
			Description:  "Pause the running OctoPrint job. OctoPrint arbitrates the job state. Requires the PRINT permission.",
			OutputSchema: commandOutputSchema,
		}, jobCommandHandler(controller.PauseJob, nil))
	}
	if _, ok := enabled[ToolResumeJob]; ok {
		mcp.AddTool(server, &mcp.Tool{
			Name:         ToolResumeJob,
			Description:  "Resume the paused OctoPrint job. OctoPrint arbitrates the job state." + operationBusyDescription + " Requires the PRINT permission.",
			OutputSchema: commandOutputSchema,
		}, jobCommandHandler(controller.ResumeJob, coordinator))
	}
	if _, ok := enabled[ToolRestartJob]; ok {
		mcp.AddTool(server, &mcp.Tool{
			Name:         ToolRestartJob,
			Description:  "Restart the current OctoPrint job. OctoPrint arbitrates the job state." + operationBusyDescription + " Requires the PRINT permission.",
			OutputSchema: commandOutputSchema,
		}, jobCommandHandler(controller.RestartJob, coordinator))
	}
	if _, ok := enabled[ToolCancelJob]; ok {
		mcp.AddTool(server, &mcp.Tool{
			Name:         ToolCancelJob,
			Description:  "Cancel the current OctoPrint job. OctoPrint arbitrates the job state. Requires the PRINT permission.",
			OutputSchema: commandOutputSchema,
		}, jobCommandHandler(controller.CancelJob, nil))
	}
}

func addPhysicalTools(server *mcp.Server, coordinator *PhysicalCoordinator, published map[string]struct{}) {
	if _, ok := published[ToolConnect]; ok {
		mcp.AddTool(server, &mcp.Tool{
			Name:         ToolConnect,
			Description:  "Connect OctoPrint to the printer using the configured connection preferences. OctoPrint acceptance does not confirm a physical connection. Requires the CONNECTION permission.",
			OutputSchema: physicalCommandOutputSchema,
		}, physicalCommandHandler(coordinator.Connect))
	}
	if _, ok := published[ToolReconnect]; ok {
		mcp.AddTool(server, &mcp.Tool{
			Name:         ToolReconnect,
			Description:  "Reconnect OctoPrint to the printer using a single connect request and the configured connection preferences. OctoPrint acceptance does not confirm a physical connection. Requires the CONNECTION permission.",
			OutputSchema: physicalCommandOutputSchema,
		}, physicalCommandHandler(coordinator.Reconnect))
	}
	if _, ok := published[ToolDisconnect]; ok {
		mcp.AddTool(server, &mcp.Tool{
			Name:         ToolDisconnect,
			Description:  "Disconnect OctoPrint from the printer. OctoPrint acceptance does not confirm a physical disconnection. Requires the CONNECTION permission.",
			OutputSchema: physicalCommandOutputSchema,
		}, physicalCommandHandler(coordinator.Disconnect))
	}
	if _, ok := published[ToolJog]; ok {
		mcp.AddTool(server, &mcp.Tool{
			Name:         ToolJog,
			Description:  "Jog the printhead by a bounded offset on one or more axes at a bounded speed. Requires the CONTROL permission.",
			OutputSchema: physicalCommandOutputSchema,
		}, jogHandler(coordinator))
	}
	if _, ok := published[ToolHome]; ok {
		mcp.AddTool(server, &mcp.Tool{
			Name:         ToolHome,
			Description:  "Home one or more printhead axes declared by the deployment limits. Requires the CONTROL permission.",
			OutputSchema: physicalCommandOutputSchema,
		}, homeHandler(coordinator))
	}
	if _, ok := published[ToolSetFeedrate]; ok {
		mcp.AddTool(server, &mcp.Tool{
			Name:         ToolSetFeedrate,
			Description:  "Set the printhead feedrate factor between 50 and 200 percent. Requires the CONTROL permission.",
			OutputSchema: physicalCommandOutputSchema,
		}, setFeedrateHandler(coordinator))
	}
	if _, ok := published[ToolSetToolTemperature]; ok {
		mcp.AddTool(server, &mcp.Tool{
			Name:         ToolSetToolTemperature,
			Description:  "Set a tool target temperature within the configured limits; zero turns the heater off. Requires the CONTROL permission.",
			OutputSchema: physicalCommandOutputSchema,
		}, setToolTemperatureHandler(coordinator))
	}
	if _, ok := published[ToolSetToolOffset]; ok {
		mcp.AddTool(server, &mcp.Tool{
			Name:         ToolSetToolOffset,
			Description:  "Set a tool offset within the configured limits. Requires the CONTROL permission.",
			OutputSchema: physicalCommandOutputSchema,
		}, setToolOffsetHandler(coordinator))
	}
	if _, ok := published[ToolSelectTool]; ok {
		mcp.AddTool(server, &mcp.Tool{
			Name:         ToolSelectTool,
			Description:  "Select a tool declared by the active printer profile and the configured limits. Requires the CONTROL permission.",
			OutputSchema: physicalCommandOutputSchema,
		}, selectToolHandler(coordinator))
	}
	if _, ok := published[ToolExtrude]; ok {
		mcp.AddTool(server, &mcp.Tool{
			Name:         ToolExtrude,
			Description:  "Extrude or retract filament with a bounded amount and speed while every reported hotend is above the configured minimum temperature. Requires the CONTROL permission.",
			OutputSchema: physicalCommandOutputSchema,
		}, extrudeHandler(coordinator))
	}
	if _, ok := published[ToolSetBedTemperature]; ok {
		mcp.AddTool(server, &mcp.Tool{
			Name:         ToolSetBedTemperature,
			Description:  "Set the bed target temperature within the configured limits; zero turns the heater off. Requires the CONTROL permission.",
			OutputSchema: physicalCommandOutputSchema,
		}, setBedTemperatureHandler(coordinator))
	}
	if _, ok := published[ToolSetBedOffset]; ok {
		mcp.AddTool(server, &mcp.Tool{
			Name:         ToolSetBedOffset,
			Description:  "Set the bed offset within the configured limits. Requires the CONTROL permission.",
			OutputSchema: physicalCommandOutputSchema,
		}, setBedOffsetHandler(coordinator))
	}
	if _, ok := published[ToolSetChamberTemperature]; ok {
		mcp.AddTool(server, &mcp.Tool{
			Name:         ToolSetChamberTemperature,
			Description:  "Set the chamber target temperature within the configured limits; zero turns the heater off. Requires the CONTROL permission.",
			OutputSchema: physicalCommandOutputSchema,
		}, setChamberTemperatureHandler(coordinator))
	}
	if _, ok := published[ToolSetChamberOffset]; ok {
		mcp.AddTool(server, &mcp.Tool{
			Name:         ToolSetChamberOffset,
			Description:  "Set the chamber offset within the configured limits. Requires the CONTROL permission.",
			OutputSchema: physicalCommandOutputSchema,
		}, setChamberOffsetHandler(coordinator))
	}
	if _, ok := published[ToolInitSD]; ok {
		mcp.AddTool(server, &mcp.Tool{
			Name:         ToolInitSD,
			Description:  "Initialize the printer SD card. Requires the CONTROL permission.",
			OutputSchema: physicalCommandOutputSchema,
		}, physicalCommandHandler(coordinator.InitSD))
	}
	if _, ok := published[ToolRefreshSD]; ok {
		mcp.AddTool(server, &mcp.Tool{
			Name:         ToolRefreshSD,
			Description:  "Refresh the file list of the printer SD card. Requires the CONTROL permission.",
			OutputSchema: physicalCommandOutputSchema,
		}, physicalCommandHandler(coordinator.RefreshSD))
	}
	if _, ok := published[ToolReleaseSD]; ok {
		mcp.AddTool(server, &mcp.Tool{
			Name:         ToolReleaseSD,
			Description:  "Release the printer SD card so the printer can access it. Requires the CONTROL permission.",
			OutputSchema: physicalCommandOutputSchema,
		}, physicalCommandHandler(coordinator.ReleaseSD))
	}
}

type createFolderInput struct {
	Name   string `json:"name" jsonschema:"Folder name without path separators"`
	Parent string `json:"parent,omitempty" jsonschema:"Optional parent folder path inside local storage"`
}

type transferFileInput struct {
	Source      string `json:"source" jsonschema:"Existing local file path"`
	Destination string `json:"destination" jsonschema:"Destination path inside local storage"`
}

type filePathInput struct {
	Path string `json:"path" jsonschema:"Local file or folder path"`
}

type sliceFileInput struct {
	Path        string `json:"path" jsonschema:"Local file path to slice"`
	Destination string `json:"destination" jsonschema:"Destination path inside local storage"`
}

func createFolderHandler(manager FileManager) mcp.ToolHandlerFor[createFolderInput, folderOutput] {
	return func(ctx context.Context, _ *mcp.CallToolRequest, input createFolderInput) (*mcp.CallToolResult, folderOutput, error) {
		response, err := manager.CreateFolder(ctx, input.Name, input.Parent)
		if err != nil {
			return nil, folderOutput{}, toolError(err)
		}
		return nil, newFolderOutput(response), nil
	}
}

func copyFileHandler(manager FileManager) mcp.ToolHandlerFor[transferFileInput, fileCommandOutput] {
	return func(ctx context.Context, _ *mcp.CallToolRequest, input transferFileInput) (*mcp.CallToolResult, fileCommandOutput, error) {
		response, err := manager.CopyFile(ctx, input.Source, input.Destination)
		if err != nil {
			return nil, fileCommandOutput{}, toolError(err)
		}
		return nil, newFileCommandOutput(response), nil
	}
}

func moveFileHandler(manager FileManager) mcp.ToolHandlerFor[transferFileInput, fileCommandOutput] {
	return func(ctx context.Context, _ *mcp.CallToolRequest, input transferFileInput) (*mcp.CallToolResult, fileCommandOutput, error) {
		response, err := manager.MoveFile(ctx, input.Source, input.Destination)
		if err != nil {
			return nil, fileCommandOutput{}, toolError(err)
		}
		return nil, newFileCommandOutput(response), nil
	}
}

func selectFileHandler(manager FileManager) mcp.ToolHandlerFor[filePathInput, commandOutput] {
	return func(ctx context.Context, _ *mcp.CallToolRequest, input filePathInput) (*mcp.CallToolResult, commandOutput, error) {
		if err := manager.SelectFile(ctx, input.Path); err != nil {
			return nil, commandOutput{}, toolError(err)
		}
		return nil, newCommandOutput(), nil
	}
}

func deleteFileHandler(manager FileManager) mcp.ToolHandlerFor[filePathInput, commandOutput] {
	return func(ctx context.Context, _ *mcp.CallToolRequest, input filePathInput) (*mcp.CallToolResult, commandOutput, error) {
		if err := manager.DeleteFile(ctx, input.Path); err != nil {
			return nil, commandOutput{}, toolError(err)
		}
		return nil, newCommandOutput(), nil
	}
}

func sliceFileHandler(manager FileManager) mcp.ToolHandlerFor[sliceFileInput, fileCommandOutput] {
	return func(ctx context.Context, _ *mcp.CallToolRequest, input sliceFileInput) (*mcp.CallToolResult, fileCommandOutput, error) {
		response, err := manager.SliceFile(ctx, input.Path, input.Destination)
		if err != nil {
			return nil, fileCommandOutput{}, toolError(err)
		}
		return nil, newFileCommandOutput(response), nil
	}
}

func jobCommandHandler(run func(context.Context) error, coordinator *PhysicalCoordinator) mcp.ToolHandlerFor[struct{}, commandOutput] {
	return func(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, commandOutput, error) {
		if err := ctx.Err(); err != nil {
			return nil, commandOutput{}, toolError(err)
		}
		if coordinator != nil {
			release, reserved := coordinator.tryReserve()
			if !reserved {
				return nil, commandOutput{}, physicalError(PhysicalErrorOperationBusy)
			}
			defer release()
		}
		if err := ctx.Err(); err != nil {
			return nil, commandOutput{}, toolError(err)
		}
		if err := run(ctx); err != nil {
			return nil, commandOutput{}, toolError(err)
		}
		return nil, newCommandOutput(), nil
	}
}

type jogInput struct {
	X     *float64 `json:"x,omitempty" jsonschema:"Offset in millimeters on the x axis"`
	Y     *float64 `json:"y,omitempty" jsonschema:"Offset in millimeters on the y axis"`
	Z     *float64 `json:"z,omitempty" jsonschema:"Offset in millimeters on the z axis"`
	Speed float64  `json:"speed" jsonschema:"Movement speed in millimeters per minute"`
}

type homeInput struct {
	Axes []string `json:"axes" jsonschema:"Axes to home, one or more of x, y and z"`
}

type feedrateInput struct {
	Factor int `json:"factor" jsonschema:"Feedrate percentage factor between 50 and 200"`
}

type toolTargetInput struct {
	Tool   string  `json:"tool" jsonschema:"Tool name such as tool0"`
	Target float64 `json:"target" jsonschema:"Target temperature in Celsius; zero turns the heater off"`
}

type toolOffsetInput struct {
	Tool   string  `json:"tool" jsonschema:"Tool name such as tool0"`
	Offset float64 `json:"offset" jsonschema:"Tool offset in millimeters"`
}

type toolSelectionInput struct {
	Tool string `json:"tool" jsonschema:"Tool name such as tool0"`
}

type extrusionInput struct {
	Amount float64 `json:"amount" jsonschema:"Extrusion amount in millimeters; negative values retract"`
	Speed  float64 `json:"speed" jsonschema:"Extrusion speed in millimeters per minute"`
}

type heaterTargetInput struct {
	Target float64 `json:"target" jsonschema:"Target temperature in Celsius; zero turns the heater off"`
}

type heaterOffsetInput struct {
	Offset float64 `json:"offset" jsonschema:"Heater offset in millimeters"`
}

func physicalCommandHandler(action func(context.Context) (PhysicalOutcome, error)) mcp.ToolHandlerFor[struct{}, physicalCommandOutput] {
	return func(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, physicalCommandOutput, error) {
		return physicalResult(action(ctx))
	}
}

func jogHandler(coordinator *PhysicalCoordinator) mcp.ToolHandlerFor[jogInput, physicalCommandOutput] {
	return func(ctx context.Context, _ *mcp.CallToolRequest, input jogInput) (*mcp.CallToolResult, physicalCommandOutput, error) {
		return physicalResult(coordinator.Jog(ctx, octoprint.PrintheadJog{
			X:     input.X,
			Y:     input.Y,
			Z:     input.Z,
			Speed: input.Speed,
		}))
	}
}

func homeHandler(coordinator *PhysicalCoordinator) mcp.ToolHandlerFor[homeInput, physicalCommandOutput] {
	return func(ctx context.Context, _ *mcp.CallToolRequest, input homeInput) (*mcp.CallToolResult, physicalCommandOutput, error) {
		axes, err := parseHomeAxes(input.Axes)
		if err != nil {
			return physicalResult("", err)
		}
		return physicalResult(coordinator.Home(ctx, axes))
	}
}

func setFeedrateHandler(coordinator *PhysicalCoordinator) mcp.ToolHandlerFor[feedrateInput, physicalCommandOutput] {
	return func(ctx context.Context, _ *mcp.CallToolRequest, input feedrateInput) (*mcp.CallToolResult, physicalCommandOutput, error) {
		return physicalResult(coordinator.SetFeedrate(ctx, input.Factor))
	}
}

func setToolTemperatureHandler(coordinator *PhysicalCoordinator) mcp.ToolHandlerFor[toolTargetInput, physicalCommandOutput] {
	return func(ctx context.Context, _ *mcp.CallToolRequest, input toolTargetInput) (*mcp.CallToolResult, physicalCommandOutput, error) {
		return physicalResult(coordinator.SetToolTarget(ctx, input.Tool, input.Target))
	}
}

func setToolOffsetHandler(coordinator *PhysicalCoordinator) mcp.ToolHandlerFor[toolOffsetInput, physicalCommandOutput] {
	return func(ctx context.Context, _ *mcp.CallToolRequest, input toolOffsetInput) (*mcp.CallToolResult, physicalCommandOutput, error) {
		return physicalResult(coordinator.SetToolOffset(ctx, input.Tool, input.Offset))
	}
}

func selectToolHandler(coordinator *PhysicalCoordinator) mcp.ToolHandlerFor[toolSelectionInput, physicalCommandOutput] {
	return func(ctx context.Context, _ *mcp.CallToolRequest, input toolSelectionInput) (*mcp.CallToolResult, physicalCommandOutput, error) {
		return physicalResult(coordinator.SelectTool(ctx, input.Tool))
	}
}

func extrudeHandler(coordinator *PhysicalCoordinator) mcp.ToolHandlerFor[extrusionInput, physicalCommandOutput] {
	return func(ctx context.Context, _ *mcp.CallToolRequest, input extrusionInput) (*mcp.CallToolResult, physicalCommandOutput, error) {
		return physicalResult(coordinator.Extrude(ctx, input.Amount, input.Speed))
	}
}

func setBedTemperatureHandler(coordinator *PhysicalCoordinator) mcp.ToolHandlerFor[heaterTargetInput, physicalCommandOutput] {
	return func(ctx context.Context, _ *mcp.CallToolRequest, input heaterTargetInput) (*mcp.CallToolResult, physicalCommandOutput, error) {
		return physicalResult(coordinator.SetBedTarget(ctx, input.Target))
	}
}

func setBedOffsetHandler(coordinator *PhysicalCoordinator) mcp.ToolHandlerFor[heaterOffsetInput, physicalCommandOutput] {
	return func(ctx context.Context, _ *mcp.CallToolRequest, input heaterOffsetInput) (*mcp.CallToolResult, physicalCommandOutput, error) {
		return physicalResult(coordinator.SetBedOffset(ctx, input.Offset))
	}
}

func setChamberTemperatureHandler(coordinator *PhysicalCoordinator) mcp.ToolHandlerFor[heaterTargetInput, physicalCommandOutput] {
	return func(ctx context.Context, _ *mcp.CallToolRequest, input heaterTargetInput) (*mcp.CallToolResult, physicalCommandOutput, error) {
		return physicalResult(coordinator.SetChamberTarget(ctx, input.Target))
	}
}

func setChamberOffsetHandler(coordinator *PhysicalCoordinator) mcp.ToolHandlerFor[heaterOffsetInput, physicalCommandOutput] {
	return func(ctx context.Context, _ *mcp.CallToolRequest, input heaterOffsetInput) (*mcp.CallToolResult, physicalCommandOutput, error) {
		return physicalResult(coordinator.SetChamberOffset(ctx, input.Offset))
	}
}

func parseHomeAxes(values []string) ([]octoprint.PrintheadAxis, error) {
	axes := make([]octoprint.PrintheadAxis, 0, len(values))
	for _, value := range values {
		axis := octoprint.PrintheadAxis(value)
		if axis != octoprint.PrintheadAxisX && axis != octoprint.PrintheadAxisY && axis != octoprint.PrintheadAxisZ {
			return nil, physicalError(PhysicalErrorInvalidArgument)
		}
		axes = append(axes, axis)
	}
	return axes, nil
}

func physicalResult(outcome PhysicalOutcome, err error) (*mcp.CallToolResult, physicalCommandOutput, error) {
	if err != nil {
		if outcome == PhysicalUnknown {
			return &mcp.CallToolResult{
				IsError: true,
				Content: []mcp.Content{&mcp.TextContent{Text: physicalError(PhysicalErrorUnknownResult).Error()}},
			}, physicalCommandOutput{Outcome: PhysicalUnknown}, nil
		}
		return nil, physicalCommandOutput{}, physicalToolError(err)
	}
	return nil, physicalCommandOutput{Outcome: outcome}, nil
}

func physicalToolError(err error) error {
	var operationError *PhysicalError
	if errors.As(err, &operationError) {
		return errors.New("physical: " + string(operationError.Kind))
	}
	return errors.New("physical: " + string(PhysicalErrorUnavailable))
}
