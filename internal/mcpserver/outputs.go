package mcpserver

import (
	"bytes"
	"encoding/json"

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
	Free  *int64       `json:"free,omitempty"`
	Total *int64       `json:"total,omitempty"`
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

type versionOutput struct {
	API    *string `json:"api,omitempty"`
	Server *string `json:"server,omitempty"`
	Text   *string `json:"text,omitempty"`
}

type serverOutput struct {
	Version  *string         `json:"version,omitempty"`
	Safemode json.RawMessage `json:"safemode,omitempty"`
}

type toolTemperaturesOutput struct {
	Tools map[string]temperatureOutput `json:"tools"`
}

type bedTemperatureOutput struct {
	Bed *temperatureOutput `json:"bed,omitempty"`
}

type chamberTemperatureOutput struct {
	Chamber *temperatureOutput `json:"chamber,omitempty"`
}

type sdStateOutput struct {
	Ready *bool `json:"ready,omitempty"`
}

type printerErrorOutput struct {
	Consequence *string `json:"consequence,omitempty"`
	Message     *string `json:"error,omitempty"`
	FAQ         *string `json:"faq,omitempty"`
	Reason      *string `json:"reason,omitempty"`
}

type profilesOutput struct {
	Profiles map[string]printerProfileOutput `json:"profiles"`
}

type printerProfileOutput struct {
	ID            *string                `json:"id,omitempty"`
	Name          *string                `json:"name,omitempty"`
	Color         *string                `json:"color,omitempty"`
	Model         *string                `json:"model,omitempty"`
	Default       *bool                  `json:"default,omitempty"`
	Current       *bool                  `json:"current,omitempty"`
	HeatedBed     *bool                  `json:"heatedBed,omitempty"`
	HeatedChamber *bool                  `json:"heatedChamber,omitempty"`
	Volume        *printerVolumeOutput   `json:"volume,omitempty"`
	Extruder      *printerExtruderOutput `json:"extruder,omitempty"`
}

type printerVolumeOutput struct {
	Width      *float64 `json:"width,omitempty"`
	Depth      *float64 `json:"depth,omitempty"`
	Height     *float64 `json:"height,omitempty"`
	FormFactor *string  `json:"formFactor,omitempty"`
	Origin     *string  `json:"origin,omitempty"`
}

type printerExtruderOutput struct {
	Count          *int     `json:"count,omitempty"`
	NozzleDiameter *float64 `json:"nozzleDiameter,omitempty"`
	SharedNozzle   *bool    `json:"sharedNozzle,omitempty"`
}

type slicingOutput struct {
	Profiles map[string][]string `json:"profiles"`
}

type timelapseOutput struct {
	Config     timelapseConfigOutput  `json:"config"`
	Files      []timelapseFileOutput  `json:"files"`
	Unrendered []timelapseEntryOutput `json:"unrendered"`
}

type timelapseConfigOutput struct {
	Type             *string  `json:"type,omitempty"`
	PostRoll         *int64   `json:"postRoll,omitempty"`
	FPS              *int64   `json:"fps,omitempty"`
	RetractionZHop   *float64 `json:"retractionZHop,omitempty"`
	MinDelay         *int64   `json:"minDelay,omitempty"`
	Interval         *int64   `json:"interval,omitempty"`
	RenderAfterPrint *string  `json:"renderAfterPrint,omitempty"`
}

type timelapseEntryOutput struct {
	Name       *string `json:"name,omitempty"`
	Size       *string `json:"size,omitempty"`
	Bytes      *int64  `json:"bytes,omitempty"`
	Date       *string `json:"date,omitempty"`
	Recording  *bool   `json:"recording,omitempty"`
	Rendering  *bool   `json:"rendering,omitempty"`
	Processing *bool   `json:"processing,omitempty"`
}

type timelapseFileOutput struct {
	Name  *string `json:"name,omitempty"`
	Size  *string `json:"size,omitempty"`
	Bytes *int64  `json:"bytes,omitempty"`
	Date  *string `json:"date,omitempty"`
}

type settingsOutput struct {
	Feature *settingsFeatureOutput `json:"feature,omitempty"`
}

type settingsFeatureOutput struct {
	TemperatureGraph            *bool `json:"temperatureGraph,omitempty"`
	SDSupport                   *bool `json:"sdSupport,omitempty"`
	ModelSizeDetection          *bool `json:"modelSizeDetection,omitempty"`
	PrintStartConfirmation      *bool `json:"printStartConfirmation,omitempty"`
	PrintCancelConfirmation     *bool `json:"printCancelConfirmation,omitempty"`
	UploadOverwriteConfirmation *bool `json:"uploadOverwriteConfirmation,omitempty"`
	FileDeleteConfirmation      *bool `json:"fileDeleteConfirmation,omitempty"`
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
	if response == nil {
		return filesOutput{Files: []fileOutput{}}
	}
	return filesOutput{Files: newFileOutputs(response.Files), Free: response.Free, Total: response.Total}
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

func newVersionOutput(response *octoprint.VersionResponse) versionOutput {
	if response == nil {
		return versionOutput{}
	}
	return versionOutput{API: response.API, Server: response.Server, Text: response.Text}
}

func newServerOutput(response *octoprint.ServerResponse) serverOutput {
	if response == nil {
		return serverOutput{}
	}
	safemode := response.Safemode
	if bytes.Equal(bytes.TrimSpace(safemode), []byte("null")) {
		safemode = nil
	}
	return serverOutput{Version: response.Version, Safemode: safemode}
}

func newToolTemperaturesOutput(response *octoprint.ToolTemperaturesResponse) toolTemperaturesOutput {
	tools := make(map[string]temperatureOutput)
	if response == nil {
		return toolTemperaturesOutput{Tools: tools}
	}
	for name, temperature := range response.Tools {
		tools[name] = newTemperatureOutput(temperature)
	}
	return toolTemperaturesOutput{Tools: tools}
}

func newBedTemperatureOutput(response *octoprint.BedTemperatureResponse) bedTemperatureOutput {
	if response == nil || response.Bed == nil {
		return bedTemperatureOutput{}
	}
	temperature := newTemperatureOutput(response.Bed)
	return bedTemperatureOutput{Bed: &temperature}
}

func newChamberTemperatureOutput(response *octoprint.ChamberTemperatureResponse) chamberTemperatureOutput {
	if response == nil || response.Chamber == nil {
		return chamberTemperatureOutput{}
	}
	temperature := newTemperatureOutput(response.Chamber)
	return chamberTemperatureOutput{Chamber: &temperature}
}

func newSDStateOutput(response *octoprint.SDStateResponse) sdStateOutput {
	if response == nil {
		return sdStateOutput{}
	}
	return sdStateOutput{Ready: response.Ready}
}

func newPrinterErrorOutput(response *octoprint.PrinterErrorResponse) printerErrorOutput {
	if response == nil {
		return printerErrorOutput{}
	}
	return printerErrorOutput{
		Consequence: response.Consequence,
		Message:     response.Message,
		FAQ:         response.FAQ,
		Reason:      response.Reason,
	}
}

func newProfilesOutput(response *octoprint.ProfilesResponse) profilesOutput {
	profiles := make(map[string]printerProfileOutput)
	if response == nil {
		return profilesOutput{Profiles: profiles}
	}
	for id, profile := range response.Profiles {
		profiles[id] = newPrinterProfileOutput(profile)
	}
	return profilesOutput{Profiles: profiles}
}

func newPrinterProfileOutput(profile octoprint.PrinterProfile) printerProfileOutput {
	return printerProfileOutput{
		ID:            profile.ID,
		Name:          profile.Name,
		Color:         profile.Color,
		Model:         profile.Model,
		Default:       profile.Default,
		Current:       profile.Current,
		HeatedBed:     profile.HeatedBed,
		HeatedChamber: profile.HeatedChamber,
		Volume:        newPrinterVolumeOutput(profile.Volume),
		Extruder:      newPrinterExtruderOutput(profile.Extruder),
	}
}

func newPrinterVolumeOutput(volume *octoprint.PrinterVolume) *printerVolumeOutput {
	if volume == nil {
		return nil
	}
	return &printerVolumeOutput{
		Width:      volume.Width,
		Depth:      volume.Depth,
		Height:     volume.Height,
		FormFactor: volume.FormFactor,
		Origin:     volume.Origin,
	}
}

func newPrinterExtruderOutput(extruder *octoprint.PrinterExtruder) *printerExtruderOutput {
	if extruder == nil {
		return nil
	}
	return &printerExtruderOutput{
		Count:          extruder.Count,
		NozzleDiameter: extruder.NozzleDiameter,
		SharedNozzle:   extruder.SharedNozzle,
	}
}

func newSlicingOutput(response *octoprint.SlicingResponse) slicingOutput {
	profiles := make(map[string][]string)
	if response == nil {
		return slicingOutput{Profiles: profiles}
	}
	for slicer, names := range response.Profiles {
		profiles[slicer] = names
	}
	return slicingOutput{Profiles: profiles}
}

func newTimelapseOutput(response *octoprint.TimelapseResponse) timelapseOutput {
	files := make([]timelapseFileOutput, 0)
	unrendered := make([]timelapseEntryOutput, 0)
	if response == nil {
		return timelapseOutput{Config: timelapseConfigOutput{}, Files: files, Unrendered: unrendered}
	}
	for _, timelapse := range response.Unrendered {
		unrendered = append(unrendered, timelapseEntryOutput{
			Name:       timelapse.Name,
			Size:       timelapse.Size,
			Bytes:      timelapse.Bytes,
			Date:       timelapse.Date,
			Recording:  timelapse.Recording,
			Rendering:  timelapse.Rendering,
			Processing: timelapse.Processing,
		})
	}
	for _, file := range response.Files {
		files = append(files, timelapseFileOutput{
			Name:  file.Name,
			Size:  file.Size,
			Bytes: file.Bytes,
			Date:  file.Date,
		})
	}
	return timelapseOutput{
		Config: timelapseConfigOutput{
			Type:             response.Config.Type,
			PostRoll:         response.Config.PostRoll,
			FPS:              response.Config.FPS,
			RetractionZHop:   response.Config.RetractionZHop,
			MinDelay:         response.Config.MinDelay,
			Interval:         response.Config.Interval,
			RenderAfterPrint: response.Config.RenderAfterPrint,
		},
		Files:      files,
		Unrendered: unrendered,
	}
}

func newSettingsOutput(response *octoprint.SettingsResponse) settingsOutput {
	if response == nil || response.Feature == nil {
		return settingsOutput{}
	}
	feature := response.Feature
	return settingsOutput{Feature: &settingsFeatureOutput{
		TemperatureGraph:            feature.TemperatureGraph,
		SDSupport:                   feature.SDSupport,
		ModelSizeDetection:          feature.ModelSizeDetection,
		PrintStartConfirmation:      feature.PrintStartConfirmation,
		PrintCancelConfirmation:     feature.PrintCancelConfirmation,
		UploadOverwriteConfirmation: feature.UploadOverwriteConfirmation,
		FileDeleteConfirmation:      feature.FileDeleteConfirmation,
	}}
}
