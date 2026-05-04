package config

import (
	"net"
	"testing"
)

func TestDNSPresets(t *testing.T) {
	// Test 1: Preset list is complete
	if len(dnsPresets) < 4 {
		t.Errorf("Expected at least 4 presets, got %d", len(dnsPresets))
	}

	// Verify "Custom" is the last element (contract for cycling logic)
	if len(dnsPresets) == 0 || dnsPresets[len(dnsPresets)-1].Name != "Custom" {
		t.Fatalf("Expected 'Custom' to be the last preset")
	}

	requiredNames := map[string]bool{
		"Cloudflare":        false,
		"Cloudflare Family": false,
		"AdGuard":           false,
		"Google":            false,
		"Custom":            false,
	}

	// Test 2: No duplicate names
	seenNames := make(map[string]bool)
	var customCount int
	for _, p := range dnsPresets {
		if seenNames[p.Name] {
			t.Errorf("Duplicate preset name found: %s", p.Name)
		}
		seenNames[p.Name] = true
		if _, ok := requiredNames[p.Name]; ok {
			requiredNames[p.Name] = true
		}

		if p.Name == "Custom" {
			customCount++
			// Test 4: Custom preset has nil Servers
			if p.Servers != nil {
				t.Errorf("Custom preset should have nil Servers, got %v", p.Servers)
			}
		} else {
			// Test 3: Server IPs are valid format
			for _, srv := range p.Servers {
				if net.ParseIP(srv) == nil {
					t.Errorf("Invalid IP for preset %s: %s", p.Name, srv)
				}
			}
		}
	}

	for name, found := range requiredNames {
		if !found {
			t.Errorf("Missing required preset: %s", name)
		}
	}

	if customCount != 1 {
		t.Errorf("Expected exactly 1 Custom preset, got %d", customCount)
	}
}

func TestMatchDNSPreset(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"1.1.1.1, 1.0.0.1", "Cloudflare"},
		{"1.1.1.1,1.0.0.1", "Cloudflare"},
		{"8.8.8.8, 8.8.4.4", "Google"},
		{"94.140.14.14", "Custom"}, // only one of the two AdGuard IPs
		{"", "Custom"},
		{"192.168.1.1", "Custom"},
	}

	for _, tt := range tests {
		result := MatchDNSPreset(tt.input)
		if result.Name != tt.expected {
			t.Errorf("MatchDNSPreset(%q) = %s; want %s", tt.input, result.Name, tt.expected)
		}
	}
}

func TestGetNextDNSPreset(t *testing.T) {
	// Use the first non-Custom preset as the starting point.
	first := dnsPresets[0]
	if first.Name == "Custom" {
		t.Fatal("Expected first preset to be a named provider, not Custom")
	}

	// Test the full cycle
	currentIP := first.IPString()
	for i := 0; i < len(dnsPresets); i++ {
		nextIP, isCustom := GetNextDNSPreset(currentIP)
		expectedPreset := dnsPresets[(i+1)%len(dnsPresets)]
		
		if nextIP != expectedPreset.IPString() {
			t.Errorf("Step %d: expected IP %q, got %q", i, expectedPreset.IPString(), nextIP)
		}
		
		expectedIsCustom := expectedPreset.Name == "Custom"
		if isCustom != expectedIsCustom {
			t.Errorf("Step %d: expected isCustom=%v, got %v", i, expectedIsCustom, isCustom)
		}
		
		if isCustom {
			currentIP = "1.2.3.4" // Simulate an arbitrary custom value for the next cycle step
		} else {
			currentIP = nextIP
		}
	}
}
