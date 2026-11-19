package mcpserver

import (
	"context"
	"errors"
	"math"
	"net/http"
	"strconv"
	"strings"
	"sync"

	"github.com/joaaomanooel/tupi-mcp/internal/config"
	"github.com/joaaomanooel/tupi-mcp/internal/octoprint"
)

const (
	connectionStateOperational = "Operational"
	connectionStateClosed      = "Closed"
	connectionStateError       = "Error"
	jobStateOperational        = "Operational"
	jobStateOffline            = "Offline"
)

type PhysicalOutcome string

const (
	PhysicalAccepted PhysicalOutcome = "accepted"
	PhysicalUnknown  PhysicalOutcome = "unknown"
)

type PhysicalErrorKind string

const (
	PhysicalErrorInvalidArgument   PhysicalErrorKind = "invalid_argument"
	PhysicalErrorLimitsUnavailable PhysicalErrorKind = "limits_unavailable"
	PhysicalErrorStateIncompatible PhysicalErrorKind = "state_incompatible"
	PhysicalErrorArgumentRejected  PhysicalErrorKind = "argument_rejected"
	PhysicalErrorPermissionDenied  PhysicalErrorKind = "permission_denied"
	PhysicalErrorResourceConflict  PhysicalErrorKind = "resource_conflict"
	PhysicalErrorOperationBusy     PhysicalErrorKind = "operation_busy"
	PhysicalErrorUnavailable       PhysicalErrorKind = "unavailable"
	PhysicalErrorUnknownResult     PhysicalErrorKind = "unknown_result"
)

type PhysicalError struct {
	Kind PhysicalErrorKind
}

func (e *PhysicalError) Error() string {
	return "physical: " + string(e.Kind)
}

type PhysicalReader interface {
	Connection(context.Context) (*octoprint.ConnectionResponse, error)
	Printer(context.Context) (*octoprint.PrinterResponse, error)
	Job(context.Context) (*octoprint.JobResponse, error)
	ToolTemperatures(context.Context) (*octoprint.ToolTemperaturesResponse, error)
	Profiles(context.Context) (*octoprint.ProfilesResponse, error)
}

type PhysicalWriter interface {
	Connect(context.Context) error
	Disconnect(context.Context) error
	JogPrinthead(context.Context, octoprint.PrintheadJog) error
	HomePrinthead(context.Context, []octoprint.PrintheadAxis) error
	SetFeedrate(context.Context, int) error
	SetToolTargets(context.Context, map[string]float64) error
	SetToolOffsets(context.Context, map[string]float64) error
	SelectTool(context.Context, string) error
	Extrude(context.Context, octoprint.Extrusion) error
	SetBedTarget(context.Context, float64) error
	SetBedOffset(context.Context, float64) error
	SetChamberTarget(context.Context, float64) error
	SetChamberOffset(context.Context, float64) error
	InitSD(context.Context) error
	RefreshSD(context.Context) error
	ReleaseSD(context.Context) error
}

type PhysicalCoordinator struct {
	reader PhysicalReader
	writer PhysicalWriter
	limits config.PhysicalLimits
	mutex  sync.Mutex
}

func NewPhysicalCoordinator(reader PhysicalReader, writer PhysicalWriter, limits config.PhysicalLimits) *PhysicalCoordinator {
	return &PhysicalCoordinator{reader: reader, writer: writer, limits: limits}
}

func (c *PhysicalCoordinator) tryReserve() (func(), bool) {
	if !c.mutex.TryLock() {
		return nil, false
	}
	return c.mutex.Unlock, true
}

var (
	_ PhysicalReader = (*octoprint.Client)(nil)
	_ PhysicalWriter = (*octoprint.Client)(nil)
)

type physicalAccess int

const (
	physicalAccessUnspecified physicalAccess = iota
	physicalAccessConnect
	physicalAccessOperational
)

type physicalOperation struct {
	access       physicalAccess
	profile      bool
	temperatures bool
	validate     func(*physicalSnapshot) error
	write        func(context.Context) error
}

type physicalSnapshot struct {
	connection   string
	flags        *octoprint.PrinterFlags
	jobState     string
	profile      *octoprint.PrinterProfile
	temperatures octoprint.Temperatures
}

func (c *PhysicalCoordinator) Connect(ctx context.Context) (PhysicalOutcome, error) {
	return c.execute(ctx, physicalOperation{
		access: physicalAccessConnect,
		write:  c.writer.Connect,
	})
}

func (c *PhysicalCoordinator) Reconnect(ctx context.Context) (PhysicalOutcome, error) {
	return c.execute(ctx, physicalOperation{
		access: physicalAccessOperational,
		write:  c.writer.Connect,
	})
}

func (c *PhysicalCoordinator) Disconnect(ctx context.Context) (PhysicalOutcome, error) {
	return c.execute(ctx, physicalOperation{
		access: physicalAccessOperational,
		write:  c.writer.Disconnect,
	})
}

func (c *PhysicalCoordinator) Jog(ctx context.Context, jog octoprint.PrintheadJog) (PhysicalOutcome, error) {
	return c.execute(ctx, physicalOperation{
		access: physicalAccessOperational,
		validate: func(*physicalSnapshot) error {
			return c.validateJog(jog)
		},
		write: func(ctx context.Context) error {
			return c.writer.JogPrinthead(ctx, jog)
		},
	})
}

func (c *PhysicalCoordinator) Home(ctx context.Context, axes []octoprint.PrintheadAxis) (PhysicalOutcome, error) {
	return c.execute(ctx, physicalOperation{
		access: physicalAccessOperational,
		validate: func(*physicalSnapshot) error {
			if err := validateHomeAxes(axes); err != nil {
				return err
			}
			if !c.limits.Configured.Axes {
				return physicalError(PhysicalErrorLimitsUnavailable)
			}
			for _, axis := range axes {
				if _, ok := c.limits.Axes[string(axis)]; !ok {
					return physicalError(PhysicalErrorLimitsUnavailable)
				}
			}
			return nil
		},
		write: func(ctx context.Context) error {
			return c.writer.HomePrinthead(ctx, axes)
		},
	})
}

func (c *PhysicalCoordinator) SetFeedrate(ctx context.Context, factor int) (PhysicalOutcome, error) {
	return c.execute(ctx, physicalOperation{
		access: physicalAccessOperational,
		write: func(ctx context.Context) error {
			return c.writer.SetFeedrate(ctx, factor)
		},
	})
}

func (c *PhysicalCoordinator) SetToolTarget(ctx context.Context, tool string, target float64) (PhysicalOutcome, error) {
	return c.execute(ctx, physicalOperation{
		access:  physicalAccessOperational,
		profile: true,
		validate: func(snapshot *physicalSnapshot) error {
			return c.validateToolTarget(snapshot.profile, tool, target)
		},
		write: func(ctx context.Context) error {
			return c.writer.SetToolTargets(ctx, map[string]float64{tool: target})
		},
	})
}

func (c *PhysicalCoordinator) SetToolOffset(ctx context.Context, tool string, offset float64) (PhysicalOutcome, error) {
	return c.execute(ctx, physicalOperation{
		access:  physicalAccessOperational,
		profile: true,
		validate: func(snapshot *physicalSnapshot) error {
			return c.validateToolOffset(snapshot.profile, tool, offset)
		},
		write: func(ctx context.Context) error {
			return c.writer.SetToolOffsets(ctx, map[string]float64{tool: offset})
		},
	})
}

func (c *PhysicalCoordinator) SelectTool(ctx context.Context, tool string) (PhysicalOutcome, error) {
	return c.execute(ctx, physicalOperation{
		access:  physicalAccessOperational,
		profile: true,
		validate: func(snapshot *physicalSnapshot) error {
			return c.validateToolSelection(snapshot.profile, tool)
		},
		write: func(ctx context.Context) error {
			return c.writer.SelectTool(ctx, tool)
		},
	})
}

func (c *PhysicalCoordinator) Extrude(ctx context.Context, amount, speed float64) (PhysicalOutcome, error) {
	return c.execute(ctx, physicalOperation{
		access:       physicalAccessOperational,
		profile:      true,
		temperatures: true,
		validate: func(snapshot *physicalSnapshot) error {
			return c.validateExtrusion(snapshot, amount, speed)
		},
		write: func(ctx context.Context) error {
			return c.writer.Extrude(ctx, octoprint.Extrusion{Amount: amount, Speed: speed})
		},
	})
}

func (c *PhysicalCoordinator) SetBedTarget(ctx context.Context, target float64) (PhysicalOutcome, error) {
	return c.execute(ctx, physicalOperation{
		access:  physicalAccessOperational,
		profile: true,
		validate: func(snapshot *physicalSnapshot) error {
			return c.validateHeaterValue("bed", heatedBed(snapshot.profile), target, temperatureLimit, true)
		},
		write: func(ctx context.Context) error {
			return c.writer.SetBedTarget(ctx, target)
		},
	})
}

func (c *PhysicalCoordinator) SetBedOffset(ctx context.Context, offset float64) (PhysicalOutcome, error) {
	return c.execute(ctx, physicalOperation{
		access:  physicalAccessOperational,
		profile: true,
		validate: func(snapshot *physicalSnapshot) error {
			return c.validateHeaterValue("bed", heatedBed(snapshot.profile), offset, offsetLimit, false)
		},
		write: func(ctx context.Context) error {
			return c.writer.SetBedOffset(ctx, offset)
		},
	})
}

func (c *PhysicalCoordinator) SetChamberTarget(ctx context.Context, target float64) (PhysicalOutcome, error) {
	return c.execute(ctx, physicalOperation{
		access:  physicalAccessOperational,
		profile: true,
		validate: func(snapshot *physicalSnapshot) error {
			return c.validateHeaterValue("chamber", heatedChamber(snapshot.profile), target, temperatureLimit, true)
		},
		write: func(ctx context.Context) error {
			return c.writer.SetChamberTarget(ctx, target)
		},
	})
}

func (c *PhysicalCoordinator) SetChamberOffset(ctx context.Context, offset float64) (PhysicalOutcome, error) {
	return c.execute(ctx, physicalOperation{
		access:  physicalAccessOperational,
		profile: true,
		validate: func(snapshot *physicalSnapshot) error {
			return c.validateHeaterValue("chamber", heatedChamber(snapshot.profile), offset, offsetLimit, false)
		},
		write: func(ctx context.Context) error {
			return c.writer.SetChamberOffset(ctx, offset)
		},
	})
}

func (c *PhysicalCoordinator) InitSD(ctx context.Context) (PhysicalOutcome, error) {
	return c.execute(ctx, physicalOperation{
		access: physicalAccessOperational,
		write:  c.writer.InitSD,
	})
}

func (c *PhysicalCoordinator) RefreshSD(ctx context.Context) (PhysicalOutcome, error) {
	return c.execute(ctx, physicalOperation{
		access: physicalAccessOperational,
		write:  c.writer.RefreshSD,
	})
}

func (c *PhysicalCoordinator) ReleaseSD(ctx context.Context) (PhysicalOutcome, error) {
	return c.execute(ctx, physicalOperation{
		access: physicalAccessOperational,
		write:  c.writer.ReleaseSD,
	})
}

func (c *PhysicalCoordinator) execute(ctx context.Context, operation physicalOperation) (PhysicalOutcome, error) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	snapshot, err := c.capture(ctx, operation)
	if err != nil {
		return "", err
	}
	if err := requirePhysicalState(snapshot, operation.access); err != nil {
		return "", err
	}
	if operation.validate != nil {
		if err := operation.validate(snapshot); err != nil {
			return "", err
		}
	}
	if err := operation.write(ctx); err != nil {
		return physicalWriteFailure(err)
	}
	return PhysicalAccepted, nil
}

func (c *PhysicalCoordinator) capture(ctx context.Context, operation physicalOperation) (*physicalSnapshot, error) {
	connection, err := c.reader.Connection(ctx)
	if err != nil {
		return nil, physicalReadFailure(err)
	}
	if connection == nil || connection.Current == nil || connection.Current.State == nil {
		return nil, physicalError(PhysicalErrorStateIncompatible)
	}
	snapshot := &physicalSnapshot{connection: *connection.Current.State}

	printer, err := c.reader.Printer(ctx)
	switch {
	case isPrinterNotOperational(err):
	case err != nil:
		return nil, physicalReadFailure(err)
	case printer == nil || printer.State == nil || printer.State.Flags == nil:
		return nil, physicalError(PhysicalErrorStateIncompatible)
	default:
		snapshot.flags = printer.State.Flags
	}

	job, err := c.reader.Job(ctx)
	if err != nil {
		return nil, physicalReadFailure(err)
	}
	if job == nil || job.State == nil {
		return nil, physicalError(PhysicalErrorStateIncompatible)
	}
	snapshot.jobState = *job.State

	if operation.profile {
		profile, err := c.currentProfile(ctx)
		if err != nil {
			return nil, err
		}
		snapshot.profile = profile
	}
	if operation.temperatures {
		temperatures, err := c.currentTemperatures(ctx)
		if err != nil {
			return nil, err
		}
		snapshot.temperatures = temperatures
	}
	return snapshot, nil
}

func (c *PhysicalCoordinator) currentProfile(ctx context.Context) (*octoprint.PrinterProfile, error) {
	profiles, err := c.reader.Profiles(ctx)
	if err != nil {
		return nil, physicalReadFailure(err)
	}
	if profiles == nil {
		return nil, physicalError(PhysicalErrorStateIncompatible)
	}
	var current *octoprint.PrinterProfile
	for _, profile := range profiles.Profiles {
		if profile.Current == nil || !*profile.Current {
			continue
		}
		if current != nil {
			return nil, physicalError(PhysicalErrorStateIncompatible)
		}
		selected := profile
		current = &selected
	}
	if current == nil {
		return nil, physicalError(PhysicalErrorStateIncompatible)
	}
	return current, nil
}

func (c *PhysicalCoordinator) currentTemperatures(ctx context.Context) (octoprint.Temperatures, error) {
	response, err := c.reader.ToolTemperatures(ctx)
	if err != nil {
		return nil, physicalReadFailure(err)
	}
	if response == nil || len(response.Tools) == 0 {
		return nil, physicalError(PhysicalErrorStateIncompatible)
	}
	return response.Tools, nil
}

func requirePhysicalState(snapshot *physicalSnapshot, access physicalAccess) error {
	switch access {
	case physicalAccessConnect:
		return requireConnectableState(snapshot)
	case physicalAccessOperational:
		return requireOperationalState(snapshot)
	}
	return physicalError(PhysicalErrorStateIncompatible)
}

func requireConnectableState(snapshot *physicalSnapshot) error {
	if snapshot.connection != connectionStateClosed && snapshot.connection != connectionStateError {
		return physicalError(PhysicalErrorStateIncompatible)
	}
	if snapshot.jobState != jobStateOffline {
		return physicalError(PhysicalErrorStateIncompatible)
	}
	if snapshot.flags == nil {
		return nil
	}
	if !physicalFlagsKnown(snapshot.flags) {
		return physicalError(PhysicalErrorStateIncompatible)
	}
	if *snapshot.flags.Operational || !*snapshot.flags.ClosedOrError || *snapshot.flags.Printing || *snapshot.flags.Paused {
		return physicalError(PhysicalErrorStateIncompatible)
	}
	return nil
}

func requireOperationalState(snapshot *physicalSnapshot) error {
	if snapshot.connection != connectionStateOperational {
		return physicalError(PhysicalErrorStateIncompatible)
	}
	if snapshot.flags == nil || !physicalFlagsKnown(snapshot.flags) {
		return physicalError(PhysicalErrorStateIncompatible)
	}
	if !*snapshot.flags.Operational || *snapshot.flags.ClosedOrError || *snapshot.flags.Printing || *snapshot.flags.Paused {
		return physicalError(PhysicalErrorStateIncompatible)
	}
	if snapshot.flags.Error != nil && *snapshot.flags.Error {
		return physicalError(PhysicalErrorStateIncompatible)
	}
	if snapshot.jobState != jobStateOperational {
		return physicalError(PhysicalErrorStateIncompatible)
	}
	return nil
}

func physicalFlagsKnown(flags *octoprint.PrinterFlags) bool {
	return flags.Operational != nil && flags.Printing != nil && flags.Paused != nil && flags.ClosedOrError != nil
}

func (c *PhysicalCoordinator) validateJog(jog octoprint.PrintheadJog) error {
	if !c.limits.Configured.Axes {
		return physicalError(PhysicalErrorLimitsUnavailable)
	}
	moving := false
	for _, entry := range []struct {
		name  string
		delta *float64
	}{{"x", jog.X}, {"y", jog.Y}, {"z", jog.Z}} {
		if entry.delta == nil {
			continue
		}
		axis, ok := c.limits.Axes[entry.name]
		if !ok {
			return physicalError(PhysicalErrorLimitsUnavailable)
		}
		if !withinLimit(*entry.delta, axis.Travel) || !withinLimit(jog.Speed, axis.Speed) {
			return physicalError(PhysicalErrorInvalidArgument)
		}
		moving = true
	}
	if !moving {
		return physicalError(PhysicalErrorInvalidArgument)
	}
	return nil
}

func validateHomeAxes(axes []octoprint.PrintheadAxis) error {
	if len(axes) == 0 {
		return physicalError(PhysicalErrorInvalidArgument)
	}
	seen := make(map[octoprint.PrintheadAxis]struct{}, len(axes))
	for _, axis := range axes {
		if axis != octoprint.PrintheadAxisX && axis != octoprint.PrintheadAxisY && axis != octoprint.PrintheadAxisZ {
			return physicalError(PhysicalErrorInvalidArgument)
		}
		if _, duplicate := seen[axis]; duplicate {
			return physicalError(PhysicalErrorInvalidArgument)
		}
		seen[axis] = struct{}{}
	}
	return nil
}

func (c *PhysicalCoordinator) validateToolTarget(profile *octoprint.PrinterProfile, tool string, target float64) error {
	if err := validateProfileTool(profile, tool); err != nil {
		return err
	}
	limits, ok := c.heaterLimits(tool)
	if !ok {
		return physicalError(PhysicalErrorLimitsUnavailable)
	}
	if target != 0 && !withinLimit(target, limits.Temperature) {
		return physicalError(PhysicalErrorInvalidArgument)
	}
	return nil
}

func (c *PhysicalCoordinator) validateToolOffset(profile *octoprint.PrinterProfile, tool string, offset float64) error {
	if err := validateProfileTool(profile, tool); err != nil {
		return err
	}
	limits, ok := c.heaterLimits(tool)
	if !ok {
		return physicalError(PhysicalErrorLimitsUnavailable)
	}
	if !withinLimit(offset, limits.Offset) {
		return physicalError(PhysicalErrorInvalidArgument)
	}
	return nil
}

func (c *PhysicalCoordinator) validateToolSelection(profile *octoprint.PrinterProfile, tool string) error {
	if err := validateProfileTool(profile, tool); err != nil {
		return err
	}
	if _, ok := c.heaterLimits(tool); !ok {
		return physicalError(PhysicalErrorLimitsUnavailable)
	}
	return nil
}

func (c *PhysicalCoordinator) validateExtrusion(snapshot *physicalSnapshot, amount, speed float64) error {
	if !c.limits.Configured.Extrusion {
		return physicalError(PhysicalErrorLimitsUnavailable)
	}
	limits := c.limits.Extrusion
	if !withinLimit(amount, limits.Length) || !withinLimit(speed, limits.Speed) {
		return physicalError(PhysicalErrorInvalidArgument)
	}
	if !profileSupportsTools(snapshot.profile) {
		return physicalError(PhysicalErrorStateIncompatible)
	}
	for _, temperature := range snapshot.temperatures {
		if temperature == nil || temperature.Actual == nil || *temperature.Actual < limits.MinimumTemperature {
			return physicalError(PhysicalErrorStateIncompatible)
		}
	}
	return nil
}

func (c *PhysicalCoordinator) validateHeaterValue(name string, supported *bool, value float64, limit heaterLimit, allowOff bool) error {
	if supported == nil || !*supported {
		return physicalError(PhysicalErrorStateIncompatible)
	}
	limits, ok := c.heaterLimits(name)
	if !ok {
		return physicalError(PhysicalErrorLimitsUnavailable)
	}
	if !(allowOff && value == 0) && !withinLimit(value, limit(limits)) {
		return physicalError(PhysicalErrorInvalidArgument)
	}
	return nil
}

func (c *PhysicalCoordinator) heaterLimits(name string) (config.HeaterLimits, bool) {
	if !c.limits.Configured.Heaters {
		return config.HeaterLimits{}, false
	}
	limits, ok := c.limits.Heaters[name]
	return limits, ok
}

type heaterLimit func(config.HeaterLimits) config.LimitRange

func temperatureLimit(limits config.HeaterLimits) config.LimitRange {
	return limits.Temperature
}

func offsetLimit(limits config.HeaterLimits) config.LimitRange {
	return limits.Offset
}

func heatedBed(profile *octoprint.PrinterProfile) *bool {
	if profile == nil {
		return nil
	}
	return profile.HeatedBed
}

func heatedChamber(profile *octoprint.PrinterProfile) *bool {
	if profile == nil {
		return nil
	}
	return profile.HeatedChamber
}

func validateProfileTool(profile *octoprint.PrinterProfile, tool string) error {
	index, ok := parseToolName(tool)
	if !ok {
		return physicalError(PhysicalErrorInvalidArgument)
	}
	if !profileSupportsTools(profile) {
		return physicalError(PhysicalErrorStateIncompatible)
	}
	if index >= *profile.Extruder.Count {
		return physicalError(PhysicalErrorStateIncompatible)
	}
	return nil
}

func profileSupportsTools(profile *octoprint.PrinterProfile) bool {
	return profile != nil && profile.Extruder != nil && profile.Extruder.Count != nil && *profile.Extruder.Count > 0
}

func parseToolName(name string) (int, bool) {
	index, found := strings.CutPrefix(name, "tool")
	if !found || index == "" {
		return 0, false
	}
	number, err := strconv.Atoi(index)
	if err != nil || number < 0 || "tool"+strconv.Itoa(number) != name {
		return 0, false
	}
	return number, true
}

func withinLimit(value float64, limits config.LimitRange) bool {
	return finiteValue(value) && value >= limits.Min && value <= limits.Max
}

func finiteValue(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0)
}

func physicalWriteFailure(err error) (PhysicalOutcome, error) {
	var operationError *octoprint.Error
	if errors.As(err, &operationError) {
		switch operationError.Kind {
		case octoprint.ErrorHTTPStatus:
			switch operationError.StatusCode {
			case http.StatusBadRequest:
				return "", physicalError(PhysicalErrorArgumentRejected)
			case http.StatusUnauthorized, http.StatusForbidden:
				return "", physicalError(PhysicalErrorPermissionDenied)
			case http.StatusConflict:
				return "", physicalError(PhysicalErrorResourceConflict)
			}
		case octoprint.ErrorInvalidArgument:
			return "", physicalError(PhysicalErrorInvalidArgument)
		case octoprint.ErrorTimeout, octoprint.ErrorTransport, octoprint.ErrorCanceled:
			return PhysicalUnknown, physicalError(PhysicalErrorUnknownResult)
		}
	}
	return PhysicalUnknown, physicalError(PhysicalErrorUnknownResult)
}

func physicalReadFailure(err error) error {
	var operationError *octoprint.Error
	if !errors.As(err, &operationError) {
		return physicalError(PhysicalErrorUnavailable)
	}
	if operationError.Kind != octoprint.ErrorHTTPStatus {
		return physicalError(PhysicalErrorUnavailable)
	}
	if operationError.StatusCode == http.StatusUnauthorized || operationError.StatusCode == http.StatusForbidden {
		return physicalError(PhysicalErrorPermissionDenied)
	}
	return physicalError(PhysicalErrorUnavailable)
}

func physicalError(kind PhysicalErrorKind) error {
	return &PhysicalError{Kind: kind}
}
