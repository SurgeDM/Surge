package tui

import (
	"time"

	"github.com/SurgeDM/Surge/internal/engine/types"
)

const (
	// === Timeouts and Intervals ===
	TickInterval = 200 * time.Millisecond

	// === Layout Ratios ===
	ListWidthRatio         = 0.6  // Dashboard: List takes 60% width
	SettingsWidthRatio     = 0.72 // Modals: Settings/Category use 72% width
	LogoWidthRatio         = 0.45 // Header: Logo takes 45% of left column
	GraphTargetHeightRatio = 0.4  // Right Column: Graph target 40% height

	// === Thresholds and Minimums ===
	MinTermWidth             = 45
	MinTermHeight            = 12
	ShortTermHeightThreshold = 25 // Switch to compact header below this height

	MinSettingsWidth      = 64
	MaxSettingsWidth      = 130
	MinSettingsHeight     = 12
	DefaultSettingsHeight = 26

	MinRightColumnWidth = 50 // Hide right column if narrow
	MinGraphStatsWidth  = 70 // Hide inline graph stats if narrow
	MinLogoWidth        = 60 // Hide ASCII logo if narrow

	MinGraphHeight      = 9
	MinGraphHeightShort = 5
	MinListHeight       = 10
	MinChunkMapHeight   = 4
	MinChunkMapVisibleH = 18 // Min term height to show chunk map

	// === Component Heights ===
	ModalHeightPadding = 4 // Bottom fallback padding for modals to avoid clipping
	HeaderHeightMax    = 11
	HeaderHeightMin    = 3
	FilePickerHeight   = 12
	CardHeight         = 2 // Compact rows for downloads list

	// === Padding and Offsets ===
	DefaultPaddingX = 1
	DefaultPaddingY = 0
	PopupPaddingX   = 2
	PopupPaddingY   = 1
	PopupWidth      = 70 // Consistent width for small popup dialogs

	HeaderWidthOffset      = 2
	ProgressBarWidthOffset = 4

	// === Graph Configuration ===
	GraphAxisWidth  = 10
	GraphStatsWidth = 18
	GraphHeadroom   = 1.1 // Scale max speed by 110% for visual headroom

	// === Input Dimensions ===
	InputWidth        = 40
	MinSettingsInputW = 8
	MaxSettingsInputW = 48

	// === Channel Buffers ===
	ProgressChannelBuffer = types.ProgressChannelBuffer

	// === Units ===
	KB = types.KB
	MB = types.MB
	GB = types.GB
)
