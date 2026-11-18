package config

import (
	"net"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/joaaomanooel/tupi-mcp/internal/security"
)

const (
	TokenVariable                      = "TUPI_MCP_TOKEN"
	ListenAddressVariable              = "TUPI_MCP_LISTEN_ADDRESS"
	AllowNonLoopbackVariable           = "TUPI_MCP_ALLOW_NON_LOOPBACK"
	AllowedHostsVariable               = "TUPI_MCP_ALLOWED_HOSTS"
	AllowedOriginsVariable             = "TUPI_MCP_ALLOWED_ORIGINS"
	MaxRequestBytesVariable            = "TUPI_MCP_MAX_REQUEST_BODY_BYTES"
	OctoPrintURLVariable               = "TUPI_OCTOPRINT_BASE_URL"
	OctoPrintAPIKeyVariable            = "TUPI_OCTOPRINT_API_KEY"
	OctoPrintAllowInsecureHTTPVariable = "TUPI_OCTOPRINT_ALLOW_INSECURE_HTTP"
	OctoPrintTimeoutVariable           = "TUPI_OCTOPRINT_TIMEOUT"
	OctoPrintMaxResponseVariable       = "TUPI_OCTOPRINT_MAX_RESPONSE_BYTES"

	DefaultListenAddress        = "127.0.0.1:8080"
	DefaultMaxRequestBodyBytes  = 10 << 20
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
	MCPToken                   string
	ListenAddress              string
	AllowedHosts               []string
	AllowedOrigins             []string
	MaxRequestBodyBytes        int64
	OctoPrintBaseURL           string
	OctoPrintAPIKey            string
	OctoPrintAllowInsecureHTTP bool
	OctoPrintTimeout           time.Duration
	OctoPrintMaxBodyBytes      int64
}

type lookupFunc func(string) (string, bool)

func Load(lookup lookupFunc) (*Config, error) {
	token, err := loadToken(lookup)
	if err != nil {
		return nil, err
	}
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
		MCPToken:                   token,
		ListenAddress:              listenAddress,
		AllowedHosts:               allowedHosts,
		AllowedOrigins:             allowedOrigins,
		MaxRequestBodyBytes:        maxRequestBodyBytes,
		OctoPrintBaseURL:           baseURL,
		OctoPrintAPIKey:            apiKey,
		OctoPrintAllowInsecureHTTP: allowInsecureHTTP,
		OctoPrintTimeout:           timeout,
		OctoPrintMaxBodyBytes:      maxResponseBytes,
	}, nil
}

func loadToken(lookup lookupFunc) (string, error) {
	token, err := loadRequired(lookup, TokenVariable)
	if err != nil {
		return "", err
	}
	if len(token) < security.MinimumTokenLength {
		return "", &Error{Message: TokenVariable + " must contain at least " + strconv.Itoa(security.MinimumTokenLength) + " bytes"}
	}
	return token, nil
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
