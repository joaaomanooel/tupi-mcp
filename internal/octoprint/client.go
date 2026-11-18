package octoprint

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime/multipart"
	"net"
	"net/http"
	"net/url"
	"path"
	"strings"
	"unicode"
	"unicode/utf8"
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
	ErrorInvalidUpload     ErrorKind = "invalid_upload"
	ErrorRequestTooLarge   ErrorKind = "request_too_large"
	ErrorInvalidPath       ErrorKind = "invalid_path"
)

const MaxGCodeUploadBytes = 8 << 20

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

type FolderResponse struct {
	Folder *FileInfo `json:"folder"`
	Done   *bool     `json:"done"`
}

type FileCommandResponse struct {
	Name   *string `json:"name"`
	Path   *string `json:"path"`
	Origin *string `json:"origin"`
}

func NewClient(baseURL, apiKey string, httpClient *http.Client, maxBodyBytes int64, allowInsecureHTTP bool) (*Client, error) {
	base, err := url.Parse(baseURL)
	if err != nil || base == nil || (base.Scheme != "http" && base.Scheme != "https") || base.Hostname() == "" || base.User != nil || base.RawQuery != "" || base.ForceQuery || strings.Contains(baseURL, "#") || base.Opaque != "" {
		return nil, &Error{Kind: ErrorConfig}
	}
	if base.Scheme == "http" {
		address := net.ParseIP(base.Hostname())
		if (address == nil || !address.IsLoopback()) && !allowInsecureHTTP {
			return nil, &Error{Kind: ErrorConfig}
		}
	}
	if apiKey == "" || apiKey != strings.TrimSpace(apiKey) || httpClient == nil || httpClient.Timeout <= 0 || maxBodyBytes <= 0 {
		return nil, &Error{Kind: ErrorConfig}
	}
	if strings.Contains(baseURL, apiKey) || strings.Contains(base.Path, apiKey) {
		return nil, &Error{Kind: ErrorConfig}
	}
	transport := *httpClient
	transport.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	return &Client{baseURL: *base, apiKey: apiKey, httpClient: transport, maxBodyBytes: maxBodyBytes}, nil
}

func (c *Client) Connection(ctx context.Context) (*ConnectionResponse, error) {
	return readResponse[ConnectionResponse](ctx, c, "/api/connection")
}

func (c *Client) Printer(ctx context.Context) (*PrinterResponse, error) {
	return readResponse[PrinterResponse](ctx, c, "/api/printer")
}

func (c *Client) Job(ctx context.Context) (*JobResponse, error) {
	return readResponse[JobResponse](ctx, c, "/api/job")
}

func (c *Client) Files(ctx context.Context) (*FilesResponse, error) {
	return readResponse[FilesResponse](ctx, c, "/api/files")
}

func (c *Client) Version(ctx context.Context) (*VersionResponse, error) {
	return readResponse[VersionResponse](ctx, c, "/api/version")
}

func (c *Client) Server(ctx context.Context) (*ServerResponse, error) {
	return readResponse[ServerResponse](ctx, c, "/api/server")
}

func (c *Client) ToolTemperatures(ctx context.Context) (*ToolTemperaturesResponse, error) {
	return readResponse[ToolTemperaturesResponse](ctx, c, "/api/printer/tool")
}

func (c *Client) BedTemperature(ctx context.Context) (*BedTemperatureResponse, error) {
	return readResponse[BedTemperatureResponse](ctx, c, "/api/printer/bed")
}

func (c *Client) ChamberTemperature(ctx context.Context) (*ChamberTemperatureResponse, error) {
	return readResponse[ChamberTemperatureResponse](ctx, c, "/api/printer/chamber")
}

func (c *Client) SDState(ctx context.Context) (*SDStateResponse, error) {
	return readResponse[SDStateResponse](ctx, c, "/api/printer/sd")
}

func (c *Client) LastPrinterError(ctx context.Context) (*PrinterErrorResponse, error) {
	return readResponse[PrinterErrorResponse](ctx, c, "/api/printer/error")
}

func (c *Client) Profiles(ctx context.Context) (*ProfilesResponse, error) {
	return readResponse[ProfilesResponse](ctx, c, "/api/printerprofiles")
}

func (c *Client) Slicing(ctx context.Context) (*SlicingResponse, error) {
	return readResponse[SlicingResponse](ctx, c, "/api/slicing")
}

func (c *Client) Timelapse(ctx context.Context) (*TimelapseResponse, error) {
	return readResponseWithQuery[TimelapseResponse](ctx, c, "/api/timelapse", url.Values{"unrendered": {"true"}})
}

func (c *Client) Settings(ctx context.Context) (*SettingsResponse, error) {
	return readResponse[SettingsResponse](ctx, c, "/api/settings")
}

func (c *Client) UploadGCode(ctx context.Context, filename string, contents []byte) (*UploadResponse, error) {
	if !validGCodeFilename(filename) || len(contents) == 0 {
		return nil, &Error{Kind: ErrorInvalidUpload}
	}
	if len(contents) > MaxGCodeUploadBytes {
		return nil, &Error{Kind: ErrorRequestTooLarge}
	}
	if !ValidGCodeContents(contents) {
		return nil, &Error{Kind: ErrorInvalidUpload}
	}

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	file, err := writer.CreateFormFile("file", filename)
	if err != nil {
		return nil, &Error{Kind: ErrorInvalidUpload}
	}
	if _, err := file.Write(contents); err != nil {
		return nil, &Error{Kind: ErrorInvalidUpload}
	}
	if err := writer.WriteField("select", "false"); err != nil {
		return nil, &Error{Kind: ErrorInvalidUpload}
	}
	if err := writer.WriteField("print", "false"); err != nil {
		return nil, &Error{Kind: ErrorInvalidUpload}
	}
	if err := writer.WriteField("noOverwrite", "true"); err != nil {
		return nil, &Error{Kind: ErrorInvalidUpload}
	}
	if err := writer.Close(); err != nil {
		return nil, &Error{Kind: ErrorInvalidUpload}
	}

	target := c.baseURL
	target.Path = strings.TrimRight(target.Path, "/") + "/api/files/local"
	target.RawPath = ""
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, target.String(), &body)
	if err != nil {
		return nil, safeTransportError(ctx, err)
	}
	request.Header.Set("X-Api-Key", c.apiKey)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	response, err := c.httpClient.Do(request)
	if err != nil {
		return nil, safeTransportError(ctx, err)
	}
	responseBody, err := c.readResponse(ctx, response)
	if err != nil {
		return nil, err
	}
	var uploaded *UploadResponse
	if err := json.Unmarshal(responseBody, &uploaded); err != nil || uploaded == nil || uploaded.File == nil && uploaded.Files["local"] == nil {
		return nil, &Error{Kind: ErrorInvalidJSON}
	}
	return uploaded, nil
}

func (c *Client) CreateFolder(ctx context.Context, name, parent string) (*FolderResponse, error) {
	if !validLocalName(name) || parent != "" && !validLocalPath(parent) {
		return nil, &Error{Kind: ErrorInvalidPath}
	}

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	if err := writer.WriteField("foldername", name); err != nil {
		return nil, &Error{Kind: ErrorInvalidPath}
	}
	if parent != "" {
		if err := writer.WriteField("path", parent); err != nil {
			return nil, &Error{Kind: ErrorInvalidPath}
		}
	}
	if err := writer.Close(); err != nil {
		return nil, &Error{Kind: ErrorInvalidPath}
	}

	responseBody, err := c.post(ctx, "/api/files/local", body.Bytes(), writer.FormDataContentType())
	if err != nil {
		return nil, err
	}
	var folder *FolderResponse
	if err := json.Unmarshal(responseBody, &folder); err != nil || folder == nil || folder.Folder == nil || folder.Folder.Path == nil || !validLocalPath(*folder.Folder.Path) {
		return nil, &Error{Kind: ErrorInvalidJSON}
	}
	return folder, nil
}

func (c *Client) CopyFile(ctx context.Context, source, destination string) (*FileCommandResponse, error) {
	return c.fileCommand(ctx, "copy", source, destination)
}

func (c *Client) MoveFile(ctx context.Context, source, destination string) (*FileCommandResponse, error) {
	return c.fileCommand(ctx, "move", source, destination)
}

func (c *Client) fileCommand(ctx context.Context, command, source, destination string) (*FileCommandResponse, error) {
	if !validLocalPath(source) || !validLocalPath(destination) {
		return nil, &Error{Kind: ErrorInvalidPath}
	}

	payload, err := json.Marshal(struct {
		Command     string `json:"command"`
		Destination string `json:"destination"`
	}{Command: command, Destination: destination})
	if err != nil {
		return nil, &Error{Kind: ErrorInvalidPath}
	}

	responseBody, err := c.post(ctx, "/api/files/local/"+source, payload, "application/json")
	if err != nil {
		return nil, err
	}
	var result *FileCommandResponse
	if err := json.Unmarshal(responseBody, &result); err != nil || result == nil || result.Path == nil || !validLocalPath(*result.Path) {
		return nil, &Error{Kind: ErrorInvalidJSON}
	}
	return result, nil
}

func (c *Client) SelectFile(ctx context.Context, filename string) error {
	if !validLocalPath(filename) {
		return &Error{Kind: ErrorInvalidPath}
	}
	payload, err := json.Marshal(struct {
		Command string `json:"command"`
		Print   bool   `json:"print"`
	}{Command: "select"})
	if err != nil {
		return &Error{Kind: ErrorInvalidPath}
	}
	return c.sendNoContent(ctx, http.MethodPost, "/api/files/local/"+filename, payload, "application/json")
}

func (c *Client) DeleteFile(ctx context.Context, filename string) error {
	if !validLocalPath(filename) {
		return &Error{Kind: ErrorInvalidPath}
	}
	return c.sendNoContent(ctx, http.MethodDelete, "/api/files/local/"+filename, nil, "")
}

func (c *Client) SliceFile(ctx context.Context, filename, destination string) (*FileCommandResponse, error) {
	if !validLocalPath(filename) || !validLocalPath(destination) {
		return nil, &Error{Kind: ErrorInvalidPath}
	}
	payload, err := json.Marshal(struct {
		Command     string `json:"command"`
		Destination string `json:"destination"`
		Select      bool   `json:"select"`
		Print       bool   `json:"print"`
	}{Command: "slice", Destination: destination})
	if err != nil {
		return nil, &Error{Kind: ErrorInvalidPath}
	}
	responseBody, err := c.post(ctx, "/api/files/local/"+filename, payload, "application/json")
	if err != nil {
		return nil, err
	}
	var result *FileCommandResponse
	if err := json.Unmarshal(responseBody, &result); err != nil || result == nil || result.Path == nil || !validLocalPath(*result.Path) {
		return nil, &Error{Kind: ErrorInvalidJSON}
	}
	return result, nil
}

func ValidGCodeContents(contents []byte) bool {
	for len(contents) > 0 {
		character, size := utf8.DecodeRune(contents)
		if character == utf8.RuneError && size == 1 || unicode.IsControl(character) && character != '\n' && character != '\r' && character != '\t' {
			return false
		}
		contents = contents[size:]
	}
	return true
}

func validGCodeFilename(filename string) bool {
	extension := path.Ext(filename)
	basename := strings.TrimSuffix(filename, extension)
	return filename != "" && len(filename) <= 255 && filename == strings.TrimSpace(filename) && !strings.ContainsAny(filename, "/\\") && !strings.ContainsFunc(filename, unicode.IsControl) && strings.EqualFold(extension, ".gcode") && basename != "" && basename != "." && basename != ".."
}

func validLocalPath(value string) bool {
	if value == "" || !utf8.ValidString(value) || strings.Contains(value, `\`) || strings.ContainsFunc(value, unicode.IsControl) {
		return false
	}
	for _, segment := range strings.Split(value, "/") {
		if segment == "" || segment == "." || segment == ".." {
			return false
		}
	}
	return true
}

func validLocalName(name string) bool {
	return name != "" && !strings.Contains(name, "/") && validLocalPath(name)
}

func (c *Client) post(ctx context.Context, endpoint string, body []byte, contentType string) ([]byte, error) {
	response, err := c.send(ctx, http.MethodPost, endpoint, body, contentType)
	if err != nil {
		return nil, err
	}
	return c.readResponse(ctx, response)
}

func (c *Client) sendNoContent(ctx context.Context, method, endpoint string, body []byte, contentType string) error {
	response, err := c.send(ctx, method, endpoint, body, contentType)
	if err != nil {
		return err
	}
	return c.readNoContent(ctx, response)
}

func (c *Client) send(ctx context.Context, method, endpoint string, body []byte, contentType string) (*http.Response, error) {
	target := c.baseURL
	target.Path = strings.TrimRight(target.Path, "/") + endpoint
	target.RawPath = ""
	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}
	request, err := http.NewRequestWithContext(ctx, method, target.String(), reader)
	if err != nil {
		return nil, safeTransportError(ctx, err)
	}
	request.Header.Set("X-Api-Key", c.apiKey)
	if contentType != "" {
		request.Header.Set("Content-Type", contentType)
	}
	response, err := c.httpClient.Do(request)
	if err != nil {
		return nil, safeTransportError(ctx, err)
	}
	return response, nil
}

func (c *Client) get(ctx context.Context, endpoint string) ([]byte, error) {
	return c.getWithQuery(ctx, endpoint, nil)
}

func (c *Client) getWithQuery(ctx context.Context, endpoint string, query url.Values) ([]byte, error) {
	target := c.baseURL
	target.Path = strings.TrimRight(target.Path, "/") + endpoint
	target.RawPath = ""
	target.RawQuery = query.Encode()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, target.String(), nil)
	if err != nil {
		return nil, safeTransportError(ctx, err)
	}
	request.Header.Set("X-Api-Key", c.apiKey)
	response, err := c.httpClient.Do(request)
	if err != nil {
		return nil, safeTransportError(ctx, err)
	}
	return c.readResponse(ctx, response)
}

func (c *Client) readNoContent(ctx context.Context, response *http.Response) error {
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, c.maxBodyBytes))
		return &Error{Kind: ErrorHTTPStatus, StatusCode: response.StatusCode}
	}
	if _, err := io.Copy(io.Discard, io.LimitReader(response.Body, c.maxBodyBytes)); err != nil {
		return safeTransportError(ctx, err)
	}
	return nil
}

func (c *Client) readResponse(ctx context.Context, response *http.Response) ([]byte, error) {
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, c.maxBodyBytes))
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
