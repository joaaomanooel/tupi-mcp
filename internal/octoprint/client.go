package octoprint

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
)

type ErrorKind string

const (
	ErrorConfig            ErrorKind = "configuration"
	ErrorTransport         ErrorKind = "transport"
	ErrorTimeout           ErrorKind = "timeout"
	ErrorCanceled          ErrorKind = "canceled"
	ErrorHTTPStatus        ErrorKind = "http_status"
	ErrorInvalidJSON       ErrorKind = "invalid_json"
	ErrorResponseTooLarge  ErrorKind = "response_too_large"
	ErrorSensitiveResponse ErrorKind = "sensitive_response"
)

type Error struct {
	Kind       ErrorKind
	StatusCode int
	cause      error
}

func (e *Error) Error() string {
	return "octoprint: " + string(e.Kind)
}

func (e *Error) Unwrap() error {
	return e.cause
}

type Client struct {
	baseURL      url.URL
	apiKey       string
	httpClient   http.Client
	maxBodyBytes int64
}

func NewClient(baseURL, apiKey string, httpClient *http.Client, maxBodyBytes int64) (*Client, error) {
	base, err := url.Parse(baseURL)
	if err != nil || base == nil || (base.Scheme != "http" && base.Scheme != "https") || base.Hostname() == "" || base.User != nil || base.RawQuery != "" || base.ForceQuery || strings.Contains(baseURL, "#") || base.Opaque != "" {
		return nil, &Error{Kind: ErrorConfig}
	}
	if strings.TrimSpace(apiKey) == "" || httpClient == nil || httpClient.Timeout <= 0 || maxBodyBytes <= 0 {
		return nil, &Error{Kind: ErrorConfig}
	}
	if strings.Contains(baseURL, apiKey) || strings.Contains(base.Path, apiKey) {
		return nil, &Error{Kind: ErrorConfig}
	}
	transport := *httpClient
	transport.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	return &Client{baseURL: *base, apiKey: apiKey, httpClient: transport, maxBodyBytes: maxBodyBytes}, nil
}

func (c *Client) Connection(ctx context.Context) ([]byte, error) {
	return c.get(ctx, "/api/connection")
}
func (c *Client) Printer(ctx context.Context) ([]byte, error) { return c.get(ctx, "/api/printer") }
func (c *Client) Job(ctx context.Context) ([]byte, error)     { return c.get(ctx, "/api/job") }
func (c *Client) Files(ctx context.Context) ([]byte, error)   { return c.get(ctx, "/api/files") }

func (c *Client) get(ctx context.Context, endpoint string) ([]byte, error) {
	target := c.baseURL
	target.Path = strings.TrimRight(target.Path, "/") + endpoint
	target.RawPath = ""
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, target.String(), nil)
	if err != nil {
		return nil, safeTransportError(ctx, err)
	}
	request.Header.Set("X-Api-Key", c.apiKey)
	response, err := c.httpClient.Do(request)
	if err != nil {
		return nil, safeTransportError(ctx, err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, &Error{Kind: ErrorHTTPStatus, StatusCode: response.StatusCode}
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, c.maxBodyBytes))
	if err != nil {
		return nil, safeTransportError(ctx, err)
	}
	var extra [1]byte
	n, err := io.ReadFull(response.Body, extra[:])
	if n > 0 {
		return nil, &Error{Kind: ErrorResponseTooLarge}
	}
	if err != nil && !errors.Is(err, io.EOF) {
		return nil, safeTransportError(ctx, err)
	}
	if !json.Valid(body) {
		return nil, &Error{Kind: ErrorInvalidJSON}
	}
	if bytes.Contains(body, []byte(c.apiKey)) {
		return nil, &Error{Kind: ErrorSensitiveResponse}
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	for {
		token, err := decoder.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, &Error{Kind: ErrorInvalidJSON}
		}
		if value, ok := token.(string); ok && strings.Contains(value, c.apiKey) {
			return nil, &Error{Kind: ErrorSensitiveResponse}
		}
	}
	return body, nil
}

func safeTransportError(ctx context.Context, err error) error {
	if contextErr := ctx.Err(); contextErr != nil {
		err = contextErr
	}
	if errors.Is(err, context.Canceled) {
		return &Error{Kind: ErrorCanceled, cause: context.Canceled}
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return &Error{Kind: ErrorTimeout, cause: context.DeadlineExceeded}
	}
	var timeout net.Error
	if errors.As(err, &timeout) && timeout.Timeout() {
		return &Error{Kind: ErrorTimeout, cause: context.DeadlineExceeded}
	}
	return &Error{Kind: ErrorTransport}
}
