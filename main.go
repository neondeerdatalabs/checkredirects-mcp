// checkredirects-mcp is an MCP (Model Context Protocol) server for checkredirects.io.
// It lets AI assistants like Claude inspect redirect chains, run batch checks,
// manage monitors, and export to Google Sheets via the checkredirects.io API.
//
// Install:
//
//	go install github.com/jasonhwolf/checkredirects-mcp@latest
//
// Configure in Claude Desktop's claude_desktop_config.json:
//
//	{
//	  "mcpServers": {
//	    "checkredirects": {
//	      "command": "checkredirects-mcp",
//	      "env": { "CHECKREDIRECTS_API_KEY": "httpd_your_key_here" }
//	    }
//	  }
//	}
package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"
)

const (
	defaultBaseURL = "https://api.checkredirects.io"
	serverName     = "checkredirects"
	serverVersion  = "1.0.0"
	protocolVersion = "2024-11-05"
)

// JSON-RPC types.
type jsonRPCRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      any             `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type jsonRPCResponse struct {
	JSONRPC string `json:"jsonrpc"`
	ID      any    `json:"id"`
	Result  any    `json:"result,omitempty"`
	Error   *jsonRPCError `json:"error,omitempty"`
}

type jsonRPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func main() {
	// Get API key from env or flag.
	apiKey := os.Getenv("CHECKREDIRECTS_API_KEY")
	if apiKey == "" {
		for i, arg := range os.Args {
			if arg == "--api-key" && i+1 < len(os.Args) {
				apiKey = os.Args[i+1]
			}
		}
	}

	// Warn if API key was passed on command line (visible in process list).
	for _, arg := range os.Args {
		if arg == "--api-key" {
			log.SetOutput(os.Stderr)
			log.Println("WARNING: passing --api-key on the command line exposes your key in the process list. Use CHECKREDIRECTS_API_KEY env var instead.")
			break
		}
	}

	baseURL := os.Getenv("CHECKREDIRECTS_API_URL")
	if baseURL == "" {
		baseURL = defaultBaseURL
	}

	// Only allow requests to checkredirects.io domains to prevent API key theft
	// via a malicious CHECKREDIRECTS_API_URL env var.
	if !strings.HasSuffix(strings.TrimRight(baseURL, "/"), ".checkredirects.io") && baseURL != "http://localhost:7070" {
		log.SetOutput(os.Stderr)
		log.Fatalf("CHECKREDIRECTS_API_URL must be a checkredirects.io domain (got %s)", baseURL)
	}

	var client *Client
	if apiKey != "" {
		client = NewClient(baseURL, apiKey)
	}

	// Log to stderr (stdout is the MCP transport).
	log.SetOutput(os.Stderr)
	log.Printf("checkredirects MCP server starting (api=%s)", baseURL)

	scanner := bufio.NewScanner(os.Stdin)
	// Increase max line size for large tool call results.
	scanner.Buffer(make([]byte, 0, 1024*1024), 1024*1024)

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var req jsonRPCRequest
		if err := json.Unmarshal(line, &req); err != nil {
			writeResponse(jsonRPCResponse{
				JSONRPC: "2.0",
				Error:   &jsonRPCError{Code: -32700, Message: "parse error"},
			})
			continue
		}

		resp := handleRequest(client, req)
		writeResponse(resp)
	}
}

func handleRequest(client *Client, req jsonRPCRequest) jsonRPCResponse {
	switch req.Method {
	case "initialize":
		return jsonRPCResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result: map[string]any{
				"protocolVersion": protocolVersion,
				"capabilities": map[string]any{
					"tools": map[string]any{},
				},
				"serverInfo": map[string]any{
					"name":    serverName,
					"version": serverVersion,
				},
			},
		}

	case "notifications/initialized":
		// No response needed for notifications.
		return jsonRPCResponse{JSONRPC: "2.0", ID: req.ID, Result: nil}

	case "tools/list":
		return jsonRPCResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result: map[string]any{
				"tools": AllTools(),
			},
		}

	case "tools/call":
		if client == nil {
			return jsonRPCResponse{
				JSONRPC: "2.0",
				ID:      req.ID,
				Result: map[string]any{
					"content": []map[string]any{
						{"type": "text", "text": "Error: CHECKREDIRECTS_API_KEY is not set. Set it as an environment variable or pass --api-key."},
					},
					"isError": true,
				},
			}
		}

		var params struct {
			Name      string         `json:"name"`
			Arguments map[string]any `json:"arguments"`
		}
		if err := json.Unmarshal(req.Params, &params); err != nil {
			return jsonRPCResponse{
				JSONRPC: "2.0",
				ID:      req.ID,
				Result: map[string]any{
					"content": []map[string]any{
						{"type": "text", "text": fmt.Sprintf("Error: invalid params: %v", err)},
					},
					"isError": true,
				},
			}
		}

		result, err := CallTool(client, params.Name, params.Arguments)
		if err != nil {
			return jsonRPCResponse{
				JSONRPC: "2.0",
				ID:      req.ID,
				Result: map[string]any{
					"content": []map[string]any{
						{"type": "text", "text": fmt.Sprintf("Error: %v", err)},
					},
					"isError": true,
				},
			}
		}

		resultJSON, _ := json.MarshalIndent(result, "", "  ")
		return jsonRPCResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result: map[string]any{
				"content": []map[string]any{
					{"type": "text", "text": string(resultJSON)},
				},
			},
		}

	default:
		return jsonRPCResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error:   &jsonRPCError{Code: -32601, Message: fmt.Sprintf("method not found: %s", req.Method)},
		}
	}
}

func writeResponse(resp jsonRPCResponse) {
	b, _ := json.Marshal(resp)
	fmt.Fprintf(os.Stdout, "%s\n", b)
}
