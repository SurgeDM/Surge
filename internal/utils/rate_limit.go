package utils

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

type rateUnit struct {
	multiplier float64
	isBits     bool
}

var rateUnits = map[string]rateUnit{
	"b":     {multiplier: 1, isBits: false},
	"byte":  {multiplier: 1, isBits: false},
	"bytes": {multiplier: 1, isBits: false},

	"kb": {multiplier: 1e3, isBits: false},
	"mb": {multiplier: 1e6, isBits: false},
	"gb": {multiplier: 1e9, isBits: false},
	"tb": {multiplier: 1e12, isBits: false},

	"kib": {multiplier: 1024, isBits: false},
	"mib": {multiplier: 1024 * 1024, isBits: false},
	"gib": {multiplier: 1024 * 1024 * 1024, isBits: false},
	"tib": {multiplier: 1024 * 1024 * 1024 * 1024, isBits: false},

	"bps":  {multiplier: 1, isBits: true},
	"kbps": {multiplier: 1e3, isBits: true},
	"mbps": {multiplier: 1e6, isBits: true},
	"gbps": {multiplier: 1e9, isBits: true},
	"tbps": {multiplier: 1e12, isBits: true},

	"kbit": {multiplier: 1e3, isBits: true},
	"mbit": {multiplier: 1e6, isBits: true},
	"gbit": {multiplier: 1e9, isBits: true},
	"tbit": {multiplier: 1e12, isBits: true},
}

// ParseRateLimit parses a human-friendly rate limit string into bytes per second.
func ParseRateLimit(input string) (int64, error) {
	trimmed := strings.TrimSpace(strings.ToLower(input))
	if trimmed == "" || trimmed == "0" || trimmed == "\u221e" || trimmed == "unlimited" {
		return 0, nil
	}

	trimmed = strings.ReplaceAll(trimmed, " ", "")
	trimmed = strings.TrimSuffix(trimmed, "/s")

	numEnd := 0
	for numEnd < len(trimmed) {
		ch := trimmed[numEnd]
		if (ch >= '0' && ch <= '9') || ch == '.' {
			numEnd++
			continue
		}
		break
	}

	if numEnd == 0 {
		return 0, fmt.Errorf("rate limit missing numeric value")
	}

	numStr := trimmed[:numEnd]
	unitStr := trimmed[numEnd:]
	if unitStr == "" {
		return 0, fmt.Errorf("missing rate limit unit (e.g. KB/s, MB/s, B/s)")
	}

	value, err := strconv.ParseFloat(numStr, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid rate limit value")
	}
	if value < 0 {
		return 0, fmt.Errorf("rate limit must be non-negative")
	}

	unit, ok := rateUnits[unitStr]
	if !ok {
		return 0, fmt.Errorf("unknown rate limit unit %q (accepted: B, KB, MB, GB, etc.)", unitStr)
	}

	bytes := value * unit.multiplier
	if unit.isBits {
		bytes = bytes / 8
	}

	if bytes <= 0 {
		return 0, nil
	}
	if bytes > float64(math.MaxInt64) {
		return 0, fmt.Errorf("rate limit too large")
	}

	return int64(math.Round(bytes)), nil
}

func ParseRateLimitValue(val any) (int64, error) {
	switch v := val.(type) {
	case nil:
		return 0, nil
	case int:
		if v < 0 {
			return 0, fmt.Errorf("rate limit must be non-negative")
		}
		return int64(v), nil
	case int64:
		if v < 0 {
			return 0, fmt.Errorf("rate limit must be non-negative")
		}
		return v, nil
	case float64:
		if v < 0 {
			return 0, fmt.Errorf("rate limit must be non-negative")
		}
		if v > float64(math.MaxInt64) {
			return 0, fmt.Errorf("rate limit too large")
		}
		return int64(math.Round(v)), nil
	case string:
		return ParseRateLimit(v)
	default:
		return 0, fmt.Errorf("unsupported rate limit type")
	}
}

func FormatRateLimit(bps int64) string {
	if bps <= 0 {
		return "\u221E"
	}
	return ConvertBytesToHumanReadable(bps) + "/s"
}
