package octoprint

import (
	"context"
	"encoding/json"
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

func readResponse[T any](ctx context.Context, client *Client, endpoint string) (*T, error) {
	body, err := client.get(ctx, endpoint)
	if err != nil {
		return nil, err
	}
	var response *T
	if err := json.Unmarshal(body, &response); err != nil || response == nil {
		return nil, &Error{Kind: ErrorInvalidJSON}
	}
	return response, nil
}
