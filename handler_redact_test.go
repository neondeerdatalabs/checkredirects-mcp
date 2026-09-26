package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestToolResultTextIsScrubbed pins the guarantee at the boundary that actually
// matters: the text handed back to the model.
//
// The unit test on scrubSensitiveHeaders passes even if the call in
// handleRequest is deleted, so on its own it proves the helper works and not
// that anything uses it. This drives a real tool call through handleRequest
// against a stub API and asserts the serialized MCP payload is clean — it fails
// if the scrub call is removed, which is the regression worth catching.
func TestToolResultTextIsScrubbed(t *testing.T) {
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// Shape mirrors a real inspect response: hop headers arrive as arrays.
		_, _ = w.Write([]byte(`{
			"final_url":"https://target.test/",
			"final_status":200,
			"hops":[{
				"url":"https://target.test/",
				"status_code":200,
				"headers":{
					"Set-Cookie":["session=LIVE_SECRET; HttpOnly"],
					"X-Auth-Token":["VENDOR_SECRET"],
					"Content-Type":["text/html"]
				}
			}]
		}`))
	}))
	defer api.Close()

	client := mustClient(t, api.URL)

	params, _ := json.Marshal(map[string]any{
		"name":      "inspect_url",
		"arguments": map[string]any{"url": "https://target.test/"},
	})
	resp := handleRequest(client, jsonRPCRequest{
		JSONRPC: "2.0",
		ID:      json.RawMessage(`1`),
		Method:  "tools/call",
		Params:  params,
	})

	payload, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("marshal response: %v", err)
	}
	text := string(payload)

	for _, secret := range []string{"LIVE_SECRET", "VENDOR_SECRET"} {
		if strings.Contains(text, secret) {
			t.Errorf("%s reached model-visible tool output:\n%s", secret, text)
		}
	}
	// The response must still be useful — scrubbing everything would also pass
	// the check above.
	if !strings.Contains(text, "target.test") {
		t.Errorf("expected the inspected URL to survive scrubbing:\n%s", text)
	}
}

// TestToolErrorDoesNotEchoUpstreamBody covers the path the scrub cannot reach:
// a non-JSON error response. Tool errors go into model-visible text without
// passing through scrubSensitiveHeaders, so the body must never be echoed.
func TestToolErrorDoesNotEchoUpstreamBody(t *testing.T) {
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte("upstream diagnostic: Set-Cookie: session=GATEWAY_SECRET"))
	}))
	defer api.Close()

	client := mustClient(t, api.URL)

	params, _ := json.Marshal(map[string]any{
		"name":      "inspect_url",
		"arguments": map[string]any{"url": "https://target.test/"},
	})
	resp := handleRequest(client, jsonRPCRequest{
		JSONRPC: "2.0",
		ID:      json.RawMessage(`1`),
		Method:  "tools/call",
		Params:  params,
	})

	payload, _ := json.Marshal(resp)
	if strings.Contains(string(payload), "GATEWAY_SECRET") {
		t.Errorf("upstream error body reached model-visible output:\n%s", payload)
	}
}

// TestToolErrorNeverEchoesRemoteText covers the harder case: a body that parses
// as our own error envelope but did not come from us.
//
// Parsing as JSON proves nothing about origin, and neither does the error code.
// This API sits behind Cloudflare, which can return its own error body for our
// own domain over a valid TLS connection, and nothing stops any proxy from
// choosing a plausible code. So a known code must be treated exactly like an
// unknown one: neither the message nor the code value is echoed, and the text
// the model sees is authored locally.
//
// An earlier version of this test asserted the opposite for known codes — that
// the message SHOULD be echoed for "rate_limited". That test passed while
// encoding the vulnerability, which is worth remembering: a green test only
// pins the behavior someone chose to assert.
func TestToolErrorNeverEchoesRemoteText(t *testing.T) {
	// Every code here must withhold, including the ones this API really does
	// issue. There is no allowlist any more.
	codes := []string{"rate_limited", "unauthorized", "forbidden", "waf_denied", ""}

	for _, code := range codes {
		name := code
		if name == "" {
			name = "empty"
		}
		t.Run(name, func(t *testing.T) {
			api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusForbidden)
				_ = json.NewEncoder(w).Encode(map[string]any{
					"error": map[string]any{
						"code":    code + "_MARKER_CODE",
						"message": "rejected request bearing PROXY_LEAKED_TOKEN",
					},
				})
			}))
			defer api.Close()

			params, _ := json.Marshal(map[string]any{
				"name":      "inspect_url",
				"arguments": map[string]any{"url": "https://target.test/"},
			})
			resp := handleRequest(mustClient(t, api.URL), jsonRPCRequest{
				JSONRPC: "2.0",
				ID:      json.RawMessage(`1`),
				Method:  "tools/call",
				Params:  params,
			})

			payload, _ := json.Marshal(resp)
			if strings.Contains(string(payload), "PROXY_LEAKED_TOKEN") {
				t.Errorf("code %q: upstream message reached model-visible output:\n%s", code, payload)
			}
			// The code value is upstream-controlled too, so it must not be
			// printed even when it is matched to pick a message.
			if strings.Contains(string(payload), "MARKER_CODE") {
				t.Errorf("code %q: upstream code value reached model-visible output:\n%s", code, payload)
			}
		})
	}
}

// TestTransportErrorRedactsCredentials pins the third model-visible path:
// errors from the HTTP client itself, which Go renders including the request
// URL and any transport diagnostic text.
func TestTransportErrorRedactsCredentials(t *testing.T) {
	tests := []struct {
		name string
		in   string
	}{
		{"api key in a query string", `Get "https://api.checkredirects.io/v1?k=httpnd_liveSECRET123": dial tcp: refused`},
		{"bearer token", `proxy rejected: Authorization: Bearer abc.DEF-123_x= denied`},
		{"jwt", `upstream said eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxIn0.sig was invalid`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := sanitizeErrorText(tt.in)
			for _, secret := range []string{"httpnd_liveSECRET123", "abc.DEF-123_x=", "eyJhbGciOiJIUzI1NiJ9"} {
				if strings.Contains(got, secret) {
					t.Errorf("credential survived sanitization: %q", got)
				}
			}
			// Asserts that sanitization DID something, without assuming how.
			// This used to require a literal "[redacted]" marker, which broke
			// when query stripping started removing the whole query rather than
			// rewriting the secret inside it — a strictly better outcome that
			// the old assertion scored as a failure. Comparing against the input
			// proves the pass ran without pinning the mechanism.
			if got == tt.in {
				t.Errorf("sanitizeErrorText returned its input unchanged: %q", got)
			}
		})
	}
}

// mustClient builds a client for a local httptest server. Tests go through the
// real constructor on purpose: a test-only bypass would be a hole in the exact
// boundary this file exists to defend.
func mustClient(t *testing.T, baseURL string) *Client {
	t.Helper()
	c, err := NewClient(baseURL, "httpnd_test")
	if err != nil {
		t.Fatalf("new client for %s: %v", baseURL, err)
	}
	return c
}
