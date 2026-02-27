package utils

import "testing"

func BenchmarkConvertBytesToHumanReadable(b *testing.B) {
	sizes := []int64{0, 512, 1024, 1500000, 1024 * 1024 * 1024}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ConvertBytesToHumanReadable(sizes[i%len(sizes)])
	}
}
