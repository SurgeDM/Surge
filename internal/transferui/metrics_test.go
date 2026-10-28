package transferui

import (
	"math"
	"strings"
	"testing"
	"time"
)

func TestETAIncludesDaysAndRejectsUnknownOrInvalidSamples(t *testing.T) {
	if eta, known := ETA(3*86400, 0, 1); !known || eta != 72*time.Hour || Duration(eta) != "3d 0h" {
		t.Fatalf("long ETA=%v known=%v", eta, known)
	}
	for _, rate := range []float64{0, -1, math.NaN(), math.Inf(1)} {
		if _, known := ETA(100, 1, rate); known {
			t.Fatalf("invalid rate accepted: %v", rate)
		}
	}
	if _, known := ETA(0, 100, 1); known {
		t.Fatal("unknown size acquired an ETA")
	}
	if got := SafeText("file\x1b]0;title\a\x1b[31m\nname"); strings.ContainsAny(got, "\x1b\n\a") {
		t.Fatal("terminal control sequence survived")
	}
}
