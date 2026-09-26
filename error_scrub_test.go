package main

import (
	"strings"
	"testing"
)

// TestErrorTextIsSanitizedBeforeReachingTheModel pins the gap in
// CHECKREDIR-51's existing fix.
//
// handleRequest has a choke point that strips auth-bearing headers from every
// tool result, with a comment saying a future tool cannot forget it. The error
// branch returns before reaching that point, so an error carrying a customer
// URL or a token went to the model provider verbatim.
//
// The credential pattern alone was also not enough: it matched API keys, bearer
// tokens and JWTs, and missed the commoner shape — a URL whose query string
// carries the secret.
func TestErrorTextIsSanitizedBeforeReachingTheModel(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		absent  []string
		present []string
	}{
		{
			name:    "session token in a query string",
			in:      `Get "https://customer.example/reset?token=s3cr3t": dial tcp: refused`,
			absent:  []string{"s3cr3t", "token="},
			present: []string{"customer.example"},
		},
		{
			name:   "email in a query string",
			in:     `fetch https://a.example/x?email=jane%40corp.com failed`,
			absent: []string{"jane", "corp.com"},
		},
		{
			name:   "api key",
			in:     "unauthorized for httpnd_livekey123456",
			absent: []string{"httpnd_livekey123456"},
		},
		{
			name:   "bearer token",
			in:     "upstream said Bearer abc123.def456 was invalid",
			absent: []string{"abc123.def456"},
		},
		{
			name:   "fragment",
			in:     "redirect to https://a.example/p#access_token=zzz",
			absent: []string{"zzz", "access_token"},
		},
		{
			name:    "ordinary error survives intact",
			in:      "connection reset by peer",
			present: []string{"connection reset by peer"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := sanitizeErrorText(tc.in)
			for _, s := range tc.absent {
				if strings.Contains(got, s) {
					t.Errorf("%q survived sanitization: %q", s, got)
				}
			}
			for _, s := range tc.present {
				if !strings.Contains(got, s) {
					t.Errorf("%q was destroyed, making the error useless for debugging: %q", s, got)
				}
			}
		})
	}
}
