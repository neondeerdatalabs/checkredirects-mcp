package main

import (
	"fmt"
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
			Name: "check_url",
			Description: `Check where a URL redirects. Returns a simplified summary: the final destination, status code, number of hops, and total time. Best for quick lookups of 1-3 URLs. For checking many URLs at once, use batch_check_and_wait instead.`,
			InputSchema: map[string]any{
				"type":     "object",
				"required": []string{"url"},
				"properties": map[string]any{
					"url":        map[string]any{"type": "string", "description": "The URL to check"},
					"user_agent": map[string]any{"type": "string", "description": "User agent preset key (e.g. googlebot_desktop, chrome_desktop, iphone). Omit for default."},
				},
			},
		},
		{
			Name: "inspect_url",
			Description: `Inspect a URL's full redirect chain with detailed hop-by-hop data. Returns every redirect hop with status codes, response headers, timing breakdown (DNS, TCP, TLS, TTFB), TLS certificate details, and IP geolocation. Use this when you need the full technical details. For a quick summary, use check_url instead.`,
			InputSchema: map[string]any{
				"type":     "object",
				"required": []string{"url"},
				"properties": map[string]any{
					"url":              map[string]any{"type": "string", "description": "The URL to inspect"},
					"method":           map[string]any{"type": "string", "enum": []string{"HEAD", "GET"}, "description": "HTTP method (default HEAD). Use GET if you need meta tags or body content."},
					"follow_redirects": map[string]any{"type": "boolean", "description": "Whether to follow redirects (default true)"},
					"max_redirects":    map[string]any{"type": "integer", "description": "Maximum redirects to follow (default 10, max 20)"},
					"user_agent":       map[string]any{"type": "string", "description": "User agent string or preset key (e.g. googlebot_desktop)"},
				},
			},
		},
		{
			Name: "compare_agents",
			Description: `Check how a URL responds to different user agents side by side. Compares redirect behavior across Googlebot, Chrome, social crawlers, etc. Useful for detecting cloaking, mobile redirects, or bot-specific behavior. Returns results for each agent so you can spot differences in status code, final URL, or number of hops.`,
			InputSchema: map[string]any{
				"type":     "object",
				"required": []string{"url"},
				"properties": map[string]any{
					"url":         map[string]any{"type": "string", "description": "The URL to check with multiple agents"},
					"user_agents": map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "List of user agent preset keys (e.g. [\"googlebot_desktop\", \"chrome_desktop\", \"iphone\"]). Max 10."},
					"pack":        map[string]any{"type": "string", "description": "Use a preset pack instead of listing agents. Options: seo_essentials, social_crawlers, browsers, mobile"},
				},
			},
		},
		{
			Name: "batch_check_and_wait",
			Description: `Submit multiple URLs for concurrent redirect checking and wait for results. This is the recommended tool for checking more than 3 URLs. Submits the batch, polls until completion, and returns all results. Can check up to 500 URLs at once.`,
			InputSchema: map[string]any{
				"type":     "object",
				"required": []string{"urls"},
				"properties": map[string]any{
					"urls":             map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "List of URLs to check (max 500)"},
					"method":           map[string]any{"type": "string", "enum": []string{"HEAD", "GET"}},
					"user_agent":       map[string]any{"type": "string", "description": "User agent preset key"},
					"export_to_sheets": map[string]any{"type": "boolean", "description": "Auto-export results to Google Sheets when done"},
				},
			},
		},
		{
			Name: "batch_results",
			Description: `Get results of a previously submitted batch job by its job ID. Returns paginated results. Only use this if you have a job_id from an earlier batch_check_and_wait call and need to re-fetch results or get a different page.`,
			InputSchema: map[string]any{
				"type":     "object",
				"required": []string{"job_id"},
				"properties": map[string]any{
					"job_id": map[string]any{"type": "string", "description": "The batch job ID"},
					"page":   map[string]any{"type": "integer", "description": "Page number (default 1, 50 results per page)"},
				},
			},
		},
		{
			Name: "list_monitors",
			Description: `List all recurring URL check monitors for the organization. Monitors automatically check a set of URLs on a schedule (e.g. every 6 hours) and can auto-export to Google Sheets.`,
			InputSchema: map[string]any{
				"type":       "object",
				"properties": map[string]any{},
			},
		},
		{
			Name: "create_monitor",
			Description: `Create a recurring URL check monitor. The monitor will automatically check the specified URLs at the given interval. Results can be auto-exported to Google Sheets and/or sent to a webhook. Requires a paid plan. Minimum interval is 30 minutes.`,
			InputSchema: map[string]any{
				"type":     "object",
				"required": []string{"name", "urls", "interval_minutes"},
				"properties": map[string]any{
					"name":             map[string]any{"type": "string", "description": "Monitor name (e.g. 'Top pages weekly check')"},
					"urls":             map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "URLs to check on each run"},
					"interval_minutes": map[string]any{"type": "integer", "description": "Check interval in minutes. Common values: 30 (every 30 min), 360 (every 6 hours), 1440 (daily), 10080 (weekly)"},
					"webhook_url":      map[string]any{"type": "string", "description": "HTTPS URL to POST results to after each run"},
					"sheets_append":    map[string]any{"type": "boolean", "description": "Auto-export results to Google Sheets after each run"},
				},
			},
		},
		{
			Name: "export_to_sheets",
			Description: `Export batch results to Google Sheets. By default, appends to the organization's single default spreadsheet (creating it on first use). Requires Google Sheets to be connected in the app settings. The batch must be completed before exporting -- use batch_check_and_wait to ensure this.`,
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
		return client.BatchResults(jobID, page)

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

	default:
		return nil, fmt.Errorf("unknown tool: %s", name)
	}
}

// simplifyResult strips a full inspect response down to the key fields
// that matter for most conversations. Keeps the response small and readable.
func simplifyResult(full map[string]any) map[string]any {
	simple := map[string]any{
		"original_url": full["original_url"],
		"final_url":    full["final_url"],
		"final_status": full["final_status"],
		"total_hops":   full["total_hops"],
		"total_time_ms": full["total_time_ms"],
	}

	if e, ok := full["error"]; ok && e != nil {
		simple["error"] = e
	}

	// Include a one-line chain summary instead of the full hops array.
	if hops, ok := full["hops"].([]any); ok && len(hops) > 0 {
		chain := make([]string, 0, len(hops))
		for _, h := range hops {
			if hop, ok := h.(map[string]any); ok {
				status := hop["status_code"]
				url := hop["url"]
				chain = append(chain, fmt.Sprintf("%v %v", status, url))
			}
		}
		simple["chain"] = chain
	}

	return simple
}
