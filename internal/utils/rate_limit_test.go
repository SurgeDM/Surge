package utils

import (
	"testing"
)

func TestParseRateLimit(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    int64
		wantErr bool
	}{
		{"Empty string", "", 0, false},
		{"Zero", "0", 0, false},
		{"Default unit (MB/s)", "5", 5 * 1000 * 1000, false},
		{"Default unit fractional", "1.5", 1500000, false},
		{"Bytes", "500 b", 500, false},
		{"Bytes suffix", "500b", 500, false},
		{"Kilobytes", "2 kb", 2000, false},
		{"Kilobytes binary", "2 kib", 2048, false},
		{"Megabytes", "10 mb", 10 * 1000 * 1000, false},
		{"Megabytes binary", "10 mib", 10 * 1024 * 1024, false},
		{"Gigabytes", "1 gb", 1000 * 1000 * 1000, false},
		{"Gigabytes binary", "1 gib", 1024 * 1024 * 1024, false},
		{"Bits (bps)", "8000 bps", 1000, false},
		{"Kilobits (kbps)", "8000 kbps", 1000 * 1000, false},
		{"Megabits (mbps)", "8 mbps", 1000 * 1000, false},
		{"With /s suffix", "10 MB/s", 10 * 1000 * 1000, false},
		{"Spaces", "  10  mb  ", 10 * 1000 * 1000, false},
		{"Invalid value", "abc mb", 0, true},
		{"Invalid unit", "10 xyz", 0, true},
		{"Negative value", "-10 mb", 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseRateLimit(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseRateLimit() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("ParseRateLimit() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestFormatRateLimit(t *testing.T) {
	tests := []struct {
		name string
		bps  int64
		want string
	}{
		{"Zero", 0, "0"},
		{"Negative", -100, "0"},
		{"Bytes", 500, "500 B/s"},
		{"Kilobytes", 1000, "1.0 kB/s"},
		{"Megabytes", 1000000, "1.0 MB/s"},
		{"Gigabytes", 1000000000, "1.0 GB/s"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := FormatRateLimit(tt.bps); got != tt.want {
				t.Errorf("FormatRateLimit() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestParseRateLimitValue(t *testing.T) {
	tests := []struct {
		name    string
		input   any
		want    int64
		wantErr bool
	}{
		{"Nil", nil, 0, false},
		{"Int zero", 0, 0, false},
		{"Int positive", 500, 500, false},
		{"Int negative", -500, 0, true},
		{"Int64 positive", int64(500), 500, false},
		{"Int64 negative", int64(-500), 0, true},
		{"Float64 positive", float64(500.2), 500, false},
		{"Float64 negative", float64(-500.2), 0, true},
		{"String valid", "500 b", 500, false},
		{"String invalid", "invalid", 0, true},
		{"Unsupported type", struct{}{}, 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseRateLimitValue(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseRateLimitValue() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("ParseRateLimitValue() = %v, want %v", got, tt.want)
			}
		})
	}
}
