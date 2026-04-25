package tui

// layout_regression_test.go – enforces that no TUI component ever produces
// output that exceeds the terminal width or height it was given.
//
// These tests are the authoritative "no cutting off" regression suite.
// Any future change that causes a line to exceed the terminal width OR adds
// more rendered lines than the terminal height will break these tests.

import (
	"bytes"
	"fmt"
	"regexp"
	"strings"
	"testing"

	"charm.land/bubbles/v2/list"
	"charm.land/lipgloss/v2"
	"github.com/SurgeDM/Surge/internal/processing"
	"github.com/SurgeDM/Surge/internal/tui/components"
	"github.com/SurgeDM/Surge/internal/version"
)

var stripANSI = regexp.MustCompile(`\x1b\[[0-9;]*m`)

// plainLines strips ANSI codes and splits on newlines.
func plainLines(s string) []string {
	return strings.Split(stripANSI.ReplaceAllString(s, ""), "\n")
}

// assertNoLineExceedsWidth fails if any rendered line is wider than wantWidth.
func assertNoLineExceedsWidth(t *testing.T, label string, output string, wantWidth int) {
	t.Helper()
	for i, line := range plainLines(output) {
		if w := lipgloss.Width(line); w > wantWidth {
			// Show a helpful excerpt of the offending line.
			excerpt := line
			if len(excerpt) > 120 {
				excerpt = excerpt[:120] + "…"
			}
			t.Errorf("[%s] line %d: width %d > max %d\n  %q", label, i, w, wantWidth, excerpt)
		}
	}
}

// assertHeightNotExceeded fails if the rendered output has more non-trailing lines
// than wantHeight allows (trailing empty lines are excluded from the count).
func assertHeightNotExceeded(t *testing.T, label string, output string, wantHeight int) {
	t.Helper()
	lines := plainLines(strings.TrimRight(output, "\n"))
	if len(lines) > wantHeight {
		t.Errorf("[%s] height %d > max %d", label, len(lines), wantHeight)
	}
}

// termSizes covers a representative range of terminal geometries including
// corner cases that historically produced overflows.
var termSizes = []struct{ width, height int }{
	{160, 50}, // extra wide, tall
	{140, 40}, // full layout
	{120, 35}, // standard large
	{100, 30}, // medium
	{80, 24},  // classic 80×24
	{72, 20},  // narrow modal threshold
	{60, 20},  // right column hidden
	{50, 16},  // very narrow
	{46, 14},  // minimum useful width
	{45, 12},  // MinTermWidth boundary
}

// ─────────────────────────────────────────────────────────────
// 1. Dashboard – all sizes
// ─────────────────────────────────────────────────────────────

func TestLayout_DashboardWidthNeverExceeds(t *testing.T) {
	m := InitialRootModel(1701, "test", nil, processing.NewLifecycleManager(nil, nil), false)
	for _, tc := range termSizes {
		m.width = tc.width
		m.height = tc.height
		label := fmt.Sprintf("dashboard %dx%d", tc.width, tc.height)
		view := m.View()
		assertNoLineExceedsWidth(t, label, view.Content, tc.width)
	}
}

func TestLayout_DashboardHeightNeverExceeds(t *testing.T) {
	m := InitialRootModel(1701, "test", nil, processing.NewLifecycleManager(nil, nil), false)
	for _, tc := range termSizes {
		m.width = tc.width
		m.height = tc.height
		label := fmt.Sprintf("dashboard %dx%d", tc.width, tc.height)
		view := m.View()
		assertHeightNotExceeded(t, label, view.Content, tc.height)
	}
}

// ─────────────────────────────────────────────────────────────
// 2. Dashboard with a selected download that has a very long URL
// ─────────────────────────────────────────────────────────────

func TestLayout_DashboardLongURLWidthNeverExceeds(t *testing.T) {
	longURL := "https://cdn.example.com/releases/v1.2.3/" + strings.Repeat("a", 300) + "/ubuntu-22.04.iso"
	m := InitialRootModel(1701, "test", nil, processing.NewLifecycleManager(nil, nil), false)
	m.downloads = []*DownloadModel{
		{
			ID:          "dl-1",
			URL:         longURL,
			Filename:    strings.Repeat("very-long-filename-", 10) + ".iso",
			Destination: "/home/user/" + strings.Repeat("deep/nested/path/", 8),
			Total:       1 << 30,
			Downloaded:  512 << 20,
			Speed:       10 * MB,
		},
	}

	for _, tc := range termSizes {
		m.width = tc.width
		m.height = tc.height
		m.UpdateListItems()
		label := fmt.Sprintf("dashboard+longURL %dx%d", tc.width, tc.height)
		view := m.View()
		assertNoLineExceedsWidth(t, label, view.Content, tc.width)
	}
}

// ─────────────────────────────────────────────────────────────
// 3. All modal states – width & height
// ─────────────────────────────────────────────────────────────

var modalStates = []struct {
	name  string
	state UIState
}{
	{"QuitConfirm", QuitConfirmState},
	{"HelpModal", HelpModalState},
	{"DuplicateWarning", DuplicateWarningState},
	{"BatchConfirm", BatchConfirmState},
	{"UpdateAvailable", UpdateAvailableState},
	{"BugReportTarget", BugReportTargetState},
	{"BugReportSystemDetails", BugReportSystemDetailsState},
	{"BugReportLogPath", BugReportLogPathState},
}

func TestLayout_AllModalsWidthNeverExceeds(t *testing.T) {
	for _, ms := range modalStates {
		for _, tc := range termSizes {
			t.Run(fmt.Sprintf("%s_%dx%d", ms.name, tc.width, tc.height), func(t *testing.T) {
				m := InitialRootModel(1701, "test", nil, processing.NewLifecycleManager(nil, nil), false)
				m.width = tc.width
				m.height = tc.height
				m.state = ms.state

				// UpdateAvailableState requires UpdateInfo
				if ms.state == UpdateAvailableState {
					m.UpdateInfo = &version.UpdateInfo{LatestVersion: "9.9.9", CurrentVersion: "1.0.0"}
				}
				// DuplicateWarningState benefits from a long detail string
				if ms.state == DuplicateWarningState {
					m.duplicateInfo = "https://very.long.domain.example.com/path/to/very/deep/file.iso"
				}
				// BatchConfirm needs pending URLs
				if ms.state == BatchConfirmState {
					m.pendingBatchURLs = []string{"url1", "url2"}
					m.batchFilePath = "/very/long/path/to/" + strings.Repeat("dir/", 10) + "urls.txt"
				}

				view := m.View()
				assertNoLineExceedsWidth(t,
					fmt.Sprintf("%s %dx%d", ms.name, tc.width, tc.height),
					view.Content, tc.width)
			})
		}
	}
}

func TestLayout_AllModalsHeightNeverExceeds(t *testing.T) {
	for _, ms := range modalStates {
		for _, tc := range termSizes {
			t.Run(fmt.Sprintf("%s_%dx%d", ms.name, tc.width, tc.height), func(t *testing.T) {
				m := InitialRootModel(1701, "test", nil, processing.NewLifecycleManager(nil, nil), false)
				m.width = tc.width
				m.height = tc.height
				m.state = ms.state

				if ms.state == UpdateAvailableState {
					m.UpdateInfo = &version.UpdateInfo{LatestVersion: "9.9.9", CurrentVersion: "1.0.0"}
				}
				if ms.state == DuplicateWarningState {
					m.duplicateInfo = "https://very.long.domain.example.com/very/deep/path/file.iso"
				}
				if ms.state == BatchConfirmState {
					m.pendingBatchURLs = []string{"url1", "url2"}
				}

				view := m.View()
				assertHeightNotExceeded(t,
					fmt.Sprintf("%s %dx%d", ms.name, tc.width, tc.height),
					view.Content, tc.height)
			})
		}
	}
}

// ─────────────────────────────────────────────────────────────
// 4. Settings & Category Manager – width & height
// ─────────────────────────────────────────────────────────────

func TestLayout_SettingsWidthNeverExceeds(t *testing.T) {
	m := InitialRootModel(1701, "test", nil, processing.NewLifecycleManager(nil, nil), false)
	m.state = SettingsState
	for _, tc := range termSizes {
		m.width = tc.width
		m.height = tc.height
		label := fmt.Sprintf("settings %dx%d", tc.width, tc.height)
		view := m.View()
		assertNoLineExceedsWidth(t, label, view.Content, tc.width)
	}
}

func TestLayout_SettingsHeightNeverExceeds(t *testing.T) {
	m := InitialRootModel(1701, "test", nil, processing.NewLifecycleManager(nil, nil), false)
	m.state = SettingsState
	for _, tc := range termSizes {
		m.width = tc.width
		m.height = tc.height
		label := fmt.Sprintf("settings %dx%d", tc.width, tc.height)
		view := m.View()
		assertHeightNotExceeded(t, label, view.Content, tc.height)
	}
}

func TestLayout_CategoryManagerWidthNeverExceeds(t *testing.T) {
	m := InitialRootModel(1701, "test", nil, processing.NewLifecycleManager(nil, nil), false)
	m.state = CategoryManagerState
	for _, tc := range termSizes {
		m.width = tc.width
		m.height = tc.height
		label := fmt.Sprintf("catmgr %dx%d", tc.width, tc.height)
		view := m.View()
		assertNoLineExceedsWidth(t, label, view.Content, tc.width)
	}
}

func TestLayout_CategoryManagerHeightNeverExceeds(t *testing.T) {
	m := InitialRootModel(1701, "test", nil, processing.NewLifecycleManager(nil, nil), false)
	m.state = CategoryManagerState
	for _, tc := range termSizes {
		m.width = tc.width
		m.height = tc.height
		label := fmt.Sprintf("catmgr %dx%d", tc.width, tc.height)
		view := m.View()
		assertHeightNotExceeded(t, label, view.Content, tc.height)
	}
}

// ─────────────────────────────────────────────────────────────
// 5. Download list delegate – Render width enforcement
// ─────────────────────────────────────────────────────────────

func TestLayout_DelegateRenderNeverExceedsWidth(t *testing.T) {
	d := newDownloadDelegate()

	widths := []int{200, 120, 80, 60, 45, 30}
	for _, w := range widths {
		t.Run(fmt.Sprintf("width_%d", w), func(t *testing.T) {
			m := list.New([]list.Item{}, d, w, 20)
			di := DownloadItem{
				download: &DownloadModel{
					ID:       "dl-abc",
					Filename: strings.Repeat("very-long-filename-", 12) + ".iso",
					URL:      "https://example.com/" + strings.Repeat("a", 400),
					Total:    1 << 30,
					Speed:    5 * MB,
				},
				spinnerView: "⠋",
			}

			var buf bytes.Buffer
			d.Render(&buf, m, 0, di)
			rendered := stripANSI.ReplaceAllString(buf.String(), "")
			for i, line := range strings.Split(rendered, "\n") {
				if lw := lipgloss.Width(line); lw > w {
					t.Errorf("line %d: width %d > max %d in rendered delegate at width %d", i, lw, w, w)
				}
			}
		})
	}
}

// ─────────────────────────────────────────────────────────────
// 6. RenderBtopBox – width invariant at all widths
// ─────────────────────────────────────────────────────────────

func TestLayout_RenderBtopBoxWidthInvariant(t *testing.T) {
	longLeft := " " + strings.Repeat("Left Title Text ", 8) + " "
	longRight := " " + strings.Repeat("Right Title ", 8) + " "
	content := strings.Repeat("x", 400)

	widths := []int{200, 120, 80, 60, 45, 30, 15, 5, 3}
	heights := []int{20, 10, 5, 3, 1}

	for _, w := range widths {
		for _, h := range heights {
			for _, tc := range []struct {
				name, left, right string
			}{
				{"no-titles", "", ""},
				{"left-only", longLeft, ""},
				{"right-only", "", longRight},
				{"both-long", longLeft, longRight},
				{"both-short", " L ", " R "},
			} {
				label := fmt.Sprintf("box_%s_%dx%d", tc.name, w, h)
				out := components.RenderBtopBox(tc.left, tc.right, content, w, h, nil)
				plain := stripANSI.ReplaceAllString(out, "")
				for i, line := range strings.Split(plain, "\n") {
					if lw := lipgloss.Width(line); lw > w {
						t.Errorf("[%s] line %d width=%d > box width=%d", label, i, lw, w)
					}
				}
				// Height: top border + content rows + bottom border
				outLines := strings.Split(plain, "\n")
				// Strip trailing empty line that JoinVertical may add
				for len(outLines) > 0 && outLines[len(outLines)-1] == "" {
					outLines = outLines[:len(outLines)-1]
				}
				// RenderBtopBox always produces exactly h lines.
				// A box has a minimum of 3 lines (top border, content rows, bottom border),
				// so for h < 3 the output will be 3 lines — that is expected behaviour.
				expectedLines := h
				if expectedLines < 3 {
					expectedLines = 3
				}
				if len(outLines) != expectedLines {
					t.Errorf("[%s] height=%d != expected=%d", label, len(outLines), expectedLines)
				}
			}
		}
	}
}

// ─────────────────────────────────────────────────────────────
// 7. Extreme sizes – no panic
// ─────────────────────────────────────────────────────────────

func TestLayout_ExtremeSizesNoPanic(t *testing.T) {
	extremes := []struct{ w, h int }{
		{1, 1},
		{2, 2},
		{45, 1},
		{1, 50},
		{0, 0},
		{500, 200},
	}

	allStates := append(modalStates,
		struct {
			name  string
			state UIState
		}{"Dashboard", DashboardState},
		struct {
			name  string
			state UIState
		}{"Settings", SettingsState},
		struct {
			name  string
			state UIState
		}{"CategoryManager", CategoryManagerState},
	)

	for _, tc := range extremes {
		for _, ms := range allStates {
			t.Run(fmt.Sprintf("%s_%dx%d", ms.name, tc.w, tc.h), func(t *testing.T) {
				m := InitialRootModel(1701, "test", nil, processing.NewLifecycleManager(nil, nil), false)
				m.width = tc.w
				m.height = tc.h
				m.state = ms.state
				if ms.state == UpdateAvailableState {
					m.UpdateInfo = &version.UpdateInfo{LatestVersion: "9.9.9", CurrentVersion: "1.0.0"}
				}
				// Must not panic
				defer func() {
					if r := recover(); r != nil {
						t.Errorf("panic for state=%s at %dx%d: %v", ms.name, tc.w, tc.h, r)
					}
				}()
				_ = m.View()
			})
		}
	}
}

// ─────────────────────────────────────────────────────────────
// 8. CalculateDashboardLayout – sum invariants
// ─────────────────────────────────────────────────────────────

func TestLayout_CalculateDashboardLayout_SumInvariants(t *testing.T) {
	sizes := []struct{ w, h int }{
		{160, 50}, {120, 35}, {80, 24}, {60, 20}, {46, 14},
	}
	for _, tc := range sizes {
		l := CalculateDashboardLayout(tc.w, tc.h)
		label := fmt.Sprintf("%dx%d", tc.w, tc.h)

		// Left + Right should equal AvailableWidth (when right column is shown)
		if !l.HideRightColumn {
			if l.LeftWidth+l.RightWidth != l.AvailableWidth {
				t.Errorf("[%s] LeftWidth(%d)+RightWidth(%d) != AvailableWidth(%d)",
					label, l.LeftWidth, l.RightWidth, l.AvailableWidth)
			}
		}

		// ListHeight should not exceed AvailableHeight
		if l.ListHeight > l.AvailableHeight {
			t.Errorf("[%s] ListHeight(%d) > AvailableHeight(%d)", label, l.ListHeight, l.AvailableHeight)
		}

		// GraphHeight + DetailHeight (+ ChunkMapHeight) should not exceed AvailableHeight
		if !l.HideRightColumn {
			total := l.GraphHeight + l.DetailHeight
			if l.ShowChunkMap {
				total += l.ChunkMapHeight
			}
			if total > l.AvailableHeight {
				t.Errorf("[%s] right column heights sum(%d) > AvailableHeight(%d)",
					label, total, l.AvailableHeight)
			}
		}

		// All dimensions must be non-negative
		for name, val := range map[string]int{
			"AvailableWidth":  l.AvailableWidth,
			"AvailableHeight": l.AvailableHeight,
			"LeftWidth":       l.LeftWidth,
			"ListHeight":      l.ListHeight,
			"HeaderHeight":    l.HeaderHeight,
		} {
			if val < 0 {
				t.Errorf("[%s] %s is negative: %d", label, name, val)
			}
		}
	}
}

// ─────────────────────────────────────────────────────────────
// 9. GetDynamicModalDimensions – output never exceeds terminal
// ─────────────────────────────────────────────────────────────

func TestLayout_GetDynamicModalDimensions_BoundedByTerminal(t *testing.T) {
	cases := []struct {
		termW, termH, minW, minH, prefW, contentH int
	}{
		{120, 35, 40, 8, 80, 20},
		{60, 20, 40, 8, 80, 20}, // pref > term
		{45, 12, 40, 8, 80, 20}, // very narrow
		{10, 5, 40, 8, 80, 20},  // smaller than min (should still not exceed term)
		{200, 60, 50, 10, 72, 15},
	}
	for _, tc := range cases {
		w, h := GetDynamicModalDimensions(tc.termW, tc.termH, tc.minW, tc.minH, tc.prefW, tc.contentH)
		label := fmt.Sprintf("term=%dx%d pref=%dx%d", tc.termW, tc.termH, tc.prefW, tc.contentH)
		if tc.termW > 0 && w > tc.termW {
			t.Errorf("[%s] modal width %d > termW %d", label, w, tc.termW)
		}
		if tc.termH > 0 && h > tc.termH {
			t.Errorf("[%s] modal height %d > termH %d", label, h, tc.termH)
		}
		if w < 1 {
			t.Errorf("[%s] modal width %d < 1", label, w)
		}
		if h < 1 {
			t.Errorf("[%s] modal height %d < 1", label, h)
		}
	}
}
