package main

import (
	"encoding/json"
	"strings"
	"testing"
)

// TestModelVisibleResultsCarryNoQueryStrings pins the other half of
// CHECKREDIR-51.
//
// The choke point removed auth-bearing HEADERS and nothing else, which
// addressed the wrong half. Every tool result carries the customer's URLs —
// original_url, final_url, and each hop — and those hold
// session tokens, reset tokens, signed-URL signatures and email addresses in
// the query string. They went to the model provider verbatim.
//
// Shaped as a real API response, nested the way inspect and compare-agents
// actually nest, because the scrub recurses and a flat fixture would not
// exercise that.
func TestModelVisibleResultsCarryNoQueryStrings(t *testing.T) {
	const secret = "s3cr3t-session-token"
	raw := `{
	  "original_url": "https://shop.example/checkout?session=` + secret + `",
	  "final_url": "https://shop.example/done#tok=` + secret + `",
	  "hops": [
	    {"url": "https://shop.example/a?token=` + secret + `", "status_code": 301,
	     "headers": {"Location": "https://shop.example/b?token=` + secret + `",
	                 "Set-Cookie": "sid=` + secret + `",
	                 "Content-Type": "text/html"}},
	    {"url": "https://shop.example/b", "status_code": 200}
	  ],
	  "agents": {
	    "googlebot": {"final_url": "https://shop.example/g?u=` + secret + `"}
	  },
	  "related_urls": ["https://shop.example/c?t=` + secret + `"],
	  "status_message": "Moved? Yes.",
	  "notes": "no url here, just a question mark? and text"
	}`

	var result any
	if err := json.Unmarshal([]byte(raw), &result); err != nil {
		t.Fatalf("fixture is not valid JSON: %v", err)
	}
	scrubSensitiveHeaders(result)

	out, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	got := string(out)

	if strings.Contains(got, secret) {
		t.Errorf("the session token reached the model conversation:\n%s", got)
	}

	// The parts that make the tool useful must survive.
	for _, want := range []string{
		"shop.example/checkout", "shop.example/done", "shop.example/a",
		"shop.example/g", "shop.example/c", "text/html", "301",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("scrubbing destroyed %q, which carries no secret:\n%s", want, got)
		}
	}

	// Ordinary text containing a question mark is not a URL and must not be
	// mangled — a scrub that quietly rewrites prose is worse than one that
	// misses, because nobody notices.
	if !strings.Contains(got, "Moved? Yes.") {
		t.Errorf("a status message was mangled as if it were a URL:\n%s", got)
	}
	if !strings.Contains(got, "just a question mark? and text") {
		t.Errorf("ordinary prose was mangled:\n%s", got)
	}
}

// TestStripQueryFromURLValueIsConservative pins the boundary between "this is a
// URL" and "this is text that happens to contain punctuation".
func TestStripQueryFromURLValueIsConservative(t *testing.T) {
	tests := []struct {
		in          string
		want        string
		wantChanged bool
	}{
		{"https://a.example/p?x=1", "https://a.example/p", true},
		{"http://a.example/p#f", "http://a.example/p", true},
		{"https://user:pw@a.example/p", "https://a.example/p", true},
		{"https://a.example/clean", "https://a.example/clean", false},
		{"not a url at all", "not a url at all", false},
		{"why? because.", "why? because.", false},
		// Prose must survive. The pattern needs a // to fire at all, so ordinary
		// sentences with question marks are untouched.
		{"is it 50% off? yes", "is it 50% off? yes", false},
		{"ratio a/b?", "ratio a/b?", false},
		// Any scheme's query is stripped now, because supporting
		// protocol-relative URLs (//host/path — what a Location header often
		// carries) means matching the //host portion whatever precedes it. A
		// query on an ftp or ws URL can carry a secret just as readily.
		{"ftp://a.example/p?x=1", "ftp://a.example/p", true},
		{"ws://a.example/s?token=t", "ws://a.example/s", true},
		{"//a.example/p?t=1", "//a.example/p", true},
		{"HTTPS://a.example/p?x=1", "https://a.example/p", true},
		{"fetch https://a.example/p?x=1 failed", "fetch https://a.example/p failed", true},
		{"", "", false},
		{"https://", "https://", false},
	}
	for _, tc := range tests {
		got, changed := stripQueryFromURLValue(tc.in)
		if got != tc.want || changed != tc.wantChanged {
			t.Errorf("stripQueryFromURLValue(%q) = (%q, %v), want (%q, %v)",
				tc.in, got, changed, tc.want, tc.wantChanged)
		}
	}
}

// TestCodexMcpFindings pins four leaks a tightly-scoped review found, each
// reproduced before it was fixed.
func TestCodexMcpFindings(t *testing.T) {
	const secret = "s3cr3t-session-token"

	t.Run("credential pass no longer corrupts the url pass", func(t *testing.T) {
		// The passes ran credentials-first, so on `Bearer https://h/p?t=x` the
		// pattern matched "Bearer https" and left "[redacted]://h/p?t=x" —
		// which no longer looked like a URL, so the query survived.
		got := sanitizeErrorText("Bearer https://h.example/p?t=" + secret)
		if strings.Contains(got, secret) {
			t.Errorf("the query survived because the credential pass ran first: %q", got)
		}
	})

	t.Run("uppercase scheme", func(t *testing.T) {
		if got, _ := stripQueryFromURLValue("HTTPS://h.example/p?t=" + secret); strings.Contains(got, secret) {
			t.Errorf("HTTPS:// is legal and was not matched: %q", got)
		}
	})

	t.Run("protocol-relative url", func(t *testing.T) {
		// What a Location header frequently carries.
		if got, _ := stripQueryFromURLValue("//h.example/p?t=" + secret); strings.Contains(got, secret) {
			t.Errorf("a protocol-relative URL was not matched: %q", got)
		}
	})

	t.Run("url inside a longer message", func(t *testing.T) {
		in := "failed to fetch https://h.example/p?t=" + secret + " after 3 tries"
		if got, _ := stripQueryFromURLValue(in); strings.Contains(got, secret) {
			t.Errorf("a URL wrapped in prose kept its query: %q", got)
		}
	})

	t.Run("multi-value header", func(t *testing.T) {
		// Headers can be multi-valued, in which case the API returns []any.
		// Only the string case was handled.
		var v any
		if err := json.Unmarshal([]byte(
			`{"headers":{"Link":["https://h.example/p?t=`+secret+`"]}}`), &v); err != nil {
			t.Fatalf("fixture: %v", err)
		}
		scrubSensitiveHeaders(v)
		out, _ := json.Marshal(v)
		if strings.Contains(string(out), secret) {
			t.Errorf("a multi-valued header kept its query: %s", out)
		}
	})

	t.Run("custom credential header names", func(t *testing.T) {
		// Their values are not URL-shaped, so nothing else would redact them.
		for _, name := range []string{
			"X-API-Key", "API-Key", "X-Company-Session", "X-Csrf",
			"X-Amzn-Oidc-Data", "X-Amzn-Oidc-Identity", "Cf-Access-Jwt-Assertion",
			"X-Goog-Iap-Jwt-Assertion", "X-Ms-Token-Aad-Access-Token",
		} {
			if !isSensitiveHeader(name) {
				t.Errorf("%s reaches the model with its value intact", name)
			}
		}
	})
}
