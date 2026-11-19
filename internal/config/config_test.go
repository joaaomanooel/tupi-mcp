package config

import (
	"errors"
	"math"
	"reflect"
	"strings"
	"testing"
	"time"
)

func lookupFrom(values map[string]string) func(string) (string, bool) {
	return func(name string) (string, bool) {
		value, ok := values[name]
		return value, ok
	}
}

func validValues() map[string]string {
	return map[string]string{
		OctoPrintURLVariable:    "http://127.0.0.1:5000",
		OctoPrintAPIKeyVariable: "test-api-key",
	}
}

func TestLoadDefaults(t *testing.T) {
	settings, err := Load(lookupFrom(validValues()))

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if settings.ListenAddress != DefaultListenAddress {
		t.Errorf("listen address = %q, want %q", settings.ListenAddress, DefaultListenAddress)
	}
	expectedHosts := []string{"127.0.0.1:8080", "localhost:8080"}
	if !reflect.DeepEqual(settings.AllowedHosts, expectedHosts) {
		t.Errorf("allowed hosts = %v, want %v", settings.AllowedHosts, expectedHosts)
	}
	if len(settings.AllowedOrigins) != 0 {
		t.Errorf("allowed origins = %v, want none", settings.AllowedOrigins)
	}
	if settings.MaxRequestBodyBytes != DefaultMaxRequestBodyBytes {
		t.Errorf("request body limit = %d, want %d", settings.MaxRequestBodyBytes, DefaultMaxRequestBodyBytes)
	}
	if len(settings.EnabledTools) != 0 {
		t.Errorf("enabled tools = %v, want none", settings.EnabledTools)
	}
	if settings.OctoPrintTimeout != DefaultOctoPrintTimeout {
		t.Errorf("timeout = %v, want %v", settings.OctoPrintTimeout, DefaultOctoPrintTimeout)
	}
	if settings.OctoPrintAllowInsecureHTTP {
		t.Error("insecure HTTP is enabled by default")
	}
	if settings.OctoPrintMaxBodyBytes != DefaultMaxResponseBodyBytes {
		t.Errorf("response limit = %d, want %d", settings.OctoPrintMaxBodyBytes, DefaultMaxResponseBodyBytes)
	}
}

func TestLoadOverrides(t *testing.T) {
	values := validValues()
	values[ListenAddressVariable] = "[::1]:9090"
	values[AllowedHostsVariable] = " mcp.example.internal:9090 , [::1]:9090 "
	values[AllowedOriginsVariable] = "https://client.example.com , http://127.0.0.1:3000"
	values[MaxRequestBytesVariable] = "2048"
	values[OctoPrintTimeoutVariable] = "3s"
	values[OctoPrintMaxResponseVariable] = "4096"
	values[OctoPrintAllowInsecureHTTPVariable] = "true"
	values[EnabledToolsVariable] = " printer_delete_file , printer_start_job "

	settings, err := Load(lookupFrom(values))

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expectedHosts := []string{"mcp.example.internal:9090", "[::1]:9090"}
	if !reflect.DeepEqual(settings.AllowedHosts, expectedHosts) {
		t.Errorf("allowed hosts = %v, want %v", settings.AllowedHosts, expectedHosts)
	}
	expectedOrigins := []string{"https://client.example.com", "http://127.0.0.1:3000"}
	if !reflect.DeepEqual(settings.AllowedOrigins, expectedOrigins) {
		t.Errorf("allowed origins = %v, want %v", settings.AllowedOrigins, expectedOrigins)
	}
	if settings.MaxRequestBodyBytes != 2048 {
		t.Errorf("request body limit = %d, want 2048", settings.MaxRequestBodyBytes)
	}
	if settings.OctoPrintTimeout != 3*time.Second {
		t.Errorf("timeout = %v, want 3s", settings.OctoPrintTimeout)
	}
	if settings.OctoPrintMaxBodyBytes != 4096 {
		t.Errorf("response limit = %d, want 4096", settings.OctoPrintMaxBodyBytes)
	}
	if !settings.OctoPrintAllowInsecureHTTP {
		t.Error("insecure HTTP = false, want true")
	}
	expectedTools := []string{"printer_delete_file", "printer_start_job"}
	if !reflect.DeepEqual(settings.EnabledTools, expectedTools) {
		t.Errorf("enabled tools = %v, want %v", settings.EnabledTools, expectedTools)
	}
}

func TestLoadMissingRequiredValues(t *testing.T) {
	for _, name := range []string{OctoPrintURLVariable, OctoPrintAPIKeyVariable} {
		t.Run(name, func(t *testing.T) {
			values := validValues()
			delete(values, name)

			_, err := Load(lookupFrom(values))

			assertConfigError(t, err, name)
		})
	}
}

func TestLoadRequiredValuesRejectWhitespace(t *testing.T) {
	for _, name := range []string{OctoPrintURLVariable, OctoPrintAPIKeyVariable} {
		t.Run(name, func(t *testing.T) {
			values := validValues()
			values[name] = "invalid\nvalue"

			_, err := Load(lookupFrom(values))

			assertConfigError(t, err, name)
		})
	}
}

func TestLoadListener(t *testing.T) {
	for _, tc := range []struct {
		name    string
		listen  string
		allow   string
		hosts   string
		wantErr string
	}{
		{"loopback localhost", "localhost:8080", "", "", ""},
		{"loopback ipv6", "[::1]:8080", "", "", ""},
		{"non loopback rejected", "0.0.0.0:8080", "", "", ListenAddressVariable},
		{"non loopback host rejected", "192.168.1.20:8080", "false", "", ListenAddressVariable},
		{"non loopback without hosts", "0.0.0.0:8080", "true", "", AllowedHostsVariable},
		{"non loopback allowed", "0.0.0.0:8080", "true", "mcp.example.test:8080", ""},
		{"missing port", "127.0.0.1", "false", "", ListenAddressVariable},
		{"invalid port", "127.0.0.1:abc", "false", "", ListenAddressVariable},
		{"out of range port", "127.0.0.1:70000", "false", "", ListenAddressVariable},
		{"zero port", "127.0.0.1:0", "false", "", ListenAddressVariable},
		{"invalid allow value", "0.0.0.0:8080", "yes", "", AllowNonLoopbackVariable},
		{"empty host entry", "127.0.0.1:8080", "false", "127.0.0.1:8080,,localhost:8080", AllowedHostsVariable},
	} {
		t.Run(tc.name, func(t *testing.T) {
			values := validValues()
			values[ListenAddressVariable] = tc.listen
			if tc.allow != "" {
				values[AllowNonLoopbackVariable] = tc.allow
			}
			if tc.hosts != "" {
				values[AllowedHostsVariable] = tc.hosts
			}

			_, err := Load(lookupFrom(values))

			if tc.wantErr == "" && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tc.wantErr != "" {
				assertConfigError(t, err, tc.wantErr)
			}
		})
	}
}

func TestLoadLocalhostDefaultsToLoopbackHostForms(t *testing.T) {
	values := validValues()
	values[ListenAddressVariable] = "localhost:9090"

	settings, err := Load(lookupFrom(values))

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expectedHosts := []string{"localhost:9090", "127.0.0.1:9090", "[::1]:9090"}
	if !reflect.DeepEqual(settings.AllowedHosts, expectedHosts) {
		t.Fatalf("allowed hosts = %v, want %v", settings.AllowedHosts, expectedHosts)
	}
}

func TestLoadInvalidLimits(t *testing.T) {
	for _, tc := range []struct {
		name, variable, value string
	}{
		{"zero request limit", MaxRequestBytesVariable, "0"},
		{"negative request limit", MaxRequestBytesVariable, "-1"},
		{"malformed request limit", MaxRequestBytesVariable, "many"},
		{"zero response limit", OctoPrintMaxResponseVariable, "0"},
		{"malformed timeout", OctoPrintTimeoutVariable, "soon"},
		{"zero timeout", OctoPrintTimeoutVariable, "0s"},
		{"negative timeout", OctoPrintTimeoutVariable, "-5s"},
		{"empty origin entry", AllowedOriginsVariable, "https://client.example.com,,"},
		{"empty tool entry", EnabledToolsVariable, "printer_start_job,,"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			values := validValues()
			values[tc.variable] = tc.value

			_, err := Load(lookupFrom(values))

			assertConfigError(t, err, tc.variable)
		})
	}
}

func TestLoadEmptyEnabledTools(t *testing.T) {
	values := validValues()
	values[EnabledToolsVariable] = "   "

	settings, err := Load(lookupFrom(values))

	if err != nil || len(settings.EnabledTools) != 0 {
		t.Fatalf("unexpected enabled tools: %v %v", settings.EnabledTools, err)
	}
}

func TestLoadIgnoresUnknownValues(t *testing.T) {
	values := validValues()
	values["TUPI_MCP_UNRELATED"] = "ignored"

	settings, err := Load(lookupFrom(values))

	if err != nil || settings.OctoPrintBaseURL != "http://127.0.0.1:5000" {
		t.Fatalf("unexpected result: %+v %v", settings, err)
	}
}

func assertConfigError(t *testing.T, err error, variable string) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected configuration error mentioning %s", variable)
	}
	var configurationError *Error
	if !errors.As(err, &configurationError) {
		t.Fatalf("expected *Error, got %T", err)
	}
	if !strings.Contains(err.Error(), variable) {
		t.Errorf("error %q does not mention %s", err, variable)
	}
}

const (
	validAxisLimitsJSON      = `{"travel":{"min":-100,"max":100},"speed":{"min":1,"max":6000}}`
	validHeaterLimitsJSON    = `{"temperature":{"min":0,"max":250},"offset":{"min":-10,"max":10}}`
	validExtrusionLimitsJSON = `{"length":{"min":-5,"max":50},"speed":{"min":60,"max":300},"minimumTemperature":180}`
)

func validPhysicalLimitsJSON(heaters string) string {
	return `{"axes":{"x":` + validAxisLimitsJSON + `,"y":` + validAxisLimitsJSON + `,"z":` + validAxisLimitsJSON + `},` +
		`"heaters":{` + heaters + `},"extrusion":` + validExtrusionLimitsJSON + `}`
}

func TestLoadMissingPhysicalLimits(t *testing.T) {
	settings, err := Load(lookupFrom(validValues()))

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if settings.PhysicalLimits.Configured != (PhysicalDomains{}) {
		t.Errorf("configured domains = %+v, want none", settings.PhysicalLimits.Configured)
	}
	if len(settings.PhysicalLimits.Axes) != 0 || len(settings.PhysicalLimits.Heaters) != 0 {
		t.Errorf("physical limits = %+v, want none", settings.PhysicalLimits)
	}
	if settings.PhysicalLimits.Extrusion != (ExtrusionLimits{}) {
		t.Errorf("extrusion limits = %+v, want none", settings.PhysicalLimits.Extrusion)
	}
	if len(settings.EnabledTools) != 0 {
		t.Errorf("enabled tools = %v, want none", settings.EnabledTools)
	}
}

func TestLoadPhysicalLimits(t *testing.T) {
	values := validValues()
	values[PhysicalLimitsVariable] = validPhysicalLimitsJSON(`"bed":` + validHeaterLimitsJSON + `,"tool0":` + validHeaterLimitsJSON)

	settings, err := Load(lookupFrom(values))

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expectedDomains := PhysicalDomains{Axes: true, Heaters: true, Extrusion: true}
	if settings.PhysicalLimits.Configured != expectedDomains {
		t.Errorf("configured domains = %+v, want %+v", settings.PhysicalLimits.Configured, expectedDomains)
	}
	if len(settings.PhysicalLimits.Axes) != len(physicalAxisNames) {
		t.Fatalf("axes = %v, want %d entries", settings.PhysicalLimits.Axes, len(physicalAxisNames))
	}
	for _, name := range physicalAxisNames {
		expectedAxis := AxisLimits{Travel: LimitRange{Min: -100, Max: 100}, Speed: LimitRange{Min: 1, Max: 6000}}
		if settings.PhysicalLimits.Axes[name] != expectedAxis {
			t.Errorf("axis %s = %+v, want %+v", name, settings.PhysicalLimits.Axes[name], expectedAxis)
		}
	}
	expectedHeater := HeaterLimits{Temperature: LimitRange{Min: 0, Max: 250}, Offset: LimitRange{Min: -10, Max: 10}}
	if len(settings.PhysicalLimits.Heaters) != 2 {
		t.Fatalf("heaters = %v, want 2 entries", settings.PhysicalLimits.Heaters)
	}
	for _, name := range []string{"bed", "tool0"} {
		if settings.PhysicalLimits.Heaters[name] != expectedHeater {
			t.Errorf("heater %s = %+v, want %+v", name, settings.PhysicalLimits.Heaters[name], expectedHeater)
		}
	}
	expectedExtrusion := ExtrusionLimits{
		Length:             LimitRange{Min: -5, Max: 50},
		Speed:              LimitRange{Min: 60, Max: 300},
		MinimumTemperature: 180,
	}
	if settings.PhysicalLimits.Extrusion != expectedExtrusion {
		t.Errorf("extrusion = %+v, want %+v", settings.PhysicalLimits.Extrusion, expectedExtrusion)
	}
	if len(settings.EnabledTools) != 0 {
		t.Errorf("enabled tools = %v, want none", settings.EnabledTools)
	}
}

func TestLoadPhysicalLimitsPartialDomains(t *testing.T) {
	for _, tc := range []struct {
		name  string
		value string
		want  PhysicalDomains
	}{
		{
			"only axes",
			`{"axes":{"x":` + validAxisLimitsJSON + `,"y":` + validAxisLimitsJSON + `,"z":` + validAxisLimitsJSON + `}}`,
			PhysicalDomains{Axes: true},
		},
		{
			"axes missing one axis",
			`{"axes":{"x":` + validAxisLimitsJSON + `,"y":` + validAxisLimitsJSON + `}}`,
			PhysicalDomains{},
		},
		{
			"empty axes",
			`{"axes":{}}`,
			PhysicalDomains{},
		},
		{
			"only heaters",
			`{"heaters":{"bed":` + validHeaterLimitsJSON + `}}`,
			PhysicalDomains{Heaters: true},
		},
		{
			"heater missing offset",
			`{"heaters":{"bed":{"temperature":{"min":0,"max":250}}}}`,
			PhysicalDomains{},
		},
		{
			"empty heaters",
			`{"heaters":{}}`,
			PhysicalDomains{},
		},
		{
			"only extrusion",
			`{"extrusion":` + validExtrusionLimitsJSON + `}`,
			PhysicalDomains{Extrusion: true},
		},
		{
			"extrusion missing minimum temperature",
			`{"extrusion":{"length":{"min":-5,"max":50},"speed":{"min":60,"max":300}}}`,
			PhysicalDomains{},
		},
		{
			"axes and heaters",
			`{"axes":{"x":` + validAxisLimitsJSON + `,"y":` + validAxisLimitsJSON + `,"z":` + validAxisLimitsJSON + `},` +
				`"heaters":{"bed":` + validHeaterLimitsJSON + `}}`,
			PhysicalDomains{Axes: true, Heaters: true},
		},
		{
			"empty document",
			`{}`,
			PhysicalDomains{},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			values := validValues()
			values[PhysicalLimitsVariable] = tc.value

			settings, err := Load(lookupFrom(values))

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if settings.PhysicalLimits.Configured != tc.want {
				t.Errorf("configured domains = %+v, want %+v", settings.PhysicalLimits.Configured, tc.want)
			}
			if (len(settings.PhysicalLimits.Axes) > 0) != tc.want.Axes {
				t.Errorf("axes = %v, want configured %v", settings.PhysicalLimits.Axes, tc.want.Axes)
			}
			if (len(settings.PhysicalLimits.Heaters) > 0) != tc.want.Heaters {
				t.Errorf("heaters = %v, want configured %v", settings.PhysicalLimits.Heaters, tc.want.Heaters)
			}
			if (settings.PhysicalLimits.Extrusion != ExtrusionLimits{}) != tc.want.Extrusion {
				t.Errorf("extrusion = %+v, want configured %v", settings.PhysicalLimits.Extrusion, tc.want.Extrusion)
			}
			if len(settings.EnabledTools) != 0 {
				t.Errorf("enabled tools = %v, want none", settings.EnabledTools)
			}
		})
	}
}

func TestLoadPhysicalLimitsInvalidDocumentDisablesAllDomains(t *testing.T) {
	for _, tc := range []struct {
		name  string
		value string
	}{
		{"malformed JSON", `{"axes":`},
		{"unknown top level field", `{"axes":{"x":` + validAxisLimitsJSON + `,"y":` + validAxisLimitsJSON + `,"z":` + validAxisLimitsJSON + `},` +
			`"heaters":{"bed":` + validHeaterLimitsJSON + `},"extrusion":` + validExtrusionLimitsJSON + `,"extra":true}`},
		{"trailing document", validPhysicalLimitsJSON(`"bed":`+validHeaterLimitsJSON) + `{}`},
		{"array document", `[]`},
		{"duplicate top level field", `{"axes":{},"axes":{"x":` + validAxisLimitsJSON + `,"y":` + validAxisLimitsJSON + `,"z":` + validAxisLimitsJSON + `}}`},
		{"duplicate nested field", `{"axes":{"x":{"travel":{"min":-100,"min":-50,"max":100},"speed":{"min":1,"max":6000}},"y":` + validAxisLimitsJSON + `,"z":` + validAxisLimitsJSON + `}}`},
		{"axes as list", `{"axes":[]}`},
		{"number out of float range", `{"axes":{"x":{"travel":{"min":1e400,"max":1},"speed":{"min":1,"max":2}}}}`},
		{"unknown axis field", `{"axes":{"x":{"travel":{"min":-1,"max":1},"speed":{"min":1,"max":2},"feedrate":1},` +
			`"y":` + validAxisLimitsJSON + `,"z":` + validAxisLimitsJSON + `}}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			values := validValues()
			values[PhysicalLimitsVariable] = tc.value

			settings, err := Load(lookupFrom(values))

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if settings.PhysicalLimits.Configured != (PhysicalDomains{}) {
				t.Errorf("configured domains = %+v, want none", settings.PhysicalLimits.Configured)
			}
			if len(settings.PhysicalLimits.Axes) != 0 || len(settings.PhysicalLimits.Heaters) != 0 {
				t.Errorf("physical limits = %+v, want none", settings.PhysicalLimits)
			}
			if settings.OctoPrintBaseURL != "http://127.0.0.1:5000" {
				t.Errorf("base URL = %q, want the configured value", settings.OctoPrintBaseURL)
			}
		})
	}
}

func TestLoadPhysicalLimitsRejectsInvalidRanges(t *testing.T) {
	axes := func(x, y, z string) string {
		return `{"axes":{"x":` + x + `,"y":` + y + `,"z":` + z + `}}`
	}
	for _, tc := range []struct {
		name  string
		value string
	}{
		{"travel minimum above maximum", `{"axes":{"x":{"travel":{"min":100,"max":-100},"speed":{"min":1,"max":6000}},"y":` +
			validAxisLimitsJSON + `,"z":` + validAxisLimitsJSON + `}}`},
		{"travel maximum not positive", axes(`{"travel":{"min":-100,"max":0},"speed":{"min":1,"max":6000}}`, validAxisLimitsJSON, validAxisLimitsJSON)},
		{"missing travel", axes(`{"speed":{"min":1,"max":6000}}`, validAxisLimitsJSON, validAxisLimitsJSON)},
		{"zero speed minimum", axes(`{"travel":{"min":-100,"max":100},"speed":{"min":0,"max":6000}}`, validAxisLimitsJSON, validAxisLimitsJSON)},
		{"speed minimum above maximum", axes(`{"travel":{"min":-100,"max":100},"speed":{"min":6000,"max":1}}`, validAxisLimitsJSON, validAxisLimitsJSON)},
		{"unknown axis name", `{"axes":{"x":` + validAxisLimitsJSON + `,"y":` + validAxisLimitsJSON + `,"z":` + validAxisLimitsJSON +
			`,"e":` + validAxisLimitsJSON + `}}`},
		{"negative temperature minimum", `{"heaters":{"bed":{"temperature":{"min":-1,"max":250},"offset":{"min":-5,"max":5}}}}`},
		{"temperature maximum not positive", `{"heaters":{"bed":{"temperature":{"min":0,"max":0},"offset":{"min":-5,"max":5}}}}`},
		{"offset minimum above maximum", `{"heaters":{"bed":{"temperature":{"min":0,"max":250},"offset":{"min":5,"max":-5}}}}`},
		{"heater name with whitespace", `{"heaters":{" tool0":` + validHeaterLimitsJSON + `}}`},
		{"extrusion length maximum not positive", `{"extrusion":{"length":{"min":-5,"max":0},"speed":{"min":60,"max":300},"minimumTemperature":180}}`},
		{"extrusion length cannot retract", `{"extrusion":{"length":{"min":0,"max":50},"speed":{"min":60,"max":300},"minimumTemperature":180}}`},
		{"zero extrusion speed minimum", `{"extrusion":{"length":{"min":-5,"max":50},"speed":{"min":0,"max":300},"minimumTemperature":180}}`},
		{"zero extrusion minimum temperature", `{"extrusion":{"length":{"min":-5,"max":50},"speed":{"min":60,"max":300},"minimumTemperature":0}}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			values := validValues()
			values[PhysicalLimitsVariable] = tc.value

			settings, err := Load(lookupFrom(values))

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if settings.PhysicalLimits.Configured != (PhysicalDomains{}) {
				t.Errorf("configured domains = %+v, want none", settings.PhysicalLimits.Configured)
			}
			if len(settings.PhysicalLimits.Axes) != 0 || len(settings.PhysicalLimits.Heaters) != 0 {
				t.Errorf("physical limits = %+v, want none", settings.PhysicalLimits)
			}
		})
	}
}

func TestLimitRangeDocumentRejectsNonFiniteValues(t *testing.T) {
	for _, tc := range []struct {
		name    string
		minimum float64
		maximum float64
	}{
		{"not a number", math.NaN(), 1},
		{"positive infinity", 0, math.Inf(1)},
		{"negative infinity", math.Inf(-1), 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			document := &limitRangeDocument{Min: &tc.minimum, Max: &tc.maximum}

			_, ok := document.valid()

			if ok {
				t.Fatalf("expected invalid range for min=%v max=%v", tc.minimum, tc.maximum)
			}
		})
	}
}
