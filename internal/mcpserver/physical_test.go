package mcpserver

import (
	"context"
	"errors"
	"math"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/joaaomanooel/tupi-mcp/internal/config"
	"github.com/joaaomanooel/tupi-mcp/internal/octoprint"
)

type physicalMock struct {
	mutex        sync.Mutex
	connection   *octoprint.ConnectionResponse
	printer      *octoprint.PrinterResponse
	printerErr   error
	job          *octoprint.JobResponse
	profiles     *octoprint.ProfilesResponse
	temperatures *octoprint.ToolTemperaturesResponse
	readErr      error
	writeErr     error
	writeHook    func(string)

	readCount       int
	writes          []string
	activeWrites    int
	maxActiveWrites int
	lastJog         octoprint.PrintheadJog
	lastExtrusion   octoprint.Extrusion
	lastAxes        []octoprint.PrintheadAxis
	lastFactor      int
	lastToolTargets map[string]float64
	lastToolOffsets map[string]float64
	lastTool        string
	lastHeaterValue float64
}

func newPhysicalMock() *physicalMock {
	return &physicalMock{
		connection: &octoprint.ConnectionResponse{Current: &octoprint.ConnectionState{
			State: pointerTo(connectionStateOperational),
		}},
		printer: &octoprint.PrinterResponse{State: &octoprint.PrinterState{Flags: &octoprint.PrinterFlags{
			Operational:   pointerTo(true),
			Printing:      pointerTo(false),
			Paused:        pointerTo(false),
			ClosedOrError: pointerTo(false),
		}}},
		job: &octoprint.JobResponse{State: pointerTo(jobStateOperational)},
		profiles: &octoprint.ProfilesResponse{Profiles: map[string]octoprint.PrinterProfile{
			"test": {
				ID:            pointerTo("test"),
				Current:       pointerTo(true),
				HeatedBed:     pointerTo(true),
				HeatedChamber: pointerTo(false),
				Extruder:      &octoprint.PrinterExtruder{Count: pointerTo(1)},
			},
		}},
		temperatures: &octoprint.ToolTemperaturesResponse{Tools: octoprint.Temperatures{
			"tool0": {Actual: pointerTo(210.0), Target: pointerTo(210.0)},
		}},
	}
}

func (m *physicalMock) Connection(context.Context) (*octoprint.ConnectionResponse, error) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	m.readCount++
	return m.connection, m.readErr
}

func (m *physicalMock) Printer(context.Context) (*octoprint.PrinterResponse, error) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	m.readCount++
	if m.printerErr != nil {
		return nil, m.printerErr
	}
	return m.printer, m.readErr
}

func (m *physicalMock) Job(context.Context) (*octoprint.JobResponse, error) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	m.readCount++
	return m.job, m.readErr
}

func (m *physicalMock) ToolTemperatures(context.Context) (*octoprint.ToolTemperaturesResponse, error) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	m.readCount++
	return m.temperatures, m.readErr
}

func (m *physicalMock) Profiles(context.Context) (*octoprint.ProfilesResponse, error) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	m.readCount++
	return m.profiles, m.readErr
}

func (m *physicalMock) Connect(context.Context) error {
	return m.recordWrite("connect")
}

func (m *physicalMock) Disconnect(context.Context) error {
	return m.recordWrite("disconnect")
}

func (m *physicalMock) JogPrinthead(_ context.Context, jog octoprint.PrintheadJog) error {
	m.mutate(func(mock *physicalMock) { mock.lastJog = jog })
	return m.recordWrite("jog")
}

func (m *physicalMock) HomePrinthead(_ context.Context, axes []octoprint.PrintheadAxis) error {
	m.mutate(func(mock *physicalMock) { mock.lastAxes = append([]octoprint.PrintheadAxis(nil), axes...) })
	return m.recordWrite("home")
}

func (m *physicalMock) SetFeedrate(_ context.Context, factor int) error {
	m.mutate(func(mock *physicalMock) { mock.lastFactor = factor })
	return m.recordWrite("feedrate")
}

func (m *physicalMock) SetToolTargets(_ context.Context, targets map[string]float64) error {
	m.mutate(func(mock *physicalMock) { mock.lastToolTargets = targets })
	return m.recordWrite("tool_target")
}

func (m *physicalMock) SetToolOffsets(_ context.Context, offsets map[string]float64) error {
	m.mutate(func(mock *physicalMock) { mock.lastToolOffsets = offsets })
	return m.recordWrite("tool_offset")
}

func (m *physicalMock) SelectTool(_ context.Context, tool string) error {
	m.mutate(func(mock *physicalMock) { mock.lastTool = tool })
	return m.recordWrite("select_tool")
}

func (m *physicalMock) Extrude(_ context.Context, extrusion octoprint.Extrusion) error {
	m.mutate(func(mock *physicalMock) { mock.lastExtrusion = extrusion })
	return m.recordWrite("extrude")
}

func (m *physicalMock) SetBedTarget(_ context.Context, target float64) error {
	m.mutate(func(mock *physicalMock) { mock.lastHeaterValue = target })
	return m.recordWrite("bed_target")
}

func (m *physicalMock) SetBedOffset(_ context.Context, offset float64) error {
	m.mutate(func(mock *physicalMock) { mock.lastHeaterValue = offset })
	return m.recordWrite("bed_offset")
}

func (m *physicalMock) SetChamberTarget(_ context.Context, target float64) error {
	m.mutate(func(mock *physicalMock) { mock.lastHeaterValue = target })
	return m.recordWrite("chamber_target")
}

func (m *physicalMock) SetChamberOffset(_ context.Context, offset float64) error {
	m.mutate(func(mock *physicalMock) { mock.lastHeaterValue = offset })
	return m.recordWrite("chamber_offset")
}

func (m *physicalMock) InitSD(context.Context) error {
	return m.recordWrite("sd_init")
}

func (m *physicalMock) RefreshSD(context.Context) error {
	return m.recordWrite("sd_refresh")
}

func (m *physicalMock) ReleaseSD(context.Context) error {
	return m.recordWrite("sd_release")
}

func (m *physicalMock) recordWrite(name string) error {
	m.mutex.Lock()
	m.writes = append(m.writes, name)
	m.activeWrites++
	if m.activeWrites > m.maxActiveWrites {
		m.maxActiveWrites = m.activeWrites
	}
	writeErr := m.writeErr
	hook := m.writeHook
	m.mutex.Unlock()

	if hook != nil {
		hook(name)
	}

	m.mutex.Lock()
	m.activeWrites--
	m.mutex.Unlock()
	return writeErr
}

func (m *physicalMock) mutate(apply func(*physicalMock)) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	apply(m)
}

func (m *physicalMock) setClosed() {
	m.mutate(func(mock *physicalMock) {
		mock.connection = &octoprint.ConnectionResponse{Current: &octoprint.ConnectionState{
			State: pointerTo(connectionStateClosed),
		}}
		mock.printer = nil
		mock.printerErr = &octoprint.Error{Kind: octoprint.ErrorHTTPStatus, StatusCode: http.StatusConflict}
		mock.job = &octoprint.JobResponse{State: pointerTo(jobStateOffline)}
	})
}

func (m *physicalMock) setPrinting(state string) {
	m.mutate(func(mock *physicalMock) {
		mock.job.State = pointerTo(state)
		mock.printer.State.Flags.Printing = pointerTo(state == "Printing")
		mock.printer.State.Flags.Paused = pointerTo(state == "Paused")
	})
}

func (m *physicalMock) writeNames() []string {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	return append([]string(nil), m.writes...)
}

func (m *physicalMock) readTotal() int {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	return m.readCount
}

func (m *physicalMock) maxConcurrentWrites() int {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	return m.maxActiveWrites
}

func pointerTo[T any](value T) *T {
	return &value
}

func testPhysicalLimits() config.PhysicalLimits {
	axis := config.AxisLimits{
		Travel: config.LimitRange{Min: -100, Max: 100},
		Speed:  config.LimitRange{Min: 60, Max: 3000},
	}
	heater := config.HeaterLimits{
		Temperature: config.LimitRange{Min: 0, Max: 260},
		Offset:      config.LimitRange{Min: -5, Max: 5},
	}
	return config.PhysicalLimits{
		Axes:    map[string]config.AxisLimits{"x": axis, "y": axis, "z": axis},
		Heaters: map[string]config.HeaterLimits{"tool0": heater, "bed": heater, "chamber": heater},
		Extrusion: config.ExtrusionLimits{
			Length:             config.LimitRange{Min: -10, Max: 10},
			Speed:              config.LimitRange{Min: 60, Max: 300},
			MinimumTemperature: 180,
		},
		Configured: config.PhysicalDomains{Axes: true, Heaters: true, Extrusion: true},
	}
}

func newTestPhysicalCoordinator(limits config.PhysicalLimits) (*physicalMock, *PhysicalCoordinator) {
	mock := newPhysicalMock()
	return mock, NewPhysicalCoordinator(mock, mock, limits)
}

func testJog() octoprint.PrintheadJog {
	return octoprint.PrintheadJog{X: pointerTo(10.0), Speed: 1200}
}

func requirePhysicalKind(t *testing.T, err error, kind PhysicalErrorKind) {
	t.Helper()
	var physicalErr *PhysicalError
	if !errors.As(err, &physicalErr) {
		t.Fatalf("error = %v, want physical error kind %q", err, kind)
	}
	if physicalErr.Kind != kind {
		t.Fatalf("error kind = %q, want %q", physicalErr.Kind, kind)
	}
}

func requireNoPhysicalWrites(t *testing.T, mock *physicalMock) {
	t.Helper()
	if writes := mock.writeNames(); len(writes) != 0 {
		t.Fatalf("unexpected writes: %v", writes)
	}
}

func requireAcceptedWrite(t *testing.T, mock *physicalMock, outcome PhysicalOutcome, err error, name string) {
	t.Helper()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if outcome != PhysicalAccepted {
		t.Fatalf("outcome = %q, want %q", outcome, PhysicalAccepted)
	}
	if writes := mock.writeNames(); len(writes) != 1 || writes[0] != name {
		t.Fatalf("writes = %v, want [%s]", writes, name)
	}
}

func TestPhysicalConnectStateMatrix(t *testing.T) {
	tests := []struct {
		name   string
		adjust func(*physicalMock)
		kind   PhysicalErrorKind
	}{
		{
			name:   "closed printer",
			adjust: func(mock *physicalMock) { mock.setClosed() },
		},
		{
			name: "closed printer with reported flags",
			adjust: func(mock *physicalMock) {
				mock.setClosed()
				mock.mutate(func(mock *physicalMock) {
					mock.printer = &octoprint.PrinterResponse{State: &octoprint.PrinterState{Flags: &octoprint.PrinterFlags{
						Operational:   pointerTo(false),
						Printing:      pointerTo(false),
						Paused:        pointerTo(false),
						ClosedOrError: pointerTo(true),
					}}}
					mock.printerErr = nil
				})
			},
		},
		{
			name: "error connection with error flag",
			adjust: func(mock *physicalMock) {
				mock.mutate(func(mock *physicalMock) {
					mock.connection.Current.State = pointerTo(connectionStateError)
					mock.printer = &octoprint.PrinterResponse{State: &octoprint.PrinterState{Flags: &octoprint.PrinterFlags{
						Operational:   pointerTo(false),
						Printing:      pointerTo(false),
						Paused:        pointerTo(false),
						Error:         pointerTo(true),
						ClosedOrError: pointerTo(true),
					}}}
					mock.printerErr = nil
					mock.job.State = pointerTo(jobStateOffline)
				})
			},
		},
		{
			name:   "operational printer",
			adjust: func(*physicalMock) {},
			kind:   PhysicalErrorStateIncompatible,
		},
		{
			name: "connecting printer",
			adjust: func(mock *physicalMock) {
				mock.setClosed()
				mock.mutate(func(mock *physicalMock) {
					mock.connection.Current.State = pointerTo("Connecting")
				})
			},
			kind: PhysicalErrorStateIncompatible,
		},
		{
			name: "active job",
			adjust: func(mock *physicalMock) {
				mock.setClosed()
				mock.mutate(func(mock *physicalMock) { mock.job.State = pointerTo("Printing") })
			},
			kind: PhysicalErrorStateIncompatible,
		},
		{
			name: "unknown job state",
			adjust: func(mock *physicalMock) {
				mock.setClosed()
				mock.mutate(func(mock *physicalMock) { mock.job.State = pointerTo("Maintenance") })
			},
			kind: PhysicalErrorStateIncompatible,
		},
		{
			name: "contradictory flags",
			adjust: func(mock *physicalMock) {
				mock.setClosed()
				mock.mutate(func(mock *physicalMock) {
					mock.printer = &octoprint.PrinterResponse{State: &octoprint.PrinterState{Flags: &octoprint.PrinterFlags{
						Operational:   pointerTo(true),
						Printing:      pointerTo(false),
						Paused:        pointerTo(false),
						ClosedOrError: pointerTo(false),
					}}}
					mock.printerErr = nil
				})
			},
			kind: PhysicalErrorStateIncompatible,
		},
		{
			name: "missing flags",
			adjust: func(mock *physicalMock) {
				mock.setClosed()
				mock.mutate(func(mock *physicalMock) {
					mock.printer = &octoprint.PrinterResponse{State: &octoprint.PrinterState{}}
					mock.printerErr = nil
				})
			},
			kind: PhysicalErrorStateIncompatible,
		},
		{
			name: "missing connection state",
			adjust: func(mock *physicalMock) {
				mock.mutate(func(mock *physicalMock) {
					mock.connection = &octoprint.ConnectionResponse{Current: &octoprint.ConnectionState{}}
				})
			},
			kind: PhysicalErrorStateIncompatible,
		},
		{
			name: "missing job state",
			adjust: func(mock *physicalMock) {
				mock.setClosed()
				mock.mutate(func(mock *physicalMock) { mock.job = &octoprint.JobResponse{} })
			},
			kind: PhysicalErrorStateIncompatible,
		},
		{
			name: "read failure",
			adjust: func(mock *physicalMock) {
				mock.setClosed()
				mock.mutate(func(mock *physicalMock) {
					mock.printerErr = &octoprint.Error{Kind: octoprint.ErrorHTTPStatus, StatusCode: http.StatusInternalServerError}
				})
			},
			kind: PhysicalErrorUnavailable,
		},
		{
			name: "read permission failure",
			adjust: func(mock *physicalMock) {
				mock.setClosed()
				mock.mutate(func(mock *physicalMock) {
					mock.printerErr = &octoprint.Error{Kind: octoprint.ErrorHTTPStatus, StatusCode: http.StatusForbidden}
				})
			},
			kind: PhysicalErrorPermissionDenied,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			mock, coordinator := newTestPhysicalCoordinator(testPhysicalLimits())
			test.adjust(mock)

			outcome, err := coordinator.Connect(context.Background())

			if test.kind == "" {
				requireAcceptedWrite(t, mock, outcome, err, "connect")
				return
			}
			requirePhysicalKind(t, err, test.kind)
			requireNoPhysicalWrites(t, mock)
		})
	}
}

func TestPhysicalOperationalStateMatrix(t *testing.T) {
	tests := []struct {
		name   string
		adjust func(*physicalMock)
		kind   PhysicalErrorKind
	}{
		{name: "operational"},
		{
			name:   "printing",
			adjust: func(mock *physicalMock) { mock.setPrinting("Printing") },
			kind:   PhysicalErrorStateIncompatible,
		},
		{
			name:   "paused",
			adjust: func(mock *physicalMock) { mock.setPrinting("Paused") },
			kind:   PhysicalErrorStateIncompatible,
		},
		{
			name: "unknown job state",
			adjust: func(mock *physicalMock) {
				mock.mutate(func(mock *physicalMock) { mock.job.State = pointerTo("Maintenance") })
			},
			kind: PhysicalErrorStateIncompatible,
		},
		{
			name:   "closed printer",
			adjust: func(mock *physicalMock) { mock.setClosed() },
			kind:   PhysicalErrorStateIncompatible,
		},
		{
			name: "connecting printer",
			adjust: func(mock *physicalMock) {
				mock.mutate(func(mock *physicalMock) { mock.connection.Current.State = pointerTo("Connecting") })
			},
			kind: PhysicalErrorStateIncompatible,
		},
		{
			name: "offline job with operational connection",
			adjust: func(mock *physicalMock) {
				mock.mutate(func(mock *physicalMock) { mock.job.State = pointerTo(jobStateOffline) })
			},
			kind: PhysicalErrorStateIncompatible,
		},
		{
			name: "conflicting printing flag",
			adjust: func(mock *physicalMock) {
				mock.mutate(func(mock *physicalMock) { mock.printer.State.Flags.Printing = pointerTo(true) })
			},
			kind: PhysicalErrorStateIncompatible,
		},
		{
			name: "conflicting closed flag",
			adjust: func(mock *physicalMock) {
				mock.mutate(func(mock *physicalMock) { mock.printer.State.Flags.ClosedOrError = pointerTo(true) })
			},
			kind: PhysicalErrorStateIncompatible,
		},
		{
			name: "printer error flag",
			adjust: func(mock *physicalMock) {
				mock.mutate(func(mock *physicalMock) { mock.printer.State.Flags.Error = pointerTo(true) })
			},
			kind: PhysicalErrorStateIncompatible,
		},
		{
			name: "missing operational flag",
			adjust: func(mock *physicalMock) {
				mock.mutate(func(mock *physicalMock) { mock.printer.State.Flags.Operational = nil })
			},
			kind: PhysicalErrorStateIncompatible,
		},
		{
			name: "printer not operational",
			adjust: func(mock *physicalMock) {
				mock.mutate(func(mock *physicalMock) {
					mock.printer = nil
					mock.printerErr = &octoprint.Error{Kind: octoprint.ErrorHTTPStatus, StatusCode: http.StatusConflict}
				})
			},
			kind: PhysicalErrorStateIncompatible,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			mock, coordinator := newTestPhysicalCoordinator(testPhysicalLimits())
			if test.adjust != nil {
				test.adjust(mock)
			}

			outcome, err := coordinator.Jog(context.Background(), testJog())

			if test.kind == "" {
				requireAcceptedWrite(t, mock, outcome, err, "jog")
				return
			}
			requirePhysicalKind(t, err, test.kind)
			requireNoPhysicalWrites(t, mock)
		})
	}
}

func TestPhysicalConnectionCommands(t *testing.T) {
	t.Run("reconnect uses one connect call", func(t *testing.T) {
		mock, coordinator := newTestPhysicalCoordinator(testPhysicalLimits())

		outcome, err := coordinator.Reconnect(context.Background())

		requireAcceptedWrite(t, mock, outcome, err, "connect")
	})

	t.Run("reconnect requires operational state", func(t *testing.T) {
		mock, coordinator := newTestPhysicalCoordinator(testPhysicalLimits())
		mock.setClosed()

		_, err := coordinator.Reconnect(context.Background())

		requirePhysicalKind(t, err, PhysicalErrorStateIncompatible)
		requireNoPhysicalWrites(t, mock)
	})

	t.Run("disconnect from operational state", func(t *testing.T) {
		mock, coordinator := newTestPhysicalCoordinator(testPhysicalLimits())

		outcome, err := coordinator.Disconnect(context.Background())

		requireAcceptedWrite(t, mock, outcome, err, "disconnect")
	})

	t.Run("disconnect while printing", func(t *testing.T) {
		mock, coordinator := newTestPhysicalCoordinator(testPhysicalLimits())
		mock.setPrinting("Printing")

		_, err := coordinator.Disconnect(context.Background())

		requirePhysicalKind(t, err, PhysicalErrorStateIncompatible)
		requireNoPhysicalWrites(t, mock)
	})
}

func TestPhysicalJogLimits(t *testing.T) {
	tests := []struct {
		name   string
		limits func() config.PhysicalLimits
		jog    octoprint.PrintheadJog
		kind   PhysicalErrorKind
	}{
		{
			name:   "within limits",
			limits: testPhysicalLimits,
			jog:    testJog(),
		},
		{
			name:   "travel boundary",
			limits: testPhysicalLimits,
			jog:    octoprint.PrintheadJog{X: pointerTo(-100.0), Speed: 60},
		},
		{
			name:   "travel beyond maximum",
			limits: testPhysicalLimits,
			jog:    octoprint.PrintheadJog{X: pointerTo(100.5), Speed: 1200},
			kind:   PhysicalErrorInvalidArgument,
		},
		{
			name:   "travel below minimum",
			limits: testPhysicalLimits,
			jog:    octoprint.PrintheadJog{Z: pointerTo(-101.0), Speed: 1200},
			kind:   PhysicalErrorInvalidArgument,
		},
		{
			name:   "speed below range",
			limits: testPhysicalLimits,
			jog:    octoprint.PrintheadJog{Y: pointerTo(10.0), Speed: 59},
			kind:   PhysicalErrorInvalidArgument,
		},
		{
			name:   "not a number",
			limits: testPhysicalLimits,
			jog:    octoprint.PrintheadJog{X: pointerTo(math.NaN()), Speed: 1200},
			kind:   PhysicalErrorInvalidArgument,
		},
		{
			name:   "no axes",
			limits: testPhysicalLimits,
			jog:    octoprint.PrintheadJog{Speed: 1200},
			kind:   PhysicalErrorInvalidArgument,
		},
		{
			name: "missing axis limits",
			limits: func() config.PhysicalLimits {
				limits := testPhysicalLimits()
				delete(limits.Axes, "z")
				return limits
			},
			jog:  octoprint.PrintheadJog{Z: pointerTo(10.0), Speed: 1200},
			kind: PhysicalErrorLimitsUnavailable,
		},
		{
			name: "missing axes domain",
			limits: func() config.PhysicalLimits {
				limits := testPhysicalLimits()
				limits.Configured.Axes = false
				return limits
			},
			jog:  testJog(),
			kind: PhysicalErrorLimitsUnavailable,
		},
		{
			name: "speed must satisfy every moving axis",
			limits: func() config.PhysicalLimits {
				limits := testPhysicalLimits()
				axis := limits.Axes["y"]
				axis.Speed = config.LimitRange{Min: 2000, Max: 3000}
				limits.Axes["y"] = axis
				return limits
			},
			jog:  octoprint.PrintheadJog{X: pointerTo(10.0), Y: pointerTo(10.0), Speed: 1200},
			kind: PhysicalErrorInvalidArgument,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			mock, coordinator := newTestPhysicalCoordinator(test.limits())

			outcome, err := coordinator.Jog(context.Background(), test.jog)

			if test.kind == "" {
				requireAcceptedWrite(t, mock, outcome, err, "jog")
				return
			}
			requirePhysicalKind(t, err, test.kind)
			requireNoPhysicalWrites(t, mock)
		})
	}
}

func TestPhysicalHomeAxes(t *testing.T) {
	t.Run("valid axes", func(t *testing.T) {
		mock, coordinator := newTestPhysicalCoordinator(testPhysicalLimits())

		outcome, err := coordinator.Home(context.Background(), []octoprint.PrintheadAxis{
			octoprint.PrintheadAxisX,
			octoprint.PrintheadAxisY,
		})

		requireAcceptedWrite(t, mock, outcome, err, "home")
		mock.mutate(func(mock *physicalMock) {
			if len(mock.lastAxes) != 2 || mock.lastAxes[0] != octoprint.PrintheadAxisX || mock.lastAxes[1] != octoprint.PrintheadAxisY {
				t.Errorf("axes = %v, want [x y]", mock.lastAxes)
			}
		})
	})

	tests := []struct {
		name string
		axes []octoprint.PrintheadAxis
	}{
		{name: "empty", axes: nil},
		{name: "unknown axis", axes: []octoprint.PrintheadAxis{"e"}},
		{name: "duplicate axis", axes: []octoprint.PrintheadAxis{octoprint.PrintheadAxisX, octoprint.PrintheadAxisX}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			mock, coordinator := newTestPhysicalCoordinator(testPhysicalLimits())

			_, err := coordinator.Home(context.Background(), test.axes)

			requirePhysicalKind(t, err, PhysicalErrorInvalidArgument)
			requireNoPhysicalWrites(t, mock)
		})
	}

	t.Run("missing axes domain", func(t *testing.T) {
		limits := testPhysicalLimits()
		limits.Configured.Axes = false
		mock, coordinator := newTestPhysicalCoordinator(limits)

		_, err := coordinator.Home(context.Background(), []octoprint.PrintheadAxis{octoprint.PrintheadAxisX})

		requirePhysicalKind(t, err, PhysicalErrorLimitsUnavailable)
		requireNoPhysicalWrites(t, mock)
	})

	t.Run("missing selected axis limits", func(t *testing.T) {
		limits := testPhysicalLimits()
		delete(limits.Axes, "y")
		mock, coordinator := newTestPhysicalCoordinator(limits)

		_, err := coordinator.Home(context.Background(), []octoprint.PrintheadAxis{octoprint.PrintheadAxisY})

		requirePhysicalKind(t, err, PhysicalErrorLimitsUnavailable)
		requireNoPhysicalWrites(t, mock)
	})

	t.Run("while printing", func(t *testing.T) {
		mock, coordinator := newTestPhysicalCoordinator(testPhysicalLimits())
		mock.setPrinting("Printing")

		_, err := coordinator.Home(context.Background(), []octoprint.PrintheadAxis{octoprint.PrintheadAxisX})

		requirePhysicalKind(t, err, PhysicalErrorStateIncompatible)
		requireNoPhysicalWrites(t, mock)
	})
}

func TestPhysicalToolTargets(t *testing.T) {
	t.Run("valid target", func(t *testing.T) {
		mock, coordinator := newTestPhysicalCoordinator(testPhysicalLimits())

		outcome, err := coordinator.SetToolTarget(context.Background(), "tool0", 200)

		requireAcceptedWrite(t, mock, outcome, err, "tool_target")
		mock.mutate(func(mock *physicalMock) {
			if mock.lastToolTargets["tool0"] != 200 {
				t.Errorf("targets = %v, want tool0=200", mock.lastToolTargets)
			}
		})
	})

	t.Run("zero target allowed within limits", func(t *testing.T) {
		mock, coordinator := newTestPhysicalCoordinator(testPhysicalLimits())

		outcome, err := coordinator.SetToolTarget(context.Background(), "tool0", 0)

		requireAcceptedWrite(t, mock, outcome, err, "tool_target")
	})

	t.Run("zero target allowed below configured minimum", func(t *testing.T) {
		limits := testPhysicalLimits()
		heater := limits.Heaters["tool0"]
		heater.Temperature.Min = 150
		limits.Heaters["tool0"] = heater
		mock, coordinator := newTestPhysicalCoordinator(limits)

		outcome, err := coordinator.SetToolTarget(context.Background(), "tool0", 0)

		requireAcceptedWrite(t, mock, outcome, err, "tool_target")
	})

	tests := []struct {
		name   string
		adjust func(*physicalMock, *config.PhysicalLimits)
		tool   string
		target float64
		kind   PhysicalErrorKind
	}{
		{
			name:   "tool outside active profile",
			tool:   "tool1",
			target: 200,
			kind:   PhysicalErrorStateIncompatible,
		},
		{
			name:   "malformed tool name",
			tool:   "tool",
			target: 200,
			kind:   PhysicalErrorInvalidArgument,
		},
		{
			name:   "target above range",
			tool:   "tool0",
			target: 261,
			kind:   PhysicalErrorInvalidArgument,
		},
		{
			name:   "target not finite",
			tool:   "tool0",
			target: math.Inf(1),
			kind:   PhysicalErrorInvalidArgument,
		},
		{
			name: "missing tool limits",
			adjust: func(_ *physicalMock, limits *config.PhysicalLimits) {
				delete(limits.Heaters, "tool0")
			},
			tool:   "tool0",
			target: 200,
			kind:   PhysicalErrorLimitsUnavailable,
		},
		{
			name: "missing heaters domain",
			adjust: func(_ *physicalMock, limits *config.PhysicalLimits) {
				limits.Configured.Heaters = false
			},
			tool:   "tool0",
			target: 200,
			kind:   PhysicalErrorLimitsUnavailable,
		},
		{
			name: "missing active profile",
			adjust: func(mock *physicalMock, _ *config.PhysicalLimits) {
				mock.mutate(func(mock *physicalMock) {
					mock.profiles = &octoprint.ProfilesResponse{Profiles: map[string]octoprint.PrinterProfile{
						"test": {ID: pointerTo("test"), Current: pointerTo(false)},
					}}
				})
			},
			tool:   "tool0",
			target: 200,
			kind:   PhysicalErrorStateIncompatible,
		},
		{
			name: "multiple active profiles",
			adjust: func(mock *physicalMock, _ *config.PhysicalLimits) {
				mock.mutate(func(mock *physicalMock) {
					mock.profiles.Profiles["other"] = octoprint.PrinterProfile{Current: pointerTo(true)}
				})
			},
			tool:   "tool0",
			target: 200,
			kind:   PhysicalErrorStateIncompatible,
		},
		{
			name: "profile read failure",
			adjust: func(mock *physicalMock, _ *config.PhysicalLimits) {
				mock.mutate(func(mock *physicalMock) {
					mock.profiles = nil
					mock.readErr = &octoprint.Error{Kind: octoprint.ErrorTimeout}
				})
			},
			tool:   "tool0",
			target: 200,
			kind:   PhysicalErrorUnavailable,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			limits := testPhysicalLimits()
			mock := newPhysicalMock()
			if test.adjust != nil {
				test.adjust(mock, &limits)
			}
			coordinator := NewPhysicalCoordinator(mock, mock, limits)

			_, err := coordinator.SetToolTarget(context.Background(), test.tool, test.target)

			requirePhysicalKind(t, err, test.kind)
			requireNoPhysicalWrites(t, mock)
		})
	}
}

func TestPhysicalToolOffsets(t *testing.T) {
	t.Run("valid offset", func(t *testing.T) {
		mock, coordinator := newTestPhysicalCoordinator(testPhysicalLimits())

		outcome, err := coordinator.SetToolOffset(context.Background(), "tool0", -4)

		requireAcceptedWrite(t, mock, outcome, err, "tool_offset")
		mock.mutate(func(mock *physicalMock) {
			if mock.lastToolOffsets["tool0"] != -4 {
				t.Errorf("offsets = %v, want tool0=-4", mock.lastToolOffsets)
			}
		})
	})

	t.Run("offset outside range", func(t *testing.T) {
		mock, coordinator := newTestPhysicalCoordinator(testPhysicalLimits())

		_, err := coordinator.SetToolOffset(context.Background(), "tool0", 6)

		requirePhysicalKind(t, err, PhysicalErrorInvalidArgument)
		requireNoPhysicalWrites(t, mock)
	})

	t.Run("tool outside active profile", func(t *testing.T) {
		mock, coordinator := newTestPhysicalCoordinator(testPhysicalLimits())

		_, err := coordinator.SetToolOffset(context.Background(), "tool1", 1)

		requirePhysicalKind(t, err, PhysicalErrorStateIncompatible)
		requireNoPhysicalWrites(t, mock)
	})
}

func TestPhysicalSelectTool(t *testing.T) {
	t.Run("selects configured tool", func(t *testing.T) {
		mock, coordinator := newTestPhysicalCoordinator(testPhysicalLimits())

		outcome, err := coordinator.SelectTool(context.Background(), "tool0")

		requireAcceptedWrite(t, mock, outcome, err, "select_tool")
	})

	t.Run("selects second configured tool", func(t *testing.T) {
		limits := testPhysicalLimits()
		limits.Heaters["tool1"] = limits.Heaters["tool0"]
		mock := newPhysicalMock()
		mock.mutate(func(mock *physicalMock) {
			mock.profiles.Profiles["test"] = octoprint.PrinterProfile{
				Current:  pointerTo(true),
				Extruder: &octoprint.PrinterExtruder{Count: pointerTo(2)},
			}
		})
		coordinator := NewPhysicalCoordinator(mock, mock, limits)

		outcome, err := coordinator.SelectTool(context.Background(), "tool1")

		requireAcceptedWrite(t, mock, outcome, err, "select_tool")
	})

	t.Run("unconfigured tool limits", func(t *testing.T) {
		limits := testPhysicalLimits()
		mock := newPhysicalMock()
		mock.mutate(func(mock *physicalMock) {
			mock.profiles.Profiles["test"] = octoprint.PrinterProfile{
				Current:  pointerTo(true),
				Extruder: &octoprint.PrinterExtruder{Count: pointerTo(2)},
			}
		})
		coordinator := NewPhysicalCoordinator(mock, mock, limits)

		_, err := coordinator.SelectTool(context.Background(), "tool1")

		requirePhysicalKind(t, err, PhysicalErrorLimitsUnavailable)
		requireNoPhysicalWrites(t, mock)
	})

	t.Run("tool outside active profile", func(t *testing.T) {
		mock, coordinator := newTestPhysicalCoordinator(testPhysicalLimits())

		_, err := coordinator.SelectTool(context.Background(), "tool1")

		requirePhysicalKind(t, err, PhysicalErrorStateIncompatible)
		requireNoPhysicalWrites(t, mock)
	})
}

func TestPhysicalHeaterSupport(t *testing.T) {
	t.Run("bed target", func(t *testing.T) {
		mock, coordinator := newTestPhysicalCoordinator(testPhysicalLimits())

		outcome, err := coordinator.SetBedTarget(context.Background(), 60)

		requireAcceptedWrite(t, mock, outcome, err, "bed_target")
	})

	t.Run("bed target zero allowed below configured minimum", func(t *testing.T) {
		limits := testPhysicalLimits()
		heater := limits.Heaters["bed"]
		heater.Temperature.Min = 50
		limits.Heaters["bed"] = heater
		mock, coordinator := newTestPhysicalCoordinator(limits)

		outcome, err := coordinator.SetBedTarget(context.Background(), 0)

		requireAcceptedWrite(t, mock, outcome, err, "bed_target")
	})

	t.Run("bed offset", func(t *testing.T) {
		mock, coordinator := newTestPhysicalCoordinator(testPhysicalLimits())

		outcome, err := coordinator.SetBedOffset(context.Background(), -2)

		requireAcceptedWrite(t, mock, outcome, err, "bed_offset")
	})

	t.Run("bed offset below range", func(t *testing.T) {
		mock, coordinator := newTestPhysicalCoordinator(testPhysicalLimits())

		_, err := coordinator.SetBedOffset(context.Background(), -6)

		requirePhysicalKind(t, err, PhysicalErrorInvalidArgument)
		requireNoPhysicalWrites(t, mock)
	})

	t.Run("bed offset above range", func(t *testing.T) {
		mock, coordinator := newTestPhysicalCoordinator(testPhysicalLimits())

		_, err := coordinator.SetBedOffset(context.Background(), 6)

		requirePhysicalKind(t, err, PhysicalErrorInvalidArgument)
		requireNoPhysicalWrites(t, mock)
	})

	t.Run("profile without heated bed", func(t *testing.T) {
		mock, coordinator := newTestPhysicalCoordinator(testPhysicalLimits())
		mock.mutate(func(mock *physicalMock) {
			profile := mock.profiles.Profiles["test"]
			profile.HeatedBed = pointerTo(false)
			mock.profiles.Profiles["test"] = profile
		})

		_, err := coordinator.SetBedTarget(context.Background(), 60)

		requirePhysicalKind(t, err, PhysicalErrorStateIncompatible)
		requireNoPhysicalWrites(t, mock)
	})

	t.Run("profile without bed declaration", func(t *testing.T) {
		mock, coordinator := newTestPhysicalCoordinator(testPhysicalLimits())
		mock.mutate(func(mock *physicalMock) {
			profile := mock.profiles.Profiles["test"]
			profile.HeatedBed = nil
			mock.profiles.Profiles["test"] = profile
		})

		_, err := coordinator.SetBedTarget(context.Background(), 60)

		requirePhysicalKind(t, err, PhysicalErrorStateIncompatible)
		requireNoPhysicalWrites(t, mock)
	})

	t.Run("missing bed limits", func(t *testing.T) {
		limits := testPhysicalLimits()
		delete(limits.Heaters, "bed")
		mock, coordinator := newTestPhysicalCoordinator(limits)

		_, err := coordinator.SetBedTarget(context.Background(), 60)

		requirePhysicalKind(t, err, PhysicalErrorLimitsUnavailable)
		requireNoPhysicalWrites(t, mock)
	})

	t.Run("bed target outside range", func(t *testing.T) {
		mock, coordinator := newTestPhysicalCoordinator(testPhysicalLimits())

		_, err := coordinator.SetBedTarget(context.Background(), 300)

		requirePhysicalKind(t, err, PhysicalErrorInvalidArgument)
		requireNoPhysicalWrites(t, mock)
	})

	t.Run("profile without heated chamber", func(t *testing.T) {
		mock, coordinator := newTestPhysicalCoordinator(testPhysicalLimits())

		_, err := coordinator.SetChamberTarget(context.Background(), 40)

		requirePhysicalKind(t, err, PhysicalErrorStateIncompatible)
		requireNoPhysicalWrites(t, mock)
	})

	t.Run("chamber target with support", func(t *testing.T) {
		mock, coordinator := newTestPhysicalCoordinator(testPhysicalLimits())
		mock.mutate(func(mock *physicalMock) {
			profile := mock.profiles.Profiles["test"]
			profile.HeatedChamber = pointerTo(true)
			mock.profiles.Profiles["test"] = profile
		})

		outcome, err := coordinator.SetChamberTarget(context.Background(), 40)

		requireAcceptedWrite(t, mock, outcome, err, "chamber_target")
	})

	t.Run("chamber target above range with support", func(t *testing.T) {
		mock, coordinator := newTestPhysicalCoordinator(testPhysicalLimits())
		mock.mutate(func(mock *physicalMock) {
			profile := mock.profiles.Profiles["test"]
			profile.HeatedChamber = pointerTo(true)
			mock.profiles.Profiles["test"] = profile
		})

		_, err := coordinator.SetChamberTarget(context.Background(), 300)

		requirePhysicalKind(t, err, PhysicalErrorInvalidArgument)
		requireNoPhysicalWrites(t, mock)
	})

	t.Run("missing chamber limits", func(t *testing.T) {
		limits := testPhysicalLimits()
		delete(limits.Heaters, "chamber")
		mock, coordinator := newTestPhysicalCoordinator(limits)
		mock.mutate(func(mock *physicalMock) {
			profile := mock.profiles.Profiles["test"]
			profile.HeatedChamber = pointerTo(true)
			mock.profiles.Profiles["test"] = profile
		})

		_, err := coordinator.SetChamberTarget(context.Background(), 40)

		requirePhysicalKind(t, err, PhysicalErrorLimitsUnavailable)
		requireNoPhysicalWrites(t, mock)
	})

	t.Run("chamber offset with support", func(t *testing.T) {
		mock, coordinator := newTestPhysicalCoordinator(testPhysicalLimits())
		mock.mutate(func(mock *physicalMock) {
			profile := mock.profiles.Profiles["test"]
			profile.HeatedChamber = pointerTo(true)
			mock.profiles.Profiles["test"] = profile
		})

		outcome, err := coordinator.SetChamberOffset(context.Background(), -2)

		requireAcceptedWrite(t, mock, outcome, err, "chamber_offset")
	})

	t.Run("chamber offset below range with support", func(t *testing.T) {
		mock, coordinator := newTestPhysicalCoordinator(testPhysicalLimits())
		mock.mutate(func(mock *physicalMock) {
			profile := mock.profiles.Profiles["test"]
			profile.HeatedChamber = pointerTo(true)
			mock.profiles.Profiles["test"] = profile
		})

		_, err := coordinator.SetChamberOffset(context.Background(), -6)

		requirePhysicalKind(t, err, PhysicalErrorInvalidArgument)
		requireNoPhysicalWrites(t, mock)
	})

	t.Run("chamber offset above range with support", func(t *testing.T) {
		mock, coordinator := newTestPhysicalCoordinator(testPhysicalLimits())
		mock.mutate(func(mock *physicalMock) {
			profile := mock.profiles.Profiles["test"]
			profile.HeatedChamber = pointerTo(true)
			mock.profiles.Profiles["test"] = profile
		})

		_, err := coordinator.SetChamberOffset(context.Background(), 6)

		requirePhysicalKind(t, err, PhysicalErrorInvalidArgument)
		requireNoPhysicalWrites(t, mock)
	})

	t.Run("missing chamber offset limits", func(t *testing.T) {
		limits := testPhysicalLimits()
		delete(limits.Heaters, "chamber")
		mock, coordinator := newTestPhysicalCoordinator(limits)
		mock.mutate(func(mock *physicalMock) {
			profile := mock.profiles.Profiles["test"]
			profile.HeatedChamber = pointerTo(true)
			mock.profiles.Profiles["test"] = profile
		})

		_, err := coordinator.SetChamberOffset(context.Background(), -2)

		requirePhysicalKind(t, err, PhysicalErrorLimitsUnavailable)
		requireNoPhysicalWrites(t, mock)
	})
}

func TestPhysicalExtrusion(t *testing.T) {
	t.Run("extrudes above minimum temperature", func(t *testing.T) {
		mock, coordinator := newTestPhysicalCoordinator(testPhysicalLimits())

		outcome, err := coordinator.Extrude(context.Background(), 5, 120)

		requireAcceptedWrite(t, mock, outcome, err, "extrude")
		mock.mutate(func(mock *physicalMock) {
			if mock.lastExtrusion.Amount != 5 || mock.lastExtrusion.Speed != 120 {
				t.Errorf("extrusion = %+v, want amount 5 speed 120", mock.lastExtrusion)
			}
		})
	})

	t.Run("retracts within negative range", func(t *testing.T) {
		mock, coordinator := newTestPhysicalCoordinator(testPhysicalLimits())

		outcome, err := coordinator.Extrude(context.Background(), -5, 120)

		requireAcceptedWrite(t, mock, outcome, err, "extrude")
	})

	tests := []struct {
		name   string
		adjust func(*physicalMock, *config.PhysicalLimits)
		amount float64
		speed  float64
		kind   PhysicalErrorKind
	}{
		{
			name: "cold hotend",
			adjust: func(mock *physicalMock, _ *config.PhysicalLimits) {
				mock.temperatures.Tools["tool0"].Actual = pointerTo(179.9)
			},
			amount: 5,
			speed:  120,
			kind:   PhysicalErrorStateIncompatible,
		},
		{
			name: "unknown temperature",
			adjust: func(mock *physicalMock, _ *config.PhysicalLimits) {
				mock.temperatures.Tools["tool0"].Actual = nil
			},
			amount: 5,
			speed:  120,
			kind:   PhysicalErrorStateIncompatible,
		},
		{
			name: "no reported tools",
			adjust: func(mock *physicalMock, _ *config.PhysicalLimits) {
				mock.temperatures.Tools = octoprint.Temperatures{}
			},
			amount: 5,
			speed:  120,
			kind:   PhysicalErrorStateIncompatible,
		},
		{
			name: "second cold tool",
			adjust: func(mock *physicalMock, _ *config.PhysicalLimits) {
				mock.temperatures.Tools["tool1"] = &octoprint.Temperature{Actual: pointerTo(25.0)}
			},
			amount: 5,
			speed:  120,
			kind:   PhysicalErrorStateIncompatible,
		},
		{
			name: "amount above range",
			adjust: func(*physicalMock, *config.PhysicalLimits) {
			},
			amount: 11,
			speed:  120,
			kind:   PhysicalErrorInvalidArgument,
		},
		{
			name: "retraction below range",
			adjust: func(*physicalMock, *config.PhysicalLimits) {
			},
			amount: -11,
			speed:  120,
			kind:   PhysicalErrorInvalidArgument,
		},
		{
			name: "speed below range",
			adjust: func(*physicalMock, *config.PhysicalLimits) {
			},
			amount: 5,
			speed:  59,
			kind:   PhysicalErrorInvalidArgument,
		},
		{
			name: "speed above range",
			adjust: func(*physicalMock, *config.PhysicalLimits) {
			},
			amount: 5,
			speed:  301,
			kind:   PhysicalErrorInvalidArgument,
		},
		{
			name: "missing extrusion limits",
			adjust: func(_ *physicalMock, limits *config.PhysicalLimits) {
				limits.Configured.Extrusion = false
			},
			amount: 5,
			speed:  120,
			kind:   PhysicalErrorLimitsUnavailable,
		},
		{
			name: "profile without extruder",
			adjust: func(mock *physicalMock, _ *config.PhysicalLimits) {
				mock.mutate(func(mock *physicalMock) {
					profile := mock.profiles.Profiles["test"]
					profile.Extruder = nil
					mock.profiles.Profiles["test"] = profile
				})
			},
			amount: 5,
			speed:  120,
			kind:   PhysicalErrorStateIncompatible,
		},
		{
			name: "temperature read failure",
			adjust: func(mock *physicalMock, _ *config.PhysicalLimits) {
				mock.mutate(func(mock *physicalMock) {
					mock.readErr = &octoprint.Error{Kind: octoprint.ErrorTransport}
				})
			},
			amount: 5,
			speed:  120,
			kind:   PhysicalErrorUnavailable,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			limits := testPhysicalLimits()
			mock := newPhysicalMock()
			test.adjust(mock, &limits)
			coordinator := NewPhysicalCoordinator(mock, mock, limits)

			_, err := coordinator.Extrude(context.Background(), test.amount, test.speed)

			requirePhysicalKind(t, err, test.kind)
			requireNoPhysicalWrites(t, mock)
		})
	}
}

func TestPhysicalSDCommands(t *testing.T) {
	t.Run("init", func(t *testing.T) {
		mock, coordinator := newTestPhysicalCoordinator(testPhysicalLimits())
		outcome, err := coordinator.InitSD(context.Background())
		requireAcceptedWrite(t, mock, outcome, err, "sd_init")
	})
	t.Run("refresh", func(t *testing.T) {
		mock, coordinator := newTestPhysicalCoordinator(testPhysicalLimits())
		outcome, err := coordinator.RefreshSD(context.Background())
		requireAcceptedWrite(t, mock, outcome, err, "sd_refresh")
	})
	t.Run("release", func(t *testing.T) {
		mock, coordinator := newTestPhysicalCoordinator(testPhysicalLimits())
		outcome, err := coordinator.ReleaseSD(context.Background())
		requireAcceptedWrite(t, mock, outcome, err, "sd_release")
	})
	t.Run("while printing", func(t *testing.T) {
		mock, coordinator := newTestPhysicalCoordinator(testPhysicalLimits())
		mock.setPrinting("Printing")
		_, err := coordinator.InitSD(context.Background())
		requirePhysicalKind(t, err, PhysicalErrorStateIncompatible)
		requireNoPhysicalWrites(t, mock)
	})
	t.Run("refresh while printing", func(t *testing.T) {
		mock, coordinator := newTestPhysicalCoordinator(testPhysicalLimits())
		mock.setPrinting("Printing")
		_, err := coordinator.RefreshSD(context.Background())
		requirePhysicalKind(t, err, PhysicalErrorStateIncompatible)
		requireNoPhysicalWrites(t, mock)
	})
	t.Run("refresh while paused", func(t *testing.T) {
		mock, coordinator := newTestPhysicalCoordinator(testPhysicalLimits())
		mock.setPrinting("Paused")
		_, err := coordinator.RefreshSD(context.Background())
		requirePhysicalKind(t, err, PhysicalErrorStateIncompatible)
		requireNoPhysicalWrites(t, mock)
	})
	t.Run("release while printing", func(t *testing.T) {
		mock, coordinator := newTestPhysicalCoordinator(testPhysicalLimits())
		mock.setPrinting("Printing")
		_, err := coordinator.ReleaseSD(context.Background())
		requirePhysicalKind(t, err, PhysicalErrorStateIncompatible)
		requireNoPhysicalWrites(t, mock)
	})
	t.Run("release while paused", func(t *testing.T) {
		mock, coordinator := newTestPhysicalCoordinator(testPhysicalLimits())
		mock.setPrinting("Paused")
		_, err := coordinator.ReleaseSD(context.Background())
		requirePhysicalKind(t, err, PhysicalErrorStateIncompatible)
		requireNoPhysicalWrites(t, mock)
	})
}

func TestPhysicalFeedrate(t *testing.T) {
	mock, coordinator := newTestPhysicalCoordinator(testPhysicalLimits())

	outcome, err := coordinator.SetFeedrate(context.Background(), 150)

	requireAcceptedWrite(t, mock, outcome, err, "feedrate")
	mock.mutate(func(mock *physicalMock) {
		if mock.lastFactor != 150 {
			t.Errorf("factor = %d, want 150", mock.lastFactor)
		}
	})
}

func TestPhysicalWriteFailureCategories(t *testing.T) {
	tests := []struct {
		name        string
		writeErr    error
		wantKind    PhysicalErrorKind
		wantOutcome PhysicalOutcome
	}{
		{
			name:     "octoprint rejects the argument",
			writeErr: &octoprint.Error{Kind: octoprint.ErrorHTTPStatus, StatusCode: http.StatusBadRequest},
			wantKind: PhysicalErrorArgumentRejected,
		},
		{
			name:     "octoprint denies permission",
			writeErr: &octoprint.Error{Kind: octoprint.ErrorHTTPStatus, StatusCode: http.StatusForbidden},
			wantKind: PhysicalErrorPermissionDenied,
		},
		{
			name:     "octoprint rejects the credential",
			writeErr: &octoprint.Error{Kind: octoprint.ErrorHTTPStatus, StatusCode: http.StatusUnauthorized},
			wantKind: PhysicalErrorPermissionDenied,
		},
		{
			name:     "octoprint reports a conflicting resource",
			writeErr: &octoprint.Error{Kind: octoprint.ErrorHTTPStatus, StatusCode: http.StatusConflict},
			wantKind: PhysicalErrorResourceConflict,
		},
		{
			name:        "timeout leaves the result unknown",
			writeErr:    &octoprint.Error{Kind: octoprint.ErrorTimeout},
			wantKind:    PhysicalErrorUnknownResult,
			wantOutcome: PhysicalUnknown,
		},
		{
			name:        "transport failure leaves the result unknown",
			writeErr:    &octoprint.Error{Kind: octoprint.ErrorTransport},
			wantKind:    PhysicalErrorUnknownResult,
			wantOutcome: PhysicalUnknown,
		},
		{
			name:        "canceled request leaves the result unknown",
			writeErr:    &octoprint.Error{Kind: octoprint.ErrorCanceled},
			wantKind:    PhysicalErrorUnknownResult,
			wantOutcome: PhysicalUnknown,
		},
		{
			name:        "unexpected server status leaves the result unknown",
			writeErr:    &octoprint.Error{Kind: octoprint.ErrorHTTPStatus, StatusCode: http.StatusBadGateway},
			wantKind:    PhysicalErrorUnknownResult,
			wantOutcome: PhysicalUnknown,
		},
		{
			name:     "client rejects invalid arguments",
			writeErr: &octoprint.Error{Kind: octoprint.ErrorInvalidArgument},
			wantKind: PhysicalErrorInvalidArgument,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			mock, coordinator := newTestPhysicalCoordinator(testPhysicalLimits())
			mock.mutate(func(mock *physicalMock) { mock.writeErr = test.writeErr })

			outcome, err := coordinator.Jog(context.Background(), testJog())

			requirePhysicalKind(t, err, test.wantKind)
			if outcome != test.wantOutcome {
				t.Errorf("outcome = %q, want %q", outcome, test.wantOutcome)
			}
			if writes := mock.writeNames(); len(writes) != 1 {
				t.Fatalf("writes = %v, want exactly one attempt", writes)
			}
		})
	}
}

func TestPhysicalErrorIsSanitized(t *testing.T) {
	mock, coordinator := newTestPhysicalCoordinator(testPhysicalLimits())
	mock.mutate(func(mock *physicalMock) {
		mock.writeErr = &octoprint.Error{Kind: octoprint.ErrorHTTPStatus, StatusCode: http.StatusForbidden}
	})

	_, err := coordinator.Jog(context.Background(), testJog())

	if err == nil {
		t.Fatal("expected an error")
	}
	if err.Error() != "physical: permission_denied" {
		t.Errorf("error = %q, want sanitized category only", err.Error())
	}
}

func TestPhysicalReadsStateBeforeEveryWrite(t *testing.T) {
	mock, coordinator := newTestPhysicalCoordinator(testPhysicalLimits())

	if _, err := coordinator.Jog(context.Background(), testJog()); err != nil {
		t.Fatalf("first jog: %v", err)
	}
	mock.setPrinting("Printing")

	_, err := coordinator.Jog(context.Background(), testJog())

	requirePhysicalKind(t, err, PhysicalErrorStateIncompatible)
	if writes := mock.writeNames(); len(writes) != 1 {
		t.Fatalf("writes = %v, want the first jog only", writes)
	}
	if reads := mock.readTotal(); reads != 6 {
		t.Fatalf("reads = %d, want connection, printer and job read for each attempt", reads)
	}
}

func TestPhysicalOperationsAreSerialized(t *testing.T) {
	mock, coordinator := newTestPhysicalCoordinator(testPhysicalLimits())
	mock.mutate(func(mock *physicalMock) {
		mock.writeHook = func(string) { time.Sleep(time.Millisecond) }
	})

	const attempts = 8
	var group sync.WaitGroup
	for range attempts {
		group.Add(2)
		go func() {
			defer group.Done()
			_, _ = coordinator.Jog(context.Background(), testJog())
		}()
		go func() {
			defer group.Done()
			_, _ = coordinator.Home(context.Background(), []octoprint.PrintheadAxis{octoprint.PrintheadAxisX})
		}()
	}
	group.Wait()

	if max := mock.maxConcurrentWrites(); max != 1 {
		t.Fatalf("concurrent writes = %d, want 1", max)
	}
	if writes := mock.writeNames(); len(writes) != attempts*2 {
		t.Fatalf("writes = %d, want %d", len(writes), attempts*2)
	}
	if reads := mock.readTotal(); reads != attempts*2*3 {
		t.Fatalf("reads = %d, want fresh state for every attempt", reads)
	}
}

func TestPhysicalReadsHappenInsideSerializedSection(t *testing.T) {
	mock, coordinator := newTestPhysicalCoordinator(testPhysicalLimits())
	writeStarted := make(chan struct{})
	releaseWrite := make(chan struct{})
	var start sync.Once
	mock.mutate(func(mock *physicalMock) {
		mock.writeHook = func(name string) {
			if name != "jog" {
				return
			}
			start.Do(func() { close(writeStarted) })
			<-releaseWrite
		}
	})

	firstDone := make(chan struct{})
	go func() {
		defer close(firstDone)
		_, _ = coordinator.Jog(context.Background(), testJog())
	}()
	<-writeStarted

	secondDone := make(chan struct{})
	go func() {
		defer close(secondDone)
		_, _ = coordinator.Home(context.Background(), []octoprint.PrintheadAxis{octoprint.PrintheadAxisX})
	}()
	select {
	case <-secondDone:
		t.Fatal("second operation completed before the first write was released")
	case <-time.After(50 * time.Millisecond):
	}
	if reads := mock.readTotal(); reads != 3 {
		t.Fatalf("reads = %d while the first write is in progress, want only the first operation reads", reads)
	}

	close(releaseWrite)
	<-firstDone
	<-secondDone
}
