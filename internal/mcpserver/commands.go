package mcpserver

import (
	"context"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"

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

func addJobTools(server *mcp.Server, controller JobController, enabled map[string]struct{}) {
	if _, ok := enabled[ToolStartJob]; ok {
		mcp.AddTool(server, &mcp.Tool{
			Name:         ToolStartJob,
			Description:  "Start the selected OctoPrint job. OctoPrint arbitrates the job state. Requires the PRINT permission.",
			OutputSchema: commandOutputSchema,
		}, jobCommandHandler(controller.StartJob))
	}
	if _, ok := enabled[ToolPauseJob]; ok {
		mcp.AddTool(server, &mcp.Tool{
			Name:         ToolPauseJob,
			Description:  "Pause the running OctoPrint job. OctoPrint arbitrates the job state. Requires the PRINT permission.",
			OutputSchema: commandOutputSchema,
		}, jobCommandHandler(controller.PauseJob))
	}
	if _, ok := enabled[ToolResumeJob]; ok {
		mcp.AddTool(server, &mcp.Tool{
			Name:         ToolResumeJob,
			Description:  "Resume the paused OctoPrint job. OctoPrint arbitrates the job state. Requires the PRINT permission.",
			OutputSchema: commandOutputSchema,
		}, jobCommandHandler(controller.ResumeJob))
	}
	if _, ok := enabled[ToolRestartJob]; ok {
		mcp.AddTool(server, &mcp.Tool{
			Name:         ToolRestartJob,
			Description:  "Restart the current OctoPrint job. OctoPrint arbitrates the job state. Requires the PRINT permission.",
			OutputSchema: commandOutputSchema,
		}, jobCommandHandler(controller.RestartJob))
	}
	if _, ok := enabled[ToolCancelJob]; ok {
		mcp.AddTool(server, &mcp.Tool{
			Name:         ToolCancelJob,
			Description:  "Cancel the current OctoPrint job. OctoPrint arbitrates the job state. Requires the PRINT permission.",
			OutputSchema: commandOutputSchema,
		}, jobCommandHandler(controller.CancelJob))
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

func jobCommandHandler(run func(context.Context) error) mcp.ToolHandlerFor[struct{}, commandOutput] {
	return func(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, commandOutput, error) {
		if err := run(ctx); err != nil {
			return nil, commandOutput{}, toolError(err)
		}
		return nil, newCommandOutput(), nil
	}
}
