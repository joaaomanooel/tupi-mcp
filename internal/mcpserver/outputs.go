package mcpserver

import (
	"github.com/joaaomanooel/tupi-mcp/internal/octoprint"
)

type statusOutput struct {
	Connection *connectionOutput `json:"connection,omitempty"`
	Printer    *printerOutput    `json:"printer,omitempty"`
}

type connectionOutput struct {
	State          *string `json:"state,omitempty"`
	Port           *string `json:"port,omitempty"`
	Baudrate       *int    `json:"baudrate,omitempty"`
	PrinterProfile *string `json:"printerProfile,omitempty"`
}

type printerOutput struct {
	State *printerStateOutput `json:"state,omitempty"`
}

type printerStateOutput struct {
	Text  *string             `json:"text,omitempty"`
	Flags *printerFlagsOutput `json:"flags,omitempty"`
}

type printerFlagsOutput struct {
	Operational   *bool `json:"operational,omitempty"`
	Printing      *bool `json:"printing,omitempty"`
	Paused        *bool `json:"paused,omitempty"`
	Ready         *bool `json:"ready,omitempty"`
	Error         *bool `json:"error,omitempty"`
	ClosedOrError *bool `json:"closedOrError,omitempty"`
}

type temperaturesOutput struct {
	Temperatures map[string]temperatureOutput `json:"temperatures,omitempty"`
}

type temperatureOutput struct {
	Actual *float64 `json:"actual,omitempty"`
	Target *float64 `json:"target,omitempty"`
	Offset *float64 `json:"offset,omitempty"`
}

type jobOutput struct {
	Job      *jobDetailsOutput  `json:"job,omitempty"`
	Progress *jobProgressOutput `json:"progress,omitempty"`
	State    *string            `json:"state,omitempty"`
}

type jobDetailsOutput struct {
	File               *fileOutput `json:"file,omitempty"`
	EstimatedPrintTime *float64    `json:"estimatedPrintTime,omitempty"`
	AveragePrintTime   *float64    `json:"averagePrintTime,omitempty"`
	LastPrintTime      *float64    `json:"lastPrintTime,omitempty"`
}

type jobProgressOutput struct {
	Completion          *float64 `json:"completion,omitempty"`
	FilePosition        *int64   `json:"filePosition,omitempty"`
	PrintTime           *float64 `json:"printTime,omitempty"`
	PrintTimeLeft       *float64 `json:"printTimeLeft,omitempty"`
	PrintTimeLeftOrigin *string  `json:"printTimeLeftOrigin,omitempty"`
}

type filesOutput struct {
	Files []fileOutput `json:"files"`
}

type uploadOutput struct {
	File            *fileOutput `json:"file,omitempty"`
	Done            *bool       `json:"done,omitempty"`
	EffectiveSelect *bool       `json:"effectiveSelect,omitempty"`
	EffectivePrint  *bool       `json:"effectivePrint,omitempty"`
}

type fileOutput struct {
	Name     *string      `json:"name,omitempty"`
	Display  *string      `json:"display,omitempty"`
	Path     *string      `json:"path,omitempty"`
	Type     *string      `json:"type,omitempty"`
	Origin   *string      `json:"origin,omitempty"`
	Size     *int64       `json:"size,omitempty"`
	Date     *int64       `json:"date,omitempty"`
	Children []fileOutput `json:"children,omitempty"`
}

func newStatusOutput(connection *octoprint.ConnectionResponse, printer *octoprint.PrinterResponse) statusOutput {
	return statusOutput{
		Connection: newConnectionOutput(connection),
		Printer:    newPrinterOutput(printer),
	}
}

func newConnectionOutput(response *octoprint.ConnectionResponse) *connectionOutput {
	if response == nil || response.Current == nil {
		return nil
	}
	return &connectionOutput{
		State:          response.Current.State,
		Port:           response.Current.Port,
		Baudrate:       response.Current.Baudrate,
		PrinterProfile: response.Current.PrinterProfile,
	}
}

func newPrinterOutput(response *octoprint.PrinterResponse) *printerOutput {
	if response == nil || response.State == nil {
		return nil
	}
	return &printerOutput{State: newPrinterStateOutput(response.State)}
}

func newPrinterStateOutput(state *octoprint.PrinterState) *printerStateOutput {
	if state == nil {
		return nil
	}
	return &printerStateOutput{Text: state.Text, Flags: newPrinterFlagsOutput(state.Flags)}
}

func newPrinterFlagsOutput(flags *octoprint.PrinterFlags) *printerFlagsOutput {
	if flags == nil {
		return nil
	}
	return &printerFlagsOutput{
		Operational:   flags.Operational,
		Printing:      flags.Printing,
		Paused:        flags.Paused,
		Ready:         flags.Ready,
		Error:         flags.Error,
		ClosedOrError: flags.ClosedOrError,
	}
}

func newTemperaturesOutput(response *octoprint.PrinterResponse) temperaturesOutput {
	if response == nil || response.Temperature == nil {
		return temperaturesOutput{}
	}
	temperatures := make(map[string]temperatureOutput, len(response.Temperature))
	for name, temperature := range response.Temperature {
		temperatures[name] = newTemperatureOutput(temperature)
	}
	return temperaturesOutput{Temperatures: temperatures}
}

func newTemperatureOutput(temperature *octoprint.Temperature) temperatureOutput {
	if temperature == nil {
		return temperatureOutput{}
	}
	return temperatureOutput{Actual: temperature.Actual, Target: temperature.Target, Offset: temperature.Offset}
}

func newJobOutput(response *octoprint.JobResponse) jobOutput {
	if response == nil {
		return jobOutput{}
	}
	return jobOutput{
		Job:      newJobDetailsOutput(response.Job),
		Progress: newJobProgressOutput(response.Progress),
		State:    response.State,
	}
}

func newJobDetailsOutput(details *octoprint.JobDetails) *jobDetailsOutput {
	if details == nil {
		return nil
	}
	return &jobDetailsOutput{
		File:               newFileOutput(details.File),
		EstimatedPrintTime: details.EstimatedPrintTime,
		AveragePrintTime:   details.AveragePrintTime,
		LastPrintTime:      details.LastPrintTime,
	}
}

func newJobProgressOutput(progress *octoprint.JobProgress) *jobProgressOutput {
	if progress == nil {
		return nil
	}
	return &jobProgressOutput{
		Completion:          progress.Completion,
		FilePosition:        progress.FilePosition,
		PrintTime:           progress.PrintTime,
		PrintTimeLeft:       progress.PrintTimeLeft,
		PrintTimeLeftOrigin: progress.PrintTimeLeftOrigin,
	}
}

func newFilesOutput(response *octoprint.FilesResponse) filesOutput {
	files := make([]fileOutput, 0)
	if response != nil {
		files = newFileOutputs(response.Files)
	}
	return filesOutput{Files: files}
}

func newUploadOutput(response *octoprint.UploadResponse) uploadOutput {
	if response == nil {
		return uploadOutput{}
	}
	file := response.File
	if file == nil {
		file = response.Files["local"]
	}
	return uploadOutput{
		File:            newFileOutput(file),
		Done:            response.Done,
		EffectiveSelect: response.EffectiveSelect,
		EffectivePrint:  response.EffectivePrint,
	}
}

func newFileOutputs(files []octoprint.FileInfo) []fileOutput {
	outputs := make([]fileOutput, 0, len(files))
	for _, file := range files {
		outputs = append(outputs, newFileOutputValue(file))
	}
	return outputs
}

func newFileOutput(file *octoprint.FileInfo) *fileOutput {
	if file == nil {
		return nil
	}
	converted := newFileOutputValue(*file)
	return &converted
}

func newFileOutputValue(file octoprint.FileInfo) fileOutput {
	return fileOutput{
		Name:     file.Name,
		Display:  file.Display,
		Path:     file.Path,
		Type:     file.Type,
		Origin:   file.Origin,
		Size:     file.Size,
		Date:     file.Date,
		Children: newFileOutputs(file.Children),
	}
}
