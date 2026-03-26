package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Client wraps the checkredirects.io REST API.
type Client struct {
	baseURL    string
	apiKey     string
	httpClient *http.Client
}

// NewClient creates a new API client.
func NewClient(baseURL, apiKey string) *Client {
	return &Client{
		baseURL: baseURL,
		apiKey:  apiKey,
		httpClient: &http.Client{
			Timeout: 120 * time.Second,
		},
	}
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

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("http request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode >= 400 {
		var apiErr struct {
			Error struct {
				Code    string `json:"code"`
				Message string `json:"message"`
			} `json:"error"`
		}
		if json.Unmarshal(respBody, &apiErr) == nil && apiErr.Error.Message != "" {
			return fmt.Errorf("API error (%d %s): %s", resp.StatusCode, apiErr.Error.Code, apiErr.Error.Message)
		}
		return fmt.Errorf("API error (%d): %s", resp.StatusCode, string(respBody))
	}

	if result != nil {
		if err := json.Unmarshal(respBody, result); err != nil {
			return fmt.Errorf("decode response: %w", err)
		}
	}
	return nil
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
