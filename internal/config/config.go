package config

import (
	"encoding/json"
	"errors"
	"io"
	"math"
	"net"
	"strconv"
	"strings"
	"time"
	"unicode"
)

const (
	ListenAddressVariable              = "TUPI_MCP_LISTEN_ADDRESS"
	AllowNonLoopbackVariable           = "TUPI_MCP_ALLOW_NON_LOOPBACK"
	AllowedHostsVariable               = "TUPI_MCP_ALLOWED_HOSTS"
	AllowedOriginsVariable             = "TUPI_MCP_ALLOWED_ORIGINS"
	MaxRequestBytesVariable            = "TUPI_MCP_MAX_REQUEST_BODY_BYTES"
	EnabledToolsVariable               = "TUPI_MCP_ENABLED_TOOLS"
	OctoPrintURLVariable               = "TUPI_OCTOPRINT_BASE_URL"
	OctoPrintAPIKeyVariable            = "TUPI_OCTOPRINT_API_KEY"
	OctoPrintAllowInsecureHTTPVariable = "TUPI_OCTOPRINT_ALLOW_INSECURE_HTTP"
	OctoPrintTimeoutVariable           = "TUPI_OCTOPRINT_TIMEOUT"
	OctoPrintMaxResponseVariable       = "TUPI_OCTOPRINT_MAX_RESPONSE_BYTES"
	PhysicalLimitsVariable             = "TUPI_MCP_PHYSICAL_LIMITS"

	DefaultListenAddress        = "127.0.0.1:8080"
	DefaultMaxRequestBodyBytes  = 18 << 20
	DefaultOctoPrintTimeout     = 10 * time.Second
	DefaultMaxResponseBodyBytes = 1 << 20
)

type Error struct {
	Message string
}

func (e *Error) Error() string {
	return "config: " + e.Message
}

type Config struct {
	ListenAddress              string
	AllowedHosts               []string
	AllowedOrigins             []string
	MaxRequestBodyBytes        int64
	EnabledTools               []string
	OctoPrintBaseURL           string
	OctoPrintAPIKey            string
	OctoPrintAllowInsecureHTTP bool
	OctoPrintTimeout           time.Duration
	OctoPrintMaxBodyBytes      int64
	PhysicalLimits             PhysicalLimits
}

type LimitRange struct {
	Min float64
	Max float64
}

type AxisLimits struct {
	Travel LimitRange
	Speed  LimitRange
}

type HeaterLimits struct {
	Temperature LimitRange
	Offset      LimitRange
}

type ExtrusionLimits struct {
	Length             LimitRange
	Speed              LimitRange
	MinimumTemperature float64
}

type PhysicalDomains struct {
	Axes      bool
	Heaters   bool
	Extrusion bool
}

type PhysicalLimits struct {
	Axes       map[string]AxisLimits
	Heaters    map[string]HeaterLimits
	Extrusion  ExtrusionLimits
	Configured PhysicalDomains
}

type physicalLimitsDocument struct {
	Axes      map[string]axisLimitsDocument   `json:"axes"`
	Heaters   map[string]heaterLimitsDocument `json:"heaters"`
	Extrusion *extrusionLimitsDocument        `json:"extrusion"`
}

type axisLimitsDocument struct {
	Travel *limitRangeDocument `json:"travel"`
	Speed  *limitRangeDocument `json:"speed"`
}

type heaterLimitsDocument struct {
	Temperature *limitRangeDocument `json:"temperature"`
	Offset      *limitRangeDocument `json:"offset"`
}

type extrusionLimitsDocument struct {
	Length             *limitRangeDocument `json:"length"`
	Speed              *limitRangeDocument `json:"speed"`
	MinimumTemperature *float64            `json:"minimumTemperature"`
}

type limitRangeDocument struct {
	Min *float64 `json:"min"`
	Max *float64 `json:"max"`
}

var physicalAxisNames = [...]string{"x", "y", "z"}

type lookupFunc func(string) (string, bool)

func Load(lookup lookupFunc) (*Config, error) {
	listenAddress, allowedHosts, err := loadListener(lookup)
	if err != nil {
		return nil, err
	}
	allowedOrigins, err := loadAllowedOrigins(lookup)
	if err != nil {
		return nil, err
	}
	maxRequestBodyBytes, err := loadPositiveInt(lookup, MaxRequestBytesVariable, DefaultMaxRequestBodyBytes)
	if err != nil {
		return nil, err
	}
	enabledTools, err := loadEnabledTools(lookup)
	if err != nil {
		return nil, err
	}
	physicalLimits := loadPhysicalLimits(lookup)
	baseURL, err := loadRequired(lookup, OctoPrintURLVariable)
	if err != nil {
		return nil, err
	}
	apiKey, err := loadRequired(lookup, OctoPrintAPIKeyVariable)
	if err != nil {
		return nil, err
	}
	allowInsecureHTTP, err := loadBool(lookup, OctoPrintAllowInsecureHTTPVariable)
	if err != nil {
		return nil, err
	}
	timeout, err := loadPositiveDuration(lookup, OctoPrintTimeoutVariable, DefaultOctoPrintTimeout)
	if err != nil {
		return nil, err
	}
	maxResponseBytes, err := loadPositiveInt(lookup, OctoPrintMaxResponseVariable, DefaultMaxResponseBodyBytes)
	if err != nil {
		return nil, err
	}
	return &Config{
		ListenAddress:              listenAddress,
		AllowedHosts:               allowedHosts,
		AllowedOrigins:             allowedOrigins,
		MaxRequestBodyBytes:        maxRequestBodyBytes,
		EnabledTools:               enabledTools,
		OctoPrintBaseURL:           baseURL,
		OctoPrintAPIKey:            apiKey,
		OctoPrintAllowInsecureHTTP: allowInsecureHTTP,
		OctoPrintTimeout:           timeout,
		OctoPrintMaxBodyBytes:      maxResponseBytes,
		PhysicalLimits:             physicalLimits,
	}, nil
}

func loadListener(lookup lookupFunc) (string, []string, error) {
	listenAddress := loadOptional(lookup, ListenAddressVariable, DefaultListenAddress)
	host, port, err := net.SplitHostPort(listenAddress)
	if err != nil {
		return "", nil, &Error{Message: ListenAddressVariable + " must be a host and port"}
	}
	portNumber, err := strconv.Atoi(port)
	if err != nil || portNumber < 1 || portNumber > 65535 {
		return "", nil, &Error{Message: ListenAddressVariable + " must use a valid port"}
	}
	allowNonLoopback, err := loadBool(lookup, AllowNonLoopbackVariable)
	if err != nil {
		return "", nil, err
	}
	if !isLoopbackHost(host) && !allowNonLoopback {
		return "", nil, &Error{Message: ListenAddressVariable + " listens outside loopback; set " + AllowNonLoopbackVariable + "=true to allow it"}
	}
	hosts, err := loadAllowedHosts(lookup, host, port)
	if err != nil {
		return "", nil, err
	}
	return listenAddress, hosts, nil
}

func loadAllowedHosts(lookup lookupFunc, host, port string) ([]string, error) {
	value, ok := lookup(AllowedHostsVariable)
	if !ok || strings.TrimSpace(value) == "" {
		if !isLoopbackHost(host) {
			return nil, &Error{Message: AllowedHostsVariable + " is required when listening outside loopback"}
		}
		return defaultAllowedHosts(host, port), nil
	}
	hosts, ok := splitList(value)
	if !ok {
		return nil, &Error{Message: AllowedHostsVariable + " must not contain empty entries"}
	}
	return hosts, nil
}

func defaultAllowedHosts(host, port string) []string {
	address := net.JoinHostPort(host, port)
	if host == "localhost" {
		return []string{address, net.JoinHostPort("127.0.0.1", port), net.JoinHostPort("::1", port)}
	}
	return []string{address, net.JoinHostPort("localhost", port)}
}

func loadAllowedOrigins(lookup lookupFunc) ([]string, error) {
	value, ok := lookup(AllowedOriginsVariable)
	if !ok || strings.TrimSpace(value) == "" {
		return nil, nil
	}
	entries, ok := splitList(value)
	if !ok {
		return nil, &Error{Message: AllowedOriginsVariable + " must not contain empty entries"}
	}
	return entries, nil
}

func loadEnabledTools(lookup lookupFunc) ([]string, error) {
	value, ok := lookup(EnabledToolsVariable)
	if !ok || strings.TrimSpace(value) == "" {
		return nil, nil
	}
	entries, ok := splitList(value)
	if !ok {
		return nil, &Error{Message: EnabledToolsVariable + " must not contain empty entries"}
	}
	return entries, nil
}

func isLoopbackHost(host string) bool {
	if host == "" {
		return false
	}
	if strings.EqualFold(host, "localhost") {
		return true
	}
	address := net.ParseIP(host)
	return address != nil && address.IsLoopback()
}

func splitList(value string) ([]string, bool) {
	parts := strings.Split(value, ",")
	entries := make([]string, 0, len(parts))
	for _, part := range parts {
		entry := strings.TrimSpace(part)
		if entry == "" {
			return nil, false
		}
		entries = append(entries, entry)
	}
	return entries, true
}

func loadRequired(lookup lookupFunc, name string) (string, error) {
	value, ok := lookup(name)
	if !ok || value == "" {
		return "", &Error{Message: name + " is required"}
	}
	if strings.IndexFunc(value, unicode.IsSpace) >= 0 {
		return "", &Error{Message: name + " must not contain whitespace"}
	}
	return value, nil
}

func loadOptional(lookup lookupFunc, name, fallback string) string {
	value, ok := lookup(name)
	if !ok || value == "" {
		return fallback
	}
	return value
}

func loadBool(lookup lookupFunc, name string) (bool, error) {
	value, ok := lookup(name)
	if !ok || value == "" {
		return false, nil
	}
	switch strings.ToLower(value) {
	case "true":
		return true, nil
	case "false":
		return false, nil
	}
	return false, &Error{Message: name + " must be true or false"}
}

func loadPositiveInt(lookup lookupFunc, name string, fallback int64) (int64, error) {
	value, ok := lookup(name)
	if !ok || value == "" {
		return fallback, nil
	}
	number, err := strconv.ParseInt(value, 10, 64)
	if err != nil || number <= 0 {
		return 0, &Error{Message: name + " must be a positive integer"}
	}
	return number, nil
}

func loadPositiveDuration(lookup lookupFunc, name string, fallback time.Duration) (time.Duration, error) {
	value, ok := lookup(name)
	if !ok || value == "" {
		return fallback, nil
	}
	duration, err := time.ParseDuration(value)
	if err != nil || duration <= 0 {
		return 0, &Error{Message: name + " must be a positive duration"}
	}
	return duration, nil
}

func loadPhysicalLimits(lookup lookupFunc) PhysicalLimits {
	value, ok := lookup(PhysicalLimitsVariable)
	if !ok || strings.TrimSpace(value) == "" {
		return PhysicalLimits{}
	}
	document, ok := decodePhysicalLimits(value)
	if !ok {
		return PhysicalLimits{}
	}
	limits := PhysicalLimits{}
	if axes, ok := document.axes(); ok {
		limits.Axes = axes
		limits.Configured.Axes = true
	}
	if heaters, ok := document.heaters(); ok {
		limits.Heaters = heaters
		limits.Configured.Heaters = true
	}
	if extrusion, ok := document.extrusion(); ok {
		limits.Extrusion = extrusion
		limits.Configured.Extrusion = true
	}
	return limits
}

func decodePhysicalLimits(value string) (physicalLimitsDocument, bool) {
	if !hasUniqueJSONKeys(value) {
		return physicalLimitsDocument{}, false
	}
	var document physicalLimitsDocument
	decoder := json.NewDecoder(strings.NewReader(value))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&document); err != nil {
		return physicalLimitsDocument{}, false
	}
	if _, err := decoder.Token(); !errors.Is(err, io.EOF) {
		return physicalLimitsDocument{}, false
	}
	return document, true
}

func hasUniqueJSONKeys(value string) bool {
	decoder := json.NewDecoder(strings.NewReader(value))
	if !consumeUniqueJSONValue(decoder) {
		return false
	}
	_, err := decoder.Token()
	return errors.Is(err, io.EOF)
}

func consumeUniqueJSONValue(decoder *json.Decoder) bool {
	token, err := decoder.Token()
	if err != nil {
		return false
	}
	delimiter, ok := token.(json.Delim)
	if !ok {
		return true
	}
	switch delimiter {
	case '{':
		seen := make(map[string]struct{})
		for decoder.More() {
			keyToken, err := decoder.Token()
			key, ok := keyToken.(string)
			if err != nil || !ok {
				return false
			}
			if _, exists := seen[key]; exists {
				return false
			}
			seen[key] = struct{}{}
			if !consumeUniqueJSONValue(decoder) {
				return false
			}
		}
		closing, err := decoder.Token()
		return err == nil && closing == json.Delim('}')
	case '[':
		for decoder.More() {
			if !consumeUniqueJSONValue(decoder) {
				return false
			}
		}
		closing, err := decoder.Token()
		return err == nil && closing == json.Delim(']')
	default:
		return false
	}
}

func (document physicalLimitsDocument) axes() (map[string]AxisLimits, bool) {
	if len(document.Axes) != len(physicalAxisNames) {
		return nil, false
	}
	axes := make(map[string]AxisLimits, len(physicalAxisNames))
	for _, name := range physicalAxisNames {
		entry, ok := document.Axes[name]
		if !ok {
			return nil, false
		}
		travel, ok := entry.Travel.valid()
		if !ok || travel.Max <= 0 {
			return nil, false
		}
		speed, ok := entry.Speed.valid()
		if !ok || speed.Min <= 0 {
			return nil, false
		}
		axes[name] = AxisLimits{Travel: travel, Speed: speed}
	}
	return axes, true
}

func (document physicalLimitsDocument) heaters() (map[string]HeaterLimits, bool) {
	if len(document.Heaters) == 0 {
		return nil, false
	}
	heaters := make(map[string]HeaterLimits, len(document.Heaters))
	for name, entry := range document.Heaters {
		if !isValidDeviceName(name) {
			return nil, false
		}
		temperature, ok := entry.Temperature.valid()
		if !ok || temperature.Min < 0 || temperature.Max <= 0 {
			return nil, false
		}
		offset, ok := entry.Offset.valid()
		if !ok {
			return nil, false
		}
		heaters[name] = HeaterLimits{Temperature: temperature, Offset: offset}
	}
	return heaters, true
}

func (document physicalLimitsDocument) extrusion() (ExtrusionLimits, bool) {
	if document.Extrusion == nil {
		return ExtrusionLimits{}, false
	}
	length, ok := document.Extrusion.Length.valid()
	if !ok || length.Min >= 0 || length.Max <= 0 {
		return ExtrusionLimits{}, false
	}
	speed, ok := document.Extrusion.Speed.valid()
	if !ok || speed.Min <= 0 {
		return ExtrusionLimits{}, false
	}
	minimumTemperature := document.Extrusion.MinimumTemperature
	if minimumTemperature == nil || !isFinite(*minimumTemperature) || *minimumTemperature <= 0 {
		return ExtrusionLimits{}, false
	}
	return ExtrusionLimits{
		Length:             length,
		Speed:              speed,
		MinimumTemperature: *minimumTemperature,
	}, true
}

func (document *limitRangeDocument) valid() (LimitRange, bool) {
	if document == nil || document.Min == nil || document.Max == nil {
		return LimitRange{}, false
	}
	minimum, maximum := *document.Min, *document.Max
	if !isFinite(minimum) || !isFinite(maximum) || minimum > maximum {
		return LimitRange{}, false
	}
	return LimitRange{Min: minimum, Max: maximum}, true
}

func isFinite(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0)
}

func isValidDeviceName(name string) bool {
	if name == "" {
		return false
	}
	return strings.IndexFunc(name, unicode.IsSpace) < 0
}
