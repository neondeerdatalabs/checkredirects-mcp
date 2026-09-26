package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

// Client wraps the checkredirects.io REST API.
type Client struct {
	baseURL    string
	apiKey     string
	httpClient *http.Client
}

// NewClient creates a new API client, refusing any base URL that is not a
// checkredirects.io endpoint over HTTPS.
//
// The check lives here, in the constructor that attaches the API key, rather
// than in main. It was a strings.HasSuffix on the raw URL performed by the
// caller, and it let four separate shapes through — verified, not theorised:
//
//	https://attacker.example/.checkredirects.io   (suffix is in the PATH)
//	https://evil.example#.checkredirects.io       (fragment)
//	https://evil.example?x=.checkredirects.io     (query)
//	http://api.checkredirects.io                  (cleartext)
//
// The first sends the customer's API key to an attacker-chosen host. The last
// sends it in the clear. Both were reachable by setting one environment
// variable, which is exactly the threat the check was written to stop.
//
// Parsing once and comparing the parsed host is the fix; doing it where the
// credential is attached is what stops a future second call site from skipping
// it.
func NewClient(baseURL, apiKey string) (*Client, error) {
	normalized, err := validateBaseURL(baseURL)
	if err != nil {
		return nil, err
	}
	return &Client{
		baseURL: normalized,
		apiKey:  apiKey,
		httpClient: &http.Client{
			Timeout: 120 * time.Second,
		},
	}, nil
}

// apiHost is the only host the API key may be sent to. Subdomains are allowed;
// anything else is not.
const apiHost = "checkredirects.io"

// isLoopback reports whether the host is unambiguously this machine.
//
// Loopback on any port is allowed, not just the one dev port: an address that
// resolves to this machine is not an attacker-controlled host, so the threat
// this allowlist exists to stop does not apply. Pinning a single port bought no
// security and forced tests onto a bypass constructor, which would have been a
// hole in the boundary this whole change is about.
//
// Literal IPs are checked with net.ParseIP rather than by string, so
// "127.0.0.1.attacker.example" is not mistaken for loopback. The bare name
// "localhost" is accepted because Go resolves it to loopback; a name merely
// ENDING in localhost is not.
func isLoopback(hostname string) bool {
	if strings.EqualFold(hostname, "localhost") {
		return true
	}
	if ip := net.ParseIP(hostname); ip != nil {
		return ip.IsLoopback()
	}
	return false
}

// validateBaseURL returns the URL to use, or an error explaining the refusal.
func validateBaseURL(raw string) (string, error) {
	trimmed := strings.TrimRight(strings.TrimSpace(raw), "/")
	u, err := url.Parse(trimmed)
	if err != nil {
		return "", fmt.Errorf("CHECKREDIRECTS_API_URL is not a valid URL: %w", err)
	}
	// A base URL carries no path, query, fragment or credentials. Rejecting
	// them is not pedantry: every one of the confirmed bypasses smuggled the
	// allowlisted name into one of those positions.
	if u.Path != "" || u.RawQuery != "" || u.Fragment != "" || u.User != nil {
		// Do not repeat the rejected value. The exact reason this branch rejects
		// userinfo/query/fragment is that those components can contain secrets;
		// echoing the value into startup logs would undo that protection.
		return "", fmt.Errorf("CHECKREDIRECTS_API_URL must be a bare origin with no path, " +
			"query, fragment or credentials")
	}
	if isLoopback(u.Hostname()) {
		if u.Scheme != "http" && u.Scheme != "https" {
			return "", fmt.Errorf("CHECKREDIRECTS_API_URL has an unsupported scheme %q", u.Scheme)
		}
		return trimmed, nil
	}
	if u.Scheme != "https" {
		return "", fmt.Errorf("CHECKREDIRECTS_API_URL must use https, so the API key is not "+
			"sent in the clear (got %s)", trimmed)
	}
	host := strings.ToLower(u.Hostname())
	if host != apiHost && !strings.HasSuffix(host, "."+apiHost) {
		return "", fmt.Errorf("CHECKREDIRECTS_API_URL must be a %s host, or the API key would "+
			"be sent to %s", apiHost, host)
	}
	return trimmed, nil
}

// do sends an HTTP request and decodes the JSON response.
func (c *Client) do(method, path string, body any, result any) error {
	var bodyReader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("marshal request: %w", err)
		}
		bodyReader = bytes.NewReader(b)
	}

	req, err := http.NewRequest(method, c.baseURL+path, bodyReader)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "checkredirects.io/mcp")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("http request: %s", sanitizeErrorText(err.Error()))
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read response: %s", sanitizeErrorText(err.Error()))
	}

	if resp.StatusCode >= 400 {
		var apiErr struct {
			Error struct {
				Code string `json:"code"`
			} `json:"error"`
		}
		code := ""
		if json.Unmarshal(respBody, &apiErr) == nil {
			code = apiErr.Error.Code
		}
		return fmt.Errorf("API error (%d): %s", resp.StatusCode, describeAPIError(resp.StatusCode, code))
	}

	if result != nil {
		if err := json.Unmarshal(respBody, result); err != nil {
			return fmt.Errorf("decode response: %w", err)
		}
	}
	return nil
}

// Ping signals to the API that the MCP server is being used.
func (c *Client) Ping() {
	_ = c.do("POST", "/v1/mcp/ping", nil, nil)
}

// InspectURL checks a single URL.
func (c *Client) InspectURL(params map[string]any) (map[string]any, error) {
	var result map[string]any
	err := c.do("POST", "/v1/inspect", params, &result)
	return result, err
}

// BatchCheck submits a batch of URLs.
func (c *Client) BatchCheck(params map[string]any) (map[string]any, error) {
	var result map[string]any
	err := c.do("POST", "/v1/batch", params, &result)
	return result, err
}

// BatchResults gets batch results.
func (c *Client) BatchResults(jobID string, page int) (map[string]any, error) {
	var result map[string]any
	path := fmt.Sprintf("/v1/batch/%s?page=%d&per_page=50", jobID, page)
	err := c.do("GET", path, nil, &result)
	return result, err
}

// BatchResultsFiltered gets batch results with a custom query string.
func (c *Client) BatchResultsFiltered(jobID, query string) (map[string]any, error) {
	var result map[string]any
	path := fmt.Sprintf("/v1/batch/%s?%s", jobID, query)
	err := c.do("GET", path, nil, &result)
	return result, err
}

// BatchProgress gets batch progress.
func (c *Client) BatchProgress(jobID string) (map[string]any, error) {
	var result map[string]any
	path := fmt.Sprintf("/v1/batch/%s/progress", jobID)
	err := c.do("GET", path, nil, &result)
	return result, err
}

// ListMonitors lists monitors.
func (c *Client) ListMonitors() (map[string]any, error) {
	var result map[string]any
	err := c.do("GET", "/v1/monitors", nil, &result)
	return result, err
}

// CreateMonitor creates a new monitor.
func (c *Client) CreateMonitor(params map[string]any) (map[string]any, error) {
	var result map[string]any
	err := c.do("POST", "/v1/monitors", params, &result)
	return result, err
}

// CompareAgents compares a URL across multiple user agents.
func (c *Client) CompareAgents(params map[string]any) (map[string]any, error) {
	var result map[string]any
	err := c.do("POST", "/v1/compare-agents", params, &result)
	return result, err
}

// ExportToSheets exports batch results to Google Sheets.
func (c *Client) ExportToSheets(jobID string, params map[string]any) (map[string]any, error) {
	var result map[string]any
	path := fmt.Sprintf("/v1/batch/%s/export/sheets", jobID)
	err := c.do("POST", path, params, &result)
	return result, err
}

// BulkCreateMonitors creates multiple monitors in one call.
func (c *Client) BulkCreateMonitors(params map[string]any) (map[string]any, error) {
	var result map[string]any
	err := c.do("POST", "/v1/monitors/bulk", params, &result)
	return result, err
}

// BulkPauseMonitors pauses multiple monitors by ID.
func (c *Client) BulkPauseMonitors(ids []string) (map[string]any, error) {
	var result map[string]any
	err := c.do("POST", "/v1/monitors/bulk/pause", map[string]any{"ids": ids}, &result)
	return result, err
}

// BulkResumeMonitors resumes multiple monitors by ID.
func (c *Client) BulkResumeMonitors(ids []string) (map[string]any, error) {
	var result map[string]any
	err := c.do("POST", "/v1/monitors/bulk/resume", map[string]any{"ids": ids}, &result)
	return result, err
}

// TriggerMonitor triggers an immediate run of a monitor.
func (c *Client) TriggerMonitor(monitorID string) (map[string]any, error) {
	var result map[string]any
	path := fmt.Sprintf("/v1/monitors/%s/trigger", monitorID)
	err := c.do("POST", path, nil, &result)
	return result, err
}

// PauseMonitor pauses a single monitor.
func (c *Client) PauseMonitor(monitorID string) (map[string]any, error) {
	var result map[string]any
	path := fmt.Sprintf("/v1/monitors/%s/pause", monitorID)
	err := c.do("POST", path, nil, &result)
	return result, err
}

// ResumeMonitor resumes a single monitor.
func (c *Client) ResumeMonitor(monitorID string) (map[string]any, error) {
	var result map[string]any
	path := fmt.Sprintf("/v1/monitors/%s/resume", monitorID)
	err := c.do("POST", path, nil, &result)
	return result, err
}

// ResetMonitorFailures resets the consecutive failure counter.
func (c *Client) ResetMonitorFailures(monitorID string) (map[string]any, error) {
	var result map[string]any
	path := fmt.Sprintf("/v1/monitors/%s/reset-failures", monitorID)
	err := c.do("POST", path, nil, &result)
	return result, err
}

// MonitorRuns lists runs for a monitor, optionally with change detection.
func (c *Client) MonitorRuns(monitorID string, includeChanges bool) (map[string]any, error) {
	var result map[string]any
	path := fmt.Sprintf("/v1/monitors/%s/runs", monitorID)
	if includeChanges {
		path += "?include_changes=true"
	}
	err := c.do("GET", path, nil, &result)
	return result, err
}

// describeAPIError renders an error for model-visible output using only text
// authored here.
//
// No part of the remote response is echoed — not the message, not the code
// value. An earlier version echoed the message when the code was one this API
// issues, on the theory that an intermediary would not use our vocabulary.
// That theory is wrong: this API sits behind Cloudflare, which can return its
// own error body for our own domain over a perfectly valid TLS connection, and
// nothing stops any proxy from picking a plausible code like "unauthorized".
// A code cannot establish provenance, so it cannot gate disclosure.
//
// The cost is real — the API's own messages name the failing field, and that
// detail is lost here. It is worth paying, because the alternative is that a
// gateway diagnostic quoting the Authorization header it just rejected lands
// in a model's context window, provider logs and transcripts, where nothing
// downstream knows it was a credential.
func describeAPIError(status int, code string) string {
	// The code is matched, never printed. Matching a known value tells us
	// which locally-authored sentence to use; printing it would forward
	// upstream-controlled bytes.
	switch code {
	case "unauthorized":
		return "Authentication failed. Check the CHECKREDIRECTS_API_KEY environment variable."
	case "forbidden":
		return "This API key is not permitted to perform that operation."
	case "upgrade_required":
		return "That operation requires a paid plan."
	case "rate_limited":
		return "Rate limit exceeded. Wait before retrying."
	case "validation_error", "invalid_request":
		return "The request was rejected as invalid. Check the arguments against the tool schema."
	case "not_found":
		return "No such resource."
	case "batch_not_available":
		return "Batch processing is not available on this plan."
	case "not_connected":
		return "That integration is not connected for this account."
	case "sheets_disabled":
		return "Google Sheets export is not enabled for this account."
	case "internal_error":
		return "The API reported an internal error. Retrying may succeed."
	}

	// Unknown or absent code. The response may not be from the API at all —
	// a proxy, gateway or WAF error page reaches here too.
	switch {
	case status == 401 || status == 403:
		return "Request was refused. This may be the API rejecting the key, or a gateway in front of it."
	case status == 429:
		return "Rate limited, by the API or by something in front of it."
	case status >= 500:
		return "Upstream returned a server error. This may be the API or an intermediary."
	default:
		return "Request failed and the response did not carry a recognized error code."
	}
}

// credentialPattern matches the credential shapes most likely to appear in a
// transport error: this API's own key format, bearer tokens, and JWTs.
//
// Transport errors are kept rather than discarded because "connection refused"
// and "TLS handshake timeout" are exactly what someone running this server
// locally needs to see. Redacting the token shapes keeps that value without
// forwarding a credential if one is ever embedded in a URL or a proxy's
// diagnostic text.
var credentialPattern = regexp.MustCompile(
	`(?i)(httpnd_[A-Za-z0-9_-]+|bearer\s+[A-Za-z0-9._~+/-]+=*|eyJ[A-Za-z0-9._-]{10,})`)

// embeddedURL matches a URL anywhere inside a larger string, so a customer URL
// wrapped in an error message can be reduced along with bare ones. Go's
// url.Error embeds the full request URL in its own message, which is how this
// class of leak has repeatedly reached logs and telemetry elsewhere in this
// codebase.
// Matches absolute URLs with any scheme casing, and protocol-relative ones
// (//host/path), anywhere inside a larger string. Case-insensitive because
// HTTPS:// is legal, and unanchored because a URL usually arrives wrapped in an
// error message rather than alone.
var embeddedURL = regexp.MustCompile(`(?i)(https?:)?//[^\s"'` + "`" + `<>\\]+`)

// sanitizeErrorText removes credentials and query strings from text that is
// about to become model-visible.
//
// The credential pattern alone was not enough. It matches API keys, bearer
// tokens and JWTs, and misses the more common shape: a customer URL whose query
// string carries a session token, a reset token or an email address. Those
// reach the model provider verbatim, which for a DPA is a disclosure to a
// sub-processor.
func sanitizeErrorText(s string) string {
	// URL pass FIRST, then credentials. The other order corrupts its own input:
	// on `Bearer https://h/p?t=secret` the credential pattern matches
	// "Bearer https" and leaves "[redacted]://h/p?t=secret", which no longer
	// looks like a URL, so the query survives. Reducing URLs first means the
	// credential pass only ever sees what is left.
	out := embeddedURL.ReplaceAllStringFunc(s, stripURLQuery)
	return credentialPattern.ReplaceAllString(out, "[redacted]")
}

// stripURLQuery reduces a URL to scheme, host and path.
//
// Shared by the error path and the model-result walk so the two cannot drift —
// they were separate implementations that already disagreed about unparseable
// input.
func stripURLQuery(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		// Unparseable: drop it entirely rather than pass through something we
		// could not reason about.
		return "[redacted-url]"
	}
	if u.RawQuery == "" && u.Fragment == "" && u.User == nil {
		return raw
	}
	u.RawQuery = ""
	u.Fragment = ""
	u.User = nil
	return u.String()
}

// stripQueryFromURLValue reduces any URL inside s and reports whether anything
// changed. Used by the model-result walk, which needs to know whether to
// replace the value.
func stripQueryFromURLValue(s string) (string, bool) {
	out := embeddedURL.ReplaceAllStringFunc(s, stripURLQuery)
	return out, out != s
}
