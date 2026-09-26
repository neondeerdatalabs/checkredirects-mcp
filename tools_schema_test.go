package main

import (
	"strings"
	"testing"
)

func toolByName(t *testing.T, name string) Tool {
	t.Helper()
	for _, tool := range AllTools() {
		if tool.Name == name {
			return tool
		}
	}
	t.Fatalf("tool %q not advertised", name)
	return Tool{}
}

func props(t *testing.T, tool Tool) map[string]any {
	t.Helper()
	p, ok := tool.InputSchema["properties"].(map[string]any)
	if !ok {
		t.Fatalf("tool %q has no properties object", tool.Name)
	}
	return p
}

// The schemas here are the contract a model reads before deciding what to send,
// so an inaccurate one produces confident, wrong calls rather than a usable
// error. compare_agents in particular advertised https_only, which
// compareAgentsRequest does not declare -- and the API's Decode helper rejects
// unknown fields, so every model that believed the schema got a 400.
func TestCompareAgentsSchemaMatchesTheAPI(t *testing.T) {
	p := props(t, toolByName(t, "compare_agents"))

	if _, found := p["https_only"]; found {
		t.Error("compare_agents advertises https_only, but compareAgentsRequest does not declare it " +
			"and unknown fields are rejected with 400")
	}

	pack, ok := p["pack"].(map[string]any)
	if !ok {
		t.Fatal("compare_agents has no pack property")
	}
	enum, ok := pack["enum"].([]string)
	if !ok {
		t.Fatal("pack has no enum; a model can then send a pack key that does not exist")
	}
	// Kept in step with internal/useragents/catalog.go by the CI cross-check in
	// .github/workflows/ci.yml -- this module cannot import that package, so
	// this list alone could only ever confirm its own assumption.
	want := []string{"seo_essentials", "mobile_vs_desktop", "social_preview", "ai_crawler_audit", "full_coverage"}
	if strings.Join(enum, ",") != strings.Join(want, ",") {
		t.Errorf("pack enum = %v, want %v", enum, want)
	}
}

// Every user-agent key named as an example has to be a real catalog key, or the
// example itself teaches a model to send something the API will reject.
func TestAdvertisedUserAgentExamplesAreRealKeys(t *testing.T) {
	// Retired or never-existent keys that previously appeared in these schemas.
	for _, tool := range AllTools() {
		blob := tool.Description
		for _, v := range props(t, tool) {
			if m, ok := v.(map[string]any); ok {
				if d, ok := m["description"].(string); ok {
					blob += " " + d
				}
			}
		}
		for _, dead := range []string{"chrome_desktop", "social_crawlers"} {
			if strings.Contains(blob, dead) {
				t.Errorf("tool %q still advertises %q, which is not a catalog key", tool.Name, dead)
			}
		}
	}
}
