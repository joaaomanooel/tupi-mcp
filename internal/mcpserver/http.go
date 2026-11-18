package mcpserver

import (
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const (
	sessionIdleTimeout = 15 * time.Minute
	mcpPath            = "/mcp"
)

type Options struct {
	Server              *mcp.Server
	AllowedHosts        []string
	AllowedOrigins      []string
	MaxRequestBodyBytes int64
}

func NewHandler(options Options) (http.Handler, error) {
	if err := options.validate(); err != nil {
		return nil, err
	}
	origins, err := canonicalOrigins(options.AllowedOrigins)
	if err != nil {
		return nil, err
	}
	streamable := mcp.NewStreamableHTTPHandler(
		func(*http.Request) *mcp.Server { return options.Server },
		&mcp.StreamableHTTPOptions{
			MaxRequestBodyBytes: options.MaxRequestBodyBytes,
			SessionTimeout:      sessionIdleTimeout,
		},
	)
	mux := http.NewServeMux()
	mux.Handle(mcpPath, streamable)
	return guard{
		allowedHosts:   options.AllowedHosts,
		allowedOrigins: origins,
		next:           mux,
	}, nil
}

func (o Options) validate() error {
	if o.Server == nil {
		return errors.New("mcpserver: MCP server is required")
	}
	if len(o.AllowedHosts) == 0 {
		return errors.New("mcpserver: at least one allowed host is required")
	}
	for _, host := range o.AllowedHosts {
		if !isAllowedHost(host) {
			return errors.New("mcpserver: allowed hosts must be exact host values without schemes, paths or wildcards")
		}
	}
	if o.MaxRequestBodyBytes <= 0 {
		return errors.New("mcpserver: request body limit must be positive")
	}
	return nil
}

func isAllowedHost(value string) bool {
	if value == "" || strings.ContainsAny(value, "*/\\ \t\r\n") {
		return false
	}
	host, port, err := net.SplitHostPort(value)
	if err != nil {
		if strings.HasPrefix(value, "[") && strings.HasSuffix(value, "]") {
			return net.ParseIP(value[1:len(value)-1]) != nil
		}
		return !strings.Contains(value, ":") && isAllowedHostname(value)
	}
	number, err := strconv.Atoi(port)
	return isAllowedHostname(host) && err == nil && number > 0 && number <= 65535
}

func isAllowedHostname(value string) bool {
	if net.ParseIP(value) != nil {
		return true
	}
	if value == "" || len(value) > 253 {
		return false
	}
	value = strings.TrimSuffix(value, ".")
	if value == "" {
		return false
	}
	for _, label := range strings.Split(value, ".") {
		if len(label) == 0 || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return false
		}
		for _, character := range label {
			if (character < 'a' || character > 'z') && (character < 'A' || character > 'Z') && (character < '0' || character > '9') && character != '-' {
				return false
			}
		}
	}
	return true
}

func canonicalOrigins(values []string) ([]string, error) {
	origins := make([]string, 0, len(values))
	for _, value := range values {
		origin, ok := canonicalOrigin(value)
		if !ok {
			return nil, errors.New("mcpserver: allowed origins must be exact http or https origins without paths or wildcards")
		}
		origins = append(origins, origin)
	}
	return origins, nil
}

type guard struct {
	allowedHosts   []string
	allowedOrigins []string
	next           http.Handler
}

func (g guard) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if !g.hostAllowed(r) || !g.originAllowed(r) {
		writeError(w, http.StatusForbidden, "forbidden")
		return
	}
	g.next.ServeHTTP(w, r)
}

func (g guard) hostAllowed(r *http.Request) bool {
	if r.Host == "" {
		return false
	}
	for _, allowed := range g.allowedHosts {
		if strings.EqualFold(r.Host, allowed) {
			return true
		}
	}
	return false
}

func (g guard) originAllowed(r *http.Request) bool {
	values := r.Header.Values("Origin")
	if len(values) == 0 {
		return true
	}
	if len(values) != 1 {
		return false
	}
	origin, ok := canonicalOrigin(values[0])
	if !ok {
		return false
	}
	for _, allowed := range g.allowedOrigins {
		if origin == allowed {
			return true
		}
	}
	return false
}

func canonicalOrigin(value string) (string, bool) {
	if value == "" || strings.ContainsAny(value, "* \\") {
		return "", false
	}
	parsed, err := url.Parse(value)
	if err != nil || parsed.Opaque != "" || parsed.User != nil || parsed.Host == "" {
		return "", false
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", false
	}
	if parsed.Path != "" || parsed.RawQuery != "" || parsed.ForceQuery || parsed.Fragment != "" {
		return "", false
	}
	if strings.HasSuffix(parsed.Host, ":") {
		return "", false
	}
	scheme := strings.ToLower(parsed.Scheme)
	host := strings.ToLower(parsed.Hostname())
	if !isAllowedHostname(host) {
		return "", false
	}
	port := parsed.Port()
	if (scheme == "http" && port == "80") || (scheme == "https" && port == "443") {
		port = ""
	}
	if strings.Contains(host, ":") {
		host = "[" + host + "]"
	}
	if port != "" {
		host += ":" + port
	}
	return scheme + "://" + host, true
}

func writeError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(status)
	_, _ = io.WriteString(w, message+"\n")
}
