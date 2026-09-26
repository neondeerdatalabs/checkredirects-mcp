package main

import (
	"fmt"
	"regexp"
	"strings"
	"time"
)

// Tool represents an MCP tool definition.
type Tool struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"inputSchema"`
}

// AllTools returns the list of available tools.
func AllTools() []Tool {
	return []Tool{
		{
			Name:        "check_url",
			Description: `Check where a URL redirects. Returns a simplified summary: the final destination, status code, number of hops, and total time. Best for quick lookups of 1-3 URLs. For checking many URLs at once, use batch_check_and_wait instead.`,
			InputSchema: map[string]any{
				"type":     "object",
				"required": []string{"url"},
				"properties": map[string]any{
					"url":        map[string]any{"type": "string", "description": "The URL to check"},
					"user_agent": map[string]any{"type": "string", "description": "User agent preset key (e.g. googlebot_desktop, chrome_windows, safari_iphone). Omit for default."},
					"https_only": map[string]any{"type": "boolean", "description": "If true, don't fall back to HTTP when HTTPS fails. By default, the engine retries on HTTP if HTTPS connection is refused or times out."},
				},
			},
		},
		{
			Name: "inspect_url",
			Description: `Inspect a URL's full redirect chain with detailed hop-by-hop data. Returns every redirect hop with status codes, response headers, timing breakdown (DNS, TCP, TLS, TTFB), TLS certificate details, and IP geolocation. Use this when you need the full technical details. For a quick summary, use check_url instead.

Caller-supplied cookies, Authorization headers, and basic authentication are sent only to the origin of the URL supplied. They are withheld if the redirect chain crosses origins or downgrades from HTTPS to HTTP.`,
			InputSchema: map[string]any{
				"type":     "object",
				"required": []string{"url"},
				"properties": map[string]any{
					"url":              map[string]any{"type": "string", "description": "The URL to inspect"},
					"method":           map[string]any{"type": "string", "enum": []string{"HEAD", "GET"}, "description": "HTTP method (default HEAD). Use GET if you need meta tags or body content."},
					"follow_redirects": map[string]any{"type": "boolean", "description": "Whether to follow redirects (default true)"},
					"max_redirects":    map[string]any{"type": "integer", "description": "Maximum redirects to follow (default 10, max 20)"},
					"user_agent":       map[string]any{"type": "string", "description": "User agent string or preset key (e.g. googlebot_desktop)"},
					"headers":          map[string]any{"type": "object", "additionalProperties": map[string]any{"type": "string"}, "description": "Custom HTTP headers to send"},
					"cookies":          map[string]any{"type": "string", "description": "Cookie header value to send. Only sent to the supplied URL's origin; credentials are withheld after an origin change or HTTPS downgrade."},
					"basic_auth":       map[string]any{"type": "object", "properties": map[string]any{"user": map[string]any{"type": "string"}, "pass": map[string]any{"type": "string"}}, "description": "HTTP basic auth credentials"},
					"retrieve_body":    map[string]any{"type": "boolean", "description": "Extract meta tags from HTML body (requires GET method, paid plans only)"},
					"https_only":       map[string]any{"type": "boolean", "description": "If true, don't fall back to HTTP when HTTPS fails."},
				},
			},
		},
		{
			Name:        "compare_agents",
			Description: `Check how a URL responds to different user agents side by side. Compares redirect behavior across Googlebot, Chrome, social crawlers, etc. Useful for detecting cloaking, mobile redirects, or bot-specific behavior. Returns results for each agent so you can spot differences in status code, final URL, or number of hops.`,
			InputSchema: map[string]any{
				"type":     "object",
				"required": []string{"url"},
				"properties": map[string]any{
					"url":         map[string]any{"type": "string", "description": "The URL to check with multiple agents"},
					"user_agents": map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "List of user agent preset keys (e.g. [\"googlebot_desktop\", \"chrome_windows\", \"safari_iphone\"]). Max 10."},
					"pack": map[string]any{
						"type": "string",
						// Enumerated so a model cannot send a pack that does not
						// exist. These are the five real keys in
						// internal/useragents/catalog.go; the previous list named
						// social_crawlers, browsers and mobile, none of which are
						// pack keys. mcp/ is a separate Go module and cannot
						// import the catalog, so this list is hand-kept and
						// pinned by TestCompareAgentsToolMatchesTheAPI.
						"description": "Use a preset pack instead of listing agents.",
						"enum":        []string{"seo_essentials", "mobile_vs_desktop", "social_preview", "ai_crawler_audit", "full_coverage"},
					},
				},
			},
		},
		{
			Name:        "batch_check_and_wait",
			Description: `Submit multiple URLs for concurrent redirect checking and wait for results. This is the recommended tool for checking more than 3 URLs. Submits the batch, polls until completion, and returns at most the first 50 results; use batch_results with the job_id and a page number to fetch the rest. The API enforces the organization's effective batch limit, available from GET /v1/usage.`,
			InputSchema: map[string]any{
				"type":     "object",
				"required": []string{"urls"},
				"properties": map[string]any{
					"urls":             map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "List of URLs to check. The organization-specific maximum is returned by GET /v1/usage."},
					"method":           map[string]any{"type": "string", "enum": []string{"HEAD", "GET"}},
					"user_agent":       map[string]any{"type": "string", "description": "User agent preset key"},
					"headers":          map[string]any{"type": "object", "additionalProperties": map[string]any{"type": "string"}, "description": "Custom HTTP headers"},
					"cookies":          map[string]any{"type": "string", "description": "Cookie header value. Only sent to each supplied URL's origin; credentials are withheld after an origin change or HTTPS downgrade."},
					"basic_auth":       map[string]any{"type": "object", "properties": map[string]any{"user": map[string]any{"type": "string"}, "pass": map[string]any{"type": "string"}}, "description": "HTTP basic auth credentials"},
					"retrieve_body":    map[string]any{"type": "boolean", "description": "Extract meta tags from HTML body (paid plans only)"},
					"https_only":       map[string]any{"type": "boolean", "description": "If true, don't fall back to HTTP when HTTPS fails."},
					"webhook_url":      map[string]any{"type": "string", "description": "Publicly reachable HTTPS URL for a batch completion notification (all plans)"},
					"export_to_sheets": map[string]any{"type": "boolean", "description": "Auto-export results to Google Sheets when done. The Sheet receives complete stored URLs, including userinfo, query strings, fragments, and path values."},
				},
			},
		},
		{
			Name:        "batch_results",
			Description: `Get results of a previously submitted batch job by its job ID. Returns paginated results with optional filtering. Only use this if you have a job_id from an earlier batch_check_and_wait call and need to re-fetch results or get a different page.`,
			InputSchema: map[string]any{
				"type":     "object",
				"required": []string{"job_id"},
				"properties": map[string]any{
					"job_id":             map[string]any{"type": "string", "description": "The batch job ID"},
					"page":               map[string]any{"type": "integer", "description": "Page number (default 1, 50 results per page)"},
					"sort":               map[string]any{"type": "string", "enum": []string{"input_order", "status", "latency", "hops"}, "description": "Sort results by field"},
					"url_contains":       map[string]any{"type": "string", "description": "Filter to URLs containing this substring (case-insensitive)"},
					"final_url_contains": map[string]any{"type": "string", "description": "Filter to results whose final destination contains this substring"},
				},
			},
		},
		{
			Name:        "list_monitors",
			Description: `List all recurring URL check monitors for the organization. Monitors automatically check a set of URLs on a schedule (e.g. every 6 hours) and can auto-export to Google Sheets.`,
			InputSchema: map[string]any{
				"type":       "object",
				"properties": map[string]any{},
			},
		},
		{
			Name:        "create_monitor",
			Description: `Create a recurring URL check monitor. The monitor will automatically check the specified URLs at the given interval. Results can be auto-exported to Google Sheets and/or sent to a webhook. Supports multi-agent comparison via user_agents or pack. Requires a paid plan. Minimum interval is 30 minutes.`,
			InputSchema: map[string]any{
				"type":     "object",
				"required": []string{"name", "urls", "interval_minutes"},
				"properties": map[string]any{
					"name":             map[string]any{"type": "string", "description": "Monitor name (e.g. 'Top pages weekly check')"},
					"urls":             map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "URLs to check on each run"},
					"interval_minutes": map[string]any{"type": "integer", "description": "Check interval in minutes. Common values: 30 (every 30 min), 360 (every 6 hours), 1440 (daily), 10080 (weekly)"},
					"user_agent":       map[string]any{"type": "string", "description": "Single user agent preset key (e.g. googlebot_desktop)"},
					"user_agents":      map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Multiple user agent keys for compare-agents mode (max 10). Mutually exclusive with user_agent and pack."},
					"pack":             map[string]any{"type": "string", "description": "User agent pack for compare-agents mode (e.g. seo_essentials). Mutually exclusive with user_agent and user_agents."},
					"webhook_url":      map[string]any{"type": "string", "description": "HTTPS URL to POST results to after each run. Includes change detection vs previous run."},
					"sheets_append":    map[string]any{"type": "boolean", "description": "Auto-export results to Google Sheets after each run. The Sheet receives complete stored URLs, including userinfo, query strings, fragments, and path values."},
				},
			},
		},
		{
			Name:        "export_to_sheets",
			Description: `Export batch results to Google Sheets. The Sheet receives complete stored URLs, including userinfo, query strings, fragments, and path values; anyone with access to the Sheet can read them. By default, appends to the organization's single default spreadsheet (creating it on first use). Requires Google Sheets to be connected in the app settings. The batch must be completed before exporting -- use batch_check_and_wait to ensure this.`,
			InputSchema: map[string]any{
				"type":     "object",
				"required": []string{"job_id"},
				"properties": map[string]any{
					"job_id":         map[string]any{"type": "string", "description": "The batch job ID to export"},
					"mode":           map[string]any{"type": "string", "enum": []string{"default", "new", "specific"}, "description": "default: append to org's spreadsheet. new: create a separate spreadsheet. specific: append to a specific spreadsheet_id."},
					"spreadsheet_id": map[string]any{"type": "string", "description": "Target spreadsheet ID (only when mode=specific)"},
				},
			},
		},
		{
			Name:        "trigger_monitor",
			Description: `Trigger an immediate run of a monitor without waiting for its next scheduled time. If the monitor is paused, it will be reactivated. Returns immediately; use monitor_runs to check results.`,
			InputSchema: map[string]any{
				"type":     "object",
				"required": []string{"monitor_id"},
				"properties": map[string]any{
					"monitor_id": map[string]any{"type": "string", "description": "The monitor ID to trigger"},
				},
			},
		},
		{
			Name:        "pause_monitor",
			Description: `Pause a single monitor. It will stop running on its schedule until resumed.`,
			InputSchema: map[string]any{
				"type":     "object",
				"required": []string{"monitor_id"},
				"properties": map[string]any{
					"monitor_id": map[string]any{"type": "string", "description": "The monitor ID to pause"},
				},
			},
		},
		{
			Name:        "resume_monitor",
			Description: `Resume a paused monitor. It will run on the next scheduler tick.`,
			InputSchema: map[string]any{
				"type":     "object",
				"required": []string{"monitor_id"},
				"properties": map[string]any{
					"monitor_id": map[string]any{"type": "string", "description": "The monitor ID to resume"},
				},
			},
		},
		{
			Name:        "monitor_runs",
			Description: `List recent runs for a monitor. Shows job ID, status, URL counts, and timestamps for each run. Optionally includes change detection (what changed vs the previous run).`,
			InputSchema: map[string]any{
				"type":     "object",
				"required": []string{"monitor_id"},
				"properties": map[string]any{
					"monitor_id":      map[string]any{"type": "string", "description": "The monitor ID"},
					"include_changes": map[string]any{"type": "boolean", "description": "Include per-run change detection (which URLs changed status, destination, or hops vs the previous run)"},
				},
			},
		},
		{
			Name:        "reset_monitor_failures",
			Description: `Reset a monitor's consecutive failure counter to zero. Use this after fixing an issue that caused repeated failures.`,
			InputSchema: map[string]any{
				"type":     "object",
				"required": []string{"monitor_id"},
				"properties": map[string]any{
					"monitor_id": map[string]any{"type": "string", "description": "The monitor ID"},
				},
			},
		},
		{
			Name:        "bulk_create_monitors",
			Description: `Create multiple monitors in one call. Each monitor is validated independently; partial success is possible. The total count must not exceed your plan's monitor limit.`,
			InputSchema: map[string]any{
				"type":     "object",
				"required": []string{"monitors"},
				"properties": map[string]any{
					"monitors": map[string]any{
						"type":        "array",
						"description": "Array of monitor definitions (max 25). Each needs name, urls, and interval_minutes.",
						"items": map[string]any{
							"type":     "object",
							"required": []string{"name", "urls", "interval_minutes"},
							"properties": map[string]any{
								"name":             map[string]any{"type": "string"},
								"urls":             map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
								"interval_minutes": map[string]any{"type": "integer"},
							},
						},
					},
				},
			},
		},
		{
			Name:        "bulk_pause_monitors",
			Description: `Pause multiple monitors at once by their IDs. Useful before a deploy or maintenance window.`,
			InputSchema: map[string]any{
				"type":     "object",
				"required": []string{"ids"},
				"properties": map[string]any{
					"ids": map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Monitor IDs to pause (max 50)"},
				},
			},
		},
		{
			Name:        "bulk_resume_monitors",
			Description: `Resume multiple paused monitors at once. They will run on the next scheduler tick.`,
			InputSchema: map[string]any{
				"type":     "object",
				"required": []string{"ids"},
				"properties": map[string]any{
					"ids": map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Monitor IDs to resume (max 50)"},
				},
			},
		},
	}
}

// CallTool dispatches a tool call to the appropriate client method.
func CallTool(client *Client, name string, args map[string]any) (any, error) {
	switch name {
	case "check_url":
		// Simplified: call inspect, return only the key fields.
		result, err := client.InspectURL(args)
		if err != nil {
			return nil, err
		}
		return simplifyResult(result), nil

	case "inspect_url":
		return client.InspectURL(args)

	case "compare_agents":
		return client.CompareAgents(args)

	case "batch_check_and_wait":
		// Submit the batch.
		batchResult, err := client.BatchCheck(args)
		if err != nil {
			return nil, err
		}
		jobID, _ := batchResult["job_id"].(string)
		if jobID == "" {
			return batchResult, nil
		}

		// Poll until complete (max 5 minutes).
		deadline := time.Now().Add(5 * time.Minute)
		for time.Now().Before(deadline) {
			time.Sleep(2 * time.Second)
			progress, err := client.BatchProgress(jobID)
			if err != nil {
				return nil, fmt.Errorf("polling progress: %w", err)
			}
			status, _ := progress["status"].(string)
			if status == "completed" || status == "failed" {
				break
			}
		}

		// Fetch results.
		results, err := client.BatchResults(jobID, 1)
		if err != nil {
			return nil, fmt.Errorf("fetching results: %w", err)
		}

		// Simplify each check for cleaner AI output.
		if checks, ok := results["checks"].([]any); ok {
			simplified := make([]any, len(checks))
			for i, c := range checks {
				if m, ok := c.(map[string]any); ok {
					simplified[i] = simplifyResult(m)
				} else {
					simplified[i] = c
				}
			}
			results["checks"] = simplified
		}

		return results, nil

	case "batch_results":
		jobID, _ := args["job_id"].(string)
		if jobID == "" {
			return nil, fmt.Errorf("job_id is required")
		}
		page := 1
		if p, ok := args["page"].(float64); ok {
			page = int(p)
		}
		// Build query string with optional filters.
		query := fmt.Sprintf("page=%d&per_page=50", page)
		if s, ok := args["sort"].(string); ok && s != "" {
			query += "&sort=" + s
		}
		if s, ok := args["url_contains"].(string); ok && s != "" {
			query += "&url_contains=" + s
		}
		if s, ok := args["final_url_contains"].(string); ok && s != "" {
			query += "&final_url_contains=" + s
		}
		return client.BatchResultsFiltered(jobID, query)

	case "list_monitors":
		return client.ListMonitors()

	case "create_monitor":
		return client.CreateMonitor(args)

	case "export_to_sheets":
		jobID, _ := args["job_id"].(string)
		if jobID == "" {
			return nil, fmt.Errorf("job_id is required")
		}
		params := map[string]any{}
		if mode, ok := args["mode"].(string); ok {
			params["mode"] = mode
		}
		if sid, ok := args["spreadsheet_id"].(string); ok {
			params["spreadsheet_id"] = sid
		}
		return client.ExportToSheets(jobID, params)

	case "trigger_monitor":
		id, _ := args["monitor_id"].(string)
		if id == "" {
			return nil, fmt.Errorf("monitor_id is required")
		}
		return client.TriggerMonitor(id)

	case "pause_monitor":
		id, _ := args["monitor_id"].(string)
		if id == "" {
			return nil, fmt.Errorf("monitor_id is required")
		}
		return client.PauseMonitor(id)

	case "resume_monitor":
		id, _ := args["monitor_id"].(string)
		if id == "" {
			return nil, fmt.Errorf("monitor_id is required")
		}
		return client.ResumeMonitor(id)

	case "monitor_runs":
		id, _ := args["monitor_id"].(string)
		if id == "" {
			return nil, fmt.Errorf("monitor_id is required")
		}
		includeChanges, _ := args["include_changes"].(bool)
		return client.MonitorRuns(id, includeChanges)

	case "reset_monitor_failures":
		id, _ := args["monitor_id"].(string)
		if id == "" {
			return nil, fmt.Errorf("monitor_id is required")
		}
		return client.ResetMonitorFailures(id)

	case "bulk_create_monitors":
		return client.BulkCreateMonitors(args)

	case "bulk_pause_monitors":
		ids := toStringSlice(args["ids"])
		if len(ids) == 0 {
			return nil, fmt.Errorf("ids is required")
		}
		return client.BulkPauseMonitors(ids)

	case "bulk_resume_monitors":
		ids := toStringSlice(args["ids"])
		if len(ids) == 0 {
			return nil, fmt.Errorf("ids is required")
		}
		return client.BulkResumeMonitors(ids)

	default:
		return nil, fmt.Errorf("unknown tool: %s", name)
	}
}

// simplifyResult strips a full inspect response down to the key fields
// that matter for most conversations. Keeps the response small and readable.
func simplifyResult(full map[string]any) map[string]any {
	simple := map[string]any{
		"original_url":  full["original_url"],
		"final_url":     full["final_url"],
		"final_status":  full["final_status"],
		"total_hops":    full["total_hops"],
		"total_time_ms": full["total_time_ms"],
	}

	if e, ok := full["error"]; ok && e != nil {
		simple["error"] = e
	}
	if fb, ok := full["http_fallback_used"]; ok && fb == true {
		simple["http_fallback_used"] = true
	}

	// Pass through the simple redirect chain from the API response.
	if chain, ok := full["redirect_chain"]; ok && chain != nil {
		simple["redirect_chain"] = chain
	}

	// The summary omits hops, so preserve the fact that credentials were
	// withheld on any hop. Otherwise a model can misdiagnose a downstream 401
	// as bad credentials when the service intentionally did not forward them.
	if withheld, reason := credentialsWithheldOnAnyHop(full["hops"]); withheld {
		simple["credentials_withheld"] = true
		if reason != "" {
			simple["credentials_withheld_reason"] = reason
		}
	}

	return simple
}

func credentialsWithheldOnAnyHop(hops any) (bool, string) {
	list, ok := hops.([]any)
	if !ok {
		return false, ""
	}
	for _, item := range list {
		hop, ok := item.(map[string]any)
		if !ok {
			continue
		}
		if withheld, _ := hop["credentials_withheld"].(bool); !withheld {
			continue
		}
		reason, _ := hop["credentials_withheld_reason"].(string)
		return true, reason
	}
	return false, ""
}

// credentialishHeader mirrors internal/httpheaders.credentialish. mcp is a
// separate module, so a source-level drift test keeps the copies synchronized.
var credentialishHeader = regexp.MustCompile(
	`(?i)(^|[-_])(auth|authz|authorization|token|secret|credential|session|passwd|password|` +
		`passphrase|pwd|bearer|jwt|` +
		`apikey|api[-_]key|access[-_]key|private[-_]key|csrf|xsrf|signature|sig)([-_]|$)`)

func isSensitiveHeader(name string) bool {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "set-cookie", "set-cookie2", "cookie", "cookie2",
		"authorization", "proxy-authorization", "www-authenticate",
		"proxy-authenticate", "authentication-info", "proxy-authentication-info",
		"x-auth-token", "x-storage-token", "x-subject-token",
		"x-amzn-oidc-accesstoken", "x-amzn-oidc-data", "x-amzn-oidc-identity",
		"cf-access-jwt-assertion", "x-goog-iap-jwt-assertion",
		"x-ms-token-aad-access-token", "x-ms-token-aad-id-token":
		return true
	}
	return credentialishHeader.MatchString(name)
}

// scrubSensitiveHeaders is the model-visible output boundary. It removes
// credential-bearing headers and strips URL userinfo, query strings, and
// fragments at every response depth without destroying ordinary diagnostics.
func scrubSensitiveHeaders(v any) {
	switch node := v.(type) {
	case map[string]any:
		for key, value := range node {
			if strings.EqualFold(key, "headers") {
				if headers, ok := value.(map[string]any); ok {
					for name, headerValue := range headers {
						if isSensitiveHeader(name) {
							delete(headers, name)
							continue
						}
						switch hv := headerValue.(type) {
						case []any:
							for i, item := range hv {
								if text, ok := item.(string); ok {
									if stripped, changed := stripQueryFromURLValue(text); changed {
										hv[i] = stripped
									}
								}
							}
						case string:
							if stripped, changed := stripQueryFromURLValue(hv); changed {
								headers[name] = stripped
							}
						}
					}
					continue
				}
			}
			if text, ok := value.(string); ok {
				if stripped, changed := stripQueryFromURLValue(text); changed {
					node[key] = stripped
				}
				continue
			}
			scrubSensitiveHeaders(value)
		}
	case []any:
		for i, item := range node {
			if text, ok := item.(string); ok {
				if stripped, changed := stripQueryFromURLValue(text); changed {
					node[i] = stripped
				}
				continue
			}
			scrubSensitiveHeaders(item)
		}
	}
}

// toStringSlice converts an any (expected []any of strings) to []string.
func toStringSlice(v any) []string {
	arr, ok := v.([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(arr))
	for _, item := range arr {
		if s, ok := item.(string); ok {
			out = append(out, s)
		}
	}
	return out
}
