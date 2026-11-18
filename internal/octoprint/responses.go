package octoprint

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"sort"
	"strings"
)

type ConnectionResponse struct {
	Current *ConnectionState `json:"current"`
}

type ConnectionState struct {
	State          *string `json:"state"`
	Port           *string `json:"port"`
	Baudrate       *int    `json:"baudrate"`
	PrinterProfile *string `json:"printerProfile"`
}

type PrinterResponse struct {
	State       *PrinterState `json:"state"`
	Temperature Temperatures  `json:"temperature"`
}

type PrinterState struct {
	Text  *string       `json:"text"`
	Flags *PrinterFlags `json:"flags"`
}

type PrinterFlags struct {
	Operational   *bool `json:"operational"`
	Printing      *bool `json:"printing"`
	Paused        *bool `json:"paused"`
	Ready         *bool `json:"ready"`
	Error         *bool `json:"error"`
	ClosedOrError *bool `json:"closedOrError"`
}

type Temperatures map[string]*Temperature

type Temperature struct {
	Actual *float64 `json:"actual"`
	Target *float64 `json:"target"`
	Offset *float64 `json:"offset"`
}

func (t *Temperatures) UnmarshalJSON(data []byte) error {
	var entries map[string]json.RawMessage
	if err := json.Unmarshal(data, &entries); err != nil {
		return err
	}
	if entries == nil {
		*t = nil
		return nil
	}
	temperatures := make(Temperatures)
	for name, entry := range entries {
		if !isTemperatureSource(name) {
			continue
		}
		var temperature *Temperature
		if err := json.Unmarshal(entry, &temperature); err != nil {
			return err
		}
		temperatures[name] = temperature
	}
	*t = temperatures
	return nil
}

func isTemperatureSource(name string) bool {
	return name == "bed" || name == "chamber" || strings.HasPrefix(name, "tool") && len(name) > 4 && strings.TrimLeft(name[4:], "0123456789") == ""
}

type JobResponse struct {
	Job      *JobDetails  `json:"job"`
	Progress *JobProgress `json:"progress"`
	State    *string      `json:"state"`
}

type JobDetails struct {
	File               *FileInfo `json:"file"`
	EstimatedPrintTime *float64  `json:"estimatedPrintTime"`
	AveragePrintTime   *float64  `json:"averagePrintTime"`
	LastPrintTime      *float64  `json:"lastPrintTime"`
}

type JobProgress struct {
	Completion          *float64 `json:"completion"`
	FilePosition        *int64   `json:"filepos"`
	PrintTime           *float64 `json:"printTime"`
	PrintTimeLeft       *float64 `json:"printTimeLeft"`
	PrintTimeLeftOrigin *string  `json:"printTimeLeftOrigin"`
}

type FilesResponse struct {
	Files []FileInfo `json:"files"`
	Free  *int64     `json:"free"`
	Total *int64     `json:"total"`
}

type UploadResponse struct {
	File            *FileInfo            `json:"file"`
	Files           map[string]*FileInfo `json:"files"`
	Done            *bool                `json:"done"`
	EffectiveSelect *bool                `json:"effectiveSelect"`
	EffectivePrint  *bool                `json:"effectivePrint"`
}

type FileInfo struct {
	Name     *string    `json:"name"`
	Display  *string    `json:"display"`
	Path     *string    `json:"path"`
	Type     *string    `json:"type"`
	Origin   *string    `json:"origin"`
	Size     *int64     `json:"size"`
	Date     *int64     `json:"date"`
	Children []FileInfo `json:"children"`
}

type VersionResponse struct {
	API    *string `json:"api"`
	Server *string `json:"server"`
	Text   *string `json:"text"`
}

type ServerResponse struct {
	Version  *string         `json:"version"`
	Safemode json.RawMessage `json:"safemode"`
}

type ToolTemperaturesResponse struct {
	Tools Temperatures
}

func (r *ToolTemperaturesResponse) UnmarshalJSON(data []byte) error {
	var temperatures *Temperatures
	if err := json.Unmarshal(data, &temperatures); err != nil {
		return err
	}
	if temperatures == nil {
		return errors.New("tool temperatures must be an object")
	}
	r.Tools = *temperatures
	return nil
}

type BedTemperatureResponse struct {
	Bed *Temperature `json:"bed"`
}

type ChamberTemperatureResponse struct {
	Chamber *Temperature `json:"chamber"`
}

type SDStateResponse struct {
	Ready *bool `json:"ready"`
}

type PrinterErrorResponse struct {
	Consequence *string `json:"consequence"`
	Message     *string `json:"error"`
	FAQ         *string `json:"faq"`
	Reason      *string `json:"reason"`
}

type ProfilesResponse struct {
	Profiles map[string]PrinterProfile `json:"profiles"`
}

type PrinterProfile struct {
	ID            *string          `json:"id"`
	Name          *string          `json:"name"`
	Color         *string          `json:"color"`
	Model         *string          `json:"model"`
	Default       *bool            `json:"default"`
	Current       *bool            `json:"current"`
	HeatedBed     *bool            `json:"heatedBed"`
	HeatedChamber *bool            `json:"heatedChamber"`
	Volume        *PrinterVolume   `json:"volume"`
	Extruder      *PrinterExtruder `json:"extruder"`
}

type PrinterVolume struct {
	Width      *float64 `json:"width"`
	Depth      *float64 `json:"depth"`
	Height     *float64 `json:"height"`
	FormFactor *string  `json:"formFactor"`
	Origin     *string  `json:"origin"`
}

type PrinterExtruder struct {
	Count          *int     `json:"count"`
	NozzleDiameter *float64 `json:"nozzleDiameter"`
	SharedNozzle   *bool    `json:"sharedNozzle"`
}

type SlicingResponse struct {
	Profiles map[string][]string
}

func (r *SlicingResponse) UnmarshalJSON(data []byte) error {
	var slicers map[string]struct {
		Profiles map[string]json.RawMessage `json:"profiles"`
	}
	if err := json.Unmarshal(data, &slicers); err != nil {
		return err
	}
	if slicers == nil {
		return errors.New("slicing response must be an object")
	}
	profiles := make(map[string][]string, len(slicers))
	for slicer, entry := range slicers {
		names := make([]string, 0, len(entry.Profiles))
		for name := range entry.Profiles {
			names = append(names, name)
		}
		sort.Strings(names)
		profiles[slicer] = names
	}
	r.Profiles = profiles
	return nil
}

type TimelapseResponse struct {
	Config     TimelapseConfig       `json:"config"`
	Files      []TimelapseFile       `json:"files"`
	Unrendered []UnrenderedTimelapse `json:"unrendered"`
}

type TimelapseConfig struct {
	Type             *string  `json:"type"`
	PostRoll         *int64   `json:"postRoll"`
	FPS              *int64   `json:"fps"`
	RetractionZHop   *float64 `json:"retractionZHop"`
	MinDelay         *int64   `json:"minDelay"`
	Interval         *int64   `json:"interval"`
	RenderAfterPrint *string  `json:"renderAfterPrint"`
}

type TimelapseFile struct {
	Name  *string `json:"name"`
	Size  *string `json:"size"`
	Bytes *int64  `json:"bytes"`
	Date  *string `json:"date"`
}

type UnrenderedTimelapse struct {
	Name       *string `json:"name"`
	Size       *string `json:"size"`
	Bytes      *int64  `json:"bytes"`
	Date       *string `json:"date"`
	Recording  *bool   `json:"recording"`
	Rendering  *bool   `json:"rendering"`
	Processing *bool   `json:"processing"`
}

type SettingsResponse struct {
	Feature *SettingsFeature `json:"feature"`
}

type SettingsFeature struct {
	TemperatureGraph            *bool `json:"temperatureGraph"`
	SDSupport                   *bool `json:"sdSupport"`
	ModelSizeDetection          *bool `json:"modelSizeDetection"`
	PrintStartConfirmation      *bool `json:"printStartConfirmation"`
	PrintCancelConfirmation     *bool `json:"printCancelConfirmation"`
	UploadOverwriteConfirmation *bool `json:"uploadOverwriteConfirmation"`
	FileDeleteConfirmation      *bool `json:"fileDeleteConfirmation"`
}

func readResponse[T any](ctx context.Context, client *Client, endpoint string) (*T, error) {
	return readResponseWithQuery[T](ctx, client, endpoint, nil)
}

func readResponseWithQuery[T any](ctx context.Context, client *Client, endpoint string, query url.Values) (*T, error) {
	body, err := client.getWithQuery(ctx, endpoint, query)
	if err != nil {
		return nil, err
	}
	var response *T
	if err := json.Unmarshal(body, &response); err != nil || response == nil {
		return nil, &Error{Kind: ErrorInvalidJSON}
	}
	return response, nil
}
