package main

import "testing"

// TestWithheldCredentialsSurviveSimplification pins CHECKREDIR-88 on the MCP
// side.
//
// simplifyResult strips hops, so a per-hop marker vanishes with them. These
// tools accept cookies and basic_auth, so a caller genuinely can supply
// credentials — and without the signal a model sees a bare 401 and tells the
// user their credentials are wrong, when in fact we withheld them because the
// chain left the origin they were supplied for.
func TestWithheldCredentialsSurviveSimplification(t *testing.T) {
	full := map[string]any{
		"original_url": "https://app.example.com/dashboard",
		"final_url":    "https://cdn.other.example/login",
		"final_status": float64(401),
		"total_hops":   float64(2),
		"hops": []any{
			map[string]any{"hop_number": float64(1), "status_code": float64(302)},
			map[string]any{
				"hop_number":                  float64(2),
				"status_code":                 float64(401),
				"credentials_withheld":        true,
				"credentials_withheld_reason": "origin_change",
			},
		},
	}

	simple := simplifyResult(full)

	if simple["credentials_withheld"] != true {
		t.Error("a 401 reached the model with no indication that we withheld the " +
			"credentials; the model would report the customer's credentials as wrong")
	}
	if got := simple["credentials_withheld_reason"]; got != "origin_change" {
		t.Errorf("credentials_withheld_reason = %v, want origin_change", got)
	}
	// The hops themselves still must not be carried — that is what simplify is for.
	if _, ok := simple["hops"]; ok {
		t.Error("simplifyResult started carrying full hops")
	}
}

// TestNoWithheldMarkerOnOrdinaryResults keeps the flag meaningful. If it
// appeared on every anonymous inspection it would be noise, and a model would
// learn to ignore it.
func TestNoWithheldMarkerOnOrdinaryResults(t *testing.T) {
	simple := simplifyResult(map[string]any{
		"original_url": "https://example.com/",
		"final_status": float64(200),
		"hops": []any{
			map[string]any{"hop_number": float64(1), "status_code": float64(200)},
		},
	})
	if _, present := simple["credentials_withheld"]; present {
		t.Error("credentials_withheld appeared on a result where nothing was withheld")
	}

	// A response with no hops at all must not panic or invent a marker.
	if _, present := simplifyResult(map[string]any{"final_status": float64(200)})["credentials_withheld"]; present {
		t.Error("credentials_withheld appeared on a result carrying no hops")
	}
}
