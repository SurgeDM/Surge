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
	trimmed := strings.TrimSpace(input)
	lower := strings.ToLower(trimmed)
	if lower == "" || lower == "0" || lower == "\u221e" || lower == "unlimited" {
		return 0, nil
	}

	trimmed = strings.ReplaceAll(trimmed, " ", "")

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

	unitStrLower := strings.ToLower(unitStr)
	lookupKey := strings.TrimSuffix(unitStrLower, "/s")

	unit, ok := rateUnits[lookupKey]
	if !ok {
		return 0, fmt.Errorf("unknown rate limit unit %q (accepted: B, KB, MB, GB, etc.)", unitStr)
	}

	// Capital-B *ps suffixes (GBps, MBps, KBps, Bps) are bytes per second,
	// not bits. This must be checked before the /s-suffix heuristics below.
	if strings.HasSuffix(unitStr, "Bps") && !strings.Contains(unitStr, "/") {
		unit.isBits = false
	}

	// Check original unitStr for 'b' vs 'B' to distinguish bits from bytes
	if strings.HasSuffix(unitStr, "bit/s") || strings.HasSuffix(unitStr, "bits/s") {
		unit.isBits = true
	} else if strings.HasSuffix(unitStr, "b/s") && !strings.HasSuffix(unitStr, "B/s") {
		// e.g. "Mb/s" vs "MB/s", but "B/s" is bytes
		unit.isBits = true
	} else if unitStrLower == "b/s" && !strings.HasPrefix(unitStr, "B") {
		// lowercase 'b' (e.g. "b/s", "b/S") means bits; uppercase 'B' means bytes
		unit.isBits = true
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

// FormatSpeed formats a live speed value (unlike FormatRateLimit, 0 means "0 B/s", not "infinity")
func FormatSpeed(speedBps float64) string {
	if speedBps <= 0 {
		return "0 B/s"
	}
	return ConvertBytesToHumanReadable(int64(math.Round(speedBps))) + "/s"
}

// IsRateLimitInherit checks if the string is one of the recognized aliases for inheriting the default limit.
func IsRateLimitInherit(s string) bool {
	normalized := strings.ToLower(strings.TrimSpace(s))
	return normalized == "inherit" || normalized == "default" || normalized == "-1"
}
