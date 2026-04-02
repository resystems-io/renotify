package config

import (
	"testing"
)

func TestRegistry_AllKeysResolve(t *testing.T) {
	cfg := Default()
	for _, p := range Registry {
		val := p.Resolve(cfg)
		if val == nil {
			t.Errorf("Registry key %q resolved to nil", p.Key)
		}
	}
}

func TestRegistry_Count(t *testing.T) {
	// Guard against accidentally dropping entries. Update this
	// count when adding new config parameters.
	const expected = 29
	if len(Registry) != expected {
		t.Errorf("Registry has %d entries, want %d",
			len(Registry), expected)
	}
}

func TestRegistry_NoDuplicateKeys(t *testing.T) {
	seen := make(map[string]bool, len(Registry))
	for _, p := range Registry {
		if seen[p.Key] {
			t.Errorf("duplicate registry key: %q", p.Key)
		}
		seen[p.Key] = true
	}
}

func TestRegistry_FormatDefault(t *testing.T) {
	cfg := Default()
	tests := []struct {
		key  string
		want string
	}{
		{"broker.tcp_port", "4222"},
		{"broker.enabled", "true"},
		{"heartbeat.interval", "30s"},
		{"jetstream.max_age", "30m0s"},
	}
	for _, tc := range tests {
		for _, p := range Registry {
			if p.Key == tc.key {
				got := p.FormatDefault(cfg)
				if got != tc.want {
					t.Errorf("FormatDefault(%q) = %q, want %q",
						tc.key, got, tc.want)
				}
				break
			}
		}
	}
}

func TestRegistry_MetadataComplete(t *testing.T) {
	for _, p := range Registry {
		if p.Key == "" {
			t.Error("registry entry with empty Key")
		}
		if p.Type == "" {
			t.Errorf("registry key %q has empty Type", p.Key)
		}
		if p.EnvVar == "" {
			t.Errorf("registry key %q has empty EnvVar", p.Key)
		}
		if p.Description == "" {
			t.Errorf("registry key %q has empty Description",
				p.Key)
		}
		if p.Resolve == nil {
			t.Errorf("registry key %q has nil Resolve", p.Key)
		}
	}
}
