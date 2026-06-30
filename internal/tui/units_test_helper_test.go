package tui

import (
	"github.com/SurgeDM/Surge/internal/types"
	"github.com/SurgeDM/Surge/internal/utils"
)

// Unit constants re-exported for use in test files within this package.
const (
	KB                    = utils.KiB
	MB                    = utils.MiB
	GB                    = utils.GiB
	ProgressChannelBuffer = types.ProgressChannelBuffer
)
