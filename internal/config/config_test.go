package config

import (
	"errors"
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
		TokenVariable:           "test-mcp-token-with-at-least-thirty-two-bytes",
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
}

func TestLoadMissingRequiredValues(t *testing.T) {
	for _, name := range []string{TokenVariable, OctoPrintURLVariable, OctoPrintAPIKeyVariable} {
		t.Run(name, func(t *testing.T) {
			values := validValues()
			delete(values, name)

			_, err := Load(lookupFrom(values))

			assertConfigError(t, err, name)
		})
	}
}

func TestLoadInvalidToken(t *testing.T) {
	for _, token := range []string{" ", " padded", "padded ", "with space", "with\ttab"} {
		t.Run(token, func(t *testing.T) {
			values := validValues()
			values[TokenVariable] = token

			_, err := Load(lookupFrom(values))

			assertConfigError(t, err, TokenVariable)
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

func TestLoadShortToken(t *testing.T) {
	values := validValues()
	values[TokenVariable] = "short-token"

	_, err := Load(lookupFrom(values))

	assertConfigError(t, err, TokenVariable)
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
	} {
		t.Run(tc.name, func(t *testing.T) {
			values := validValues()
			values[tc.variable] = tc.value

			_, err := Load(lookupFrom(values))

			assertConfigError(t, err, tc.variable)
		})
	}
}

func TestLoadIgnoresUnknownValues(t *testing.T) {
	values := validValues()
	values["TUPI_MCP_UNRELATED"] = "ignored"

	settings, err := Load(lookupFrom(values))

	if err != nil || settings.MCPToken != "test-mcp-token-with-at-least-thirty-two-bytes" {
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
