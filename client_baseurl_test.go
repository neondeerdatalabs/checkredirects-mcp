package main

import (
	"strings"
	"testing"
)

// TestBaseURLAllowlist pins CHECKREDIR-14.
//
// The previous check was strings.HasSuffix on the raw URL, performed by main
// rather than by the constructor that attaches the API key. Every "must reject"
// case below PASSED it — confirmed by running the old expression, not inferred.
// The first one sends a customer's API key to a host of the attacker's
// choosing; the cleartext one sends it unencrypted.
func TestBaseURLAllowlist(t *testing.T) {
	tests := []struct {
		name    string
		url     string
		wantErr bool
	}{
		{"canonical api host", "https://api.checkredirects.io", false},
		{"apex", "https://checkredirects.io", false},
		{"trailing slash tolerated", "https://api.checkredirects.io/", false},
		{"dev loopback over http", "http://localhost:7070", false},
		{"dev loopback by ip", "http://127.0.0.1:7070", false},

		// Every case below defeated the old suffix check.
		{"allowlisted name hidden in the path", "https://attacker.example/.checkredirects.io", true},
		{"allowlisted name in a fragment", "https://evil.example#.checkredirects.io", true},
		{"allowlisted name in a query", "https://evil.example?x=.checkredirects.io", true},
		{"cleartext to the real host", "http://api.checkredirects.io", true},

		// Shapes the old check happened to reject, pinned so the rewrite does
		// not quietly widen the allowlist.
		{"lookalike suffix", "https://notcheckredirects.io", true},
		{"lookalike with separator", "https://checkredirects.io.evil.example", true},
		{"credentials in the url", "https://user:pw@api.checkredirects.io", true},
		{"unrelated host", "https://example.com", true},
		// Loopback on ANY port is allowed on purpose: an address resolving to
		// this machine is not an attacker-controlled host, so the threat this
		// allowlist exists to stop does not apply. Pinning one port bought no
		// security and would have forced tests onto a bypass constructor.
		{"loopback on any port", "http://localhost:9999", false},
		{"loopback by ipv4 on any port", "http://127.0.0.1:54321", false},
		{"ipv6 loopback", "http://[::1]:8080", false},

		// Widening loopback makes these the cases that matter: a hostname that
		// merely contains or ends with a loopback name is a remote host.
		{"loopback lookalike suffix", "https://127.0.0.1.attacker.example", true},
		{"localhost lookalike suffix", "https://localhost.attacker.example", true},
		{"localhost as a subdomain label", "https://attacker.example.localhost.evil.example", true},
		{"empty", "", true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := NewClient(tc.url, "httpnd_secret")
			if (err != nil) != tc.wantErr {
				t.Errorf("NewClient(%q) error = %v, wantErr %v", tc.url, err, tc.wantErr)
			}
		})
	}
}

// TestRejectedBaseURLYieldsNoClient pins that a refusal cannot leave a usable
// client behind. Returning a client alongside an error would let a caller that
// ignores the error send the key anyway.
func TestRejectedBaseURLYieldsNoClient(t *testing.T) {
	c, err := NewClient("https://attacker.example/.checkredirects.io", "httpnd_secret")
	if err == nil {
		t.Fatal("the attacker host was accepted")
	}
	if c != nil {
		t.Error("a client was returned alongside the error, so a caller ignoring err " +
			"would still send the API key to the attacker's host")
	}
}

func TestRejectedBaseURLDoesNotEchoCredentials(t *testing.T) {
	const secret = "operator-password"
	_, err := NewClient("https://user:"+secret+"@api.checkredirects.io?token="+secret, "httpnd_secret")
	if err == nil {
		t.Fatal("credential-bearing base URL was accepted")
	}
	if strings.Contains(err.Error(), secret) {
		t.Fatalf("rejected base URL leaked credentials into its error: %v", err)
	}
}
