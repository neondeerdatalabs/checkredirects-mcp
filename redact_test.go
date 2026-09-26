package main

import (
	"encoding/json"
	"strings"
	"testing"
)

// TestScrubRemovesAuthHeadersAtEveryDepth pins the guarantee that no
// auth-bearing header from an inspected site reaches the model.
//
// The nesting matters as much as the header names. inspect returns hops at the
// top level, compare-agents nests them under per-agent results, and batch nests
// them under checks — three shapes today and no reason to think that is the
// last one. The scrubber matches the "headers" key wherever it appears, so this
// test asserts all three depths rather than the one that happened to be written
// first.
func TestScrubRemovesAuthHeadersAtEveryDepth(t *testing.T) {
	raw := `{
	  "final_url":"https://x.test",
	  "hops":[{"url":"https://x.test","headers":{"Set-Cookie":"sid=SECRET1","Content-Type":"text/html"}}],
	  "results":[{"hops":[{"headers":{"WWW-Authenticate":"Basic realm=SECRET2","Server":"nginx"}}]}],
	  "checks":[{"hops":[{"headers":{"authorization":"Bearer SECRET3"}}]}]
	}`

	var v any
	if err := json.Unmarshal([]byte(raw), &v); err != nil {
		t.Fatalf("fixture is not valid JSON: %v", err)
	}
	scrubSensitiveHeaders(v)

	out, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal after scrub: %v", err)
	}
	got := string(out)

	// Header names are matched case-insensitively, hence the mixed casing above.
	for _, secret := range []string{"SECRET1", "SECRET2", "SECRET3"} {
		if strings.Contains(got, secret) {
			t.Errorf("%s reached model-visible output: %s", secret, got)
		}
	}

	// Over-scrubbing is its own failure: the caller asked to see the response.
	for _, keep := range []string{"text/html", "nginx", "final_url"} {
		if !strings.Contains(got, keep) {
			t.Errorf("scrub removed non-sensitive value %q: %s", keep, got)
		}
	}
}

// TestScrubHandlesMalformedShapes guards the recursion against input that is
// not the shape we expect — a "headers" key holding a string or null, which a
// future API change or an error payload could produce. Panicking here would
// take down the MCP server on a malformed response.
func TestScrubHandlesMalformedShapes(t *testing.T) {
	for _, raw := range []string{
		`{"headers":"not-an-object"}`,
		`{"headers":null}`,
		`{"hops":[null,{"headers":{}}]}`,
		`[]`,
		`null`,
	} {
		var v any
		if err := json.Unmarshal([]byte(raw), &v); err != nil {
			t.Fatalf("fixture %q invalid: %v", raw, err)
		}
		scrubSensitiveHeaders(v) // must not panic
	}
}
