package config

import "strings"

// DNSPreset represents a common DNS provider configuration.
type DNSPreset struct {
	Name    string
	Servers []string // One or two IPs, matching the format expected by the engine.
}

// dnsPresets holds the list of supported DNS presets plus "Custom".
var dnsPresets = []DNSPreset{
	{Name: "Cloudflare", Servers: []string{"1.1.1.1", "1.0.0.1"}},
	{Name: "Cloudflare Family", Servers: []string{"1.1.1.3", "1.0.0.3"}},
	{Name: "AdGuard", Servers: []string{"94.140.14.14", "94.140.15.15"}},
	{Name: "Google", Servers: []string{"8.8.8.8", "8.8.4.4"}},
	{Name: "Custom", Servers: nil}, // Signals manual entry
}

// IPString returns the comma-separated IP string for the preset.
func (p DNSPreset) IPString() string {
	if p.Servers == nil {
		return ""
	}
	return strings.Join(p.Servers, ", ")
}

// MatchDNSPreset attempts to find a matching DNSPreset for a given comma-separated IP string.
// If it perfectly matches a known preset's servers, it returns that preset.
// Otherwise, it returns the Custom preset.
func MatchDNSPreset(ipStr string) DNSPreset {
	parts := strings.Split(ipStr, ",")
	var cleaned []string
	for _, p := range parts {
		trimmed := strings.TrimSpace(p)
		if trimmed != "" {
			cleaned = append(cleaned, trimmed)
		}
	}
	cleanedStr := strings.Join(cleaned, ",")

	for _, p := range dnsPresets {
		if p.Name == "Custom" {
			continue
		}
		if cleanedStr == strings.Join(p.Servers, ",") {
			return p
		}
	}

	// Fallback to Custom
	if len(dnsPresets) > 0 && dnsPresets[len(dnsPresets)-1].Name == "Custom" {
		return dnsPresets[len(dnsPresets)-1]
	}
	return DNSPreset{Name: "Custom", Servers: nil}
}

// GetNextDNSPreset given a current IP string, returns the next preset in the cycle.
// Also returns a boolean indicating if the next preset is "Custom".
func GetNextDNSPreset(currentIPStr string) (string, bool) {
	currentPreset := MatchDNSPreset(currentIPStr)
	currentIndex := 0
	for i, p := range dnsPresets {
		if p.Name == currentPreset.Name {
			currentIndex = i
			break
		}
	}

	nextIndex := (currentIndex + 1) % len(dnsPresets)
	nextPreset := dnsPresets[nextIndex]

	isCustom := nextPreset.Name == "Custom"
	return nextPreset.IPString(), isCustom
}
