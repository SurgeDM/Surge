package utils

import "testing"

func TestConvertBytesToHumanReadable(t *testing.T) {
	tests := []struct {
		name  string
		bytes int64
		want  string
	}{
		{"Zero", 0, "0 B"},
		{"Negative", -1, "0 B"},
		{"Bytes", 500, "500 B"},
		// go-humanize Bytes uses base-1000 for formatting but we want to make sure we know what it outputs
		{"Kilobytes 1000", 1000, "1.0 kB"},
		{"Kilobytes 1024", 1024, "1.0 kB"},
		{"Megabytes 1e6", 1000000, "1.0 MB"},
		{"Megabytes 2^20", 1048576, "1.0 MB"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ConvertBytesToHumanReadable(tt.bytes); got != tt.want {
				t.Errorf("ConvertBytesToHumanReadable(%d) = %q, want %q", tt.bytes, got, tt.want)
			}
		})
	}
}

func BenchmarkConvertBytesToHumanReadable(b *testing.B) {
	sizes := []int64{0, 512, 1024, 1500000, 1024 * 1024 * 1024}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ConvertBytesToHumanReadable(sizes[i%len(sizes)])
	}
}
