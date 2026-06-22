package tui

import "github.com/SurgeDM/Surge/internal/config"

// Keys is a convenience alias for tests that need to reference keymap fields
// without constructing a full RootModel.
var Keys = *config.DefaultKeyMap()
