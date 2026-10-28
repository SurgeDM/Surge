// Package transferui contains pure, shared transfer presentation rules.
package transferui

import (
	"fmt"
	"math"
	"strings"
	"time"
	"unicode"

	"github.com/charmbracelet/x/ansi"
)

func FiniteRate(rate float64) bool { return rate >= 0 && !math.IsNaN(rate) && !math.IsInf(rate, 0) }
func ETA(total, downloaded int64, rate float64) (time.Duration, bool) {
	if total <= 0 || rate <= 0 || !FiniteRate(rate) {
		return 0, false
	}
	seconds := float64(max(0, total-downloaded)) / rate
	if seconds > float64(math.MaxInt64)/float64(time.Second) {
		return 0, false
	}
	return time.Duration(seconds * float64(time.Second)), true
}
func Duration(value time.Duration) string {
	if value >= 24*time.Hour {
		return fmt.Sprintf("%dd %dh", int64(value.Hours())/24, int64(value.Hours())%24)
	}
	if value >= time.Hour {
		return fmt.Sprintf("%dh %dm", int64(value.Hours()), int64(value.Minutes())%60)
	}
	if value >= time.Minute {
		return fmt.Sprintf("%dm %ds", int64(value.Minutes()), int64(value.Seconds())%60)
	}
	return fmt.Sprintf("%ds", max(0, int64(value.Seconds())))
}
func Bytes(value int64) string { return ByteValue(float64(max(0, value))) }
func ByteValue(value float64) string {
	if !FiniteRate(value) {
		return "--"
	}
	units := []string{"B", "KiB", "MiB", "GiB", "TiB", "PiB", "EiB", "ZiB"}
	index := 0
	for value >= 1024 && index < len(units)-1 {
		value /= 1024
		index++
	}
	if index == 0 {
		return fmt.Sprintf("%.0f B", value)
	}
	return fmt.Sprintf("%.2f %s", value, units[index])
}
func SafeText(value string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsControl(r) || r == 0x7f {
			return -1
		}
		return r
	}, ansi.Strip(value))
}
