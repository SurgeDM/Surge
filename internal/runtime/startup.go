package runtime

import (
	"path/filepath"

	"github.com/surge-downloader/surge/internal/config"
	"github.com/surge-downloader/surge/internal/engine/state"
	"github.com/surge-downloader/surge/internal/utils"
)

// InitializeState configures the process-wide state database and logging.
func InitializeState(settings *config.Settings) error {
	if settings == nil {
		settings = config.DefaultSettings()
	}

	if err := config.EnsureDirs(); err != nil {
		return err
	}

	state.Configure(filepath.Join(config.GetStateDir(), "surge.db"))
	utils.ConfigureDebug(config.GetLogsDir())
	utils.CleanupLogs(settings.General.LogRetentionCount)
	return nil
}
