package orchestrator

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/SurgeDM/Surge/internal/config"
	"github.com/SurgeDM/Surge/internal/store"
	"github.com/SurgeDM/Surge/internal/types"
	"github.com/SurgeDM/Surge/internal/utils"
)

// postDownloadCommand resolves explicit category assignments before automatic rules.
func postDownloadCommand(settings *config.Settings, record *types.DownloadRecord) string {
	if settings == nil || record == nil {
		return ""
	}
	var category *config.Category
	if record.CategoryOverride != nil {
		for i := range settings.Categories.Categories {
			if settings.Categories.Categories[i].Name == *record.CategoryOverride {
				category = &settings.Categories.Categories[i]
				break
			}
		}
	} else if config.Resolve[bool](settings.Categories.CategoryEnabled) {
		category, _ = config.GetCategoryForFile(record.Filename, settings.Categories.Categories)
	}
	if category == nil {
		return ""
	}
	return strings.TrimSpace(category.PostDownloadCommand)
}

func postDownloadProcess(command, destPath string) (*exec.Cmd, error) {
	absolute, err := filepath.Abs(destPath)
	if err != nil {
		return nil, err
	}
	dir, filename := filepath.Dir(absolute), filepath.Base(absolute)
	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.Command("powershell.exe", "-NoProfile", "-NonInteractive", "-Command", "$d=$env:SURGE_HOOK_DIR; $f=$env:SURGE_HOOK_FILE; "+command)
	} else {
		cmd = exec.Command("sh", "-c", command)
	}
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "d="+dir, "f="+filename, "SURGE_HOOK_DIR="+dir, "SURGE_HOOK_FILE="+filename)
	return cmd, nil
}

func runPostDownload(command, destPath string) error {
	cmd, err := postDownloadProcess(command, destPath)
	if err != nil {
		return err
	}
	return cmd.Run()
}

func (mgr *LifecycleManager) startPostDownload(command, path, id string) {
	if command == "" || path == "" {
		return
	}
	update := func(running bool, message string) {
		store.SetHookState(id, store.HookState{Running: running, Processed: !running, Error: message})
		if mgr.eventBus != nil {
			_ = mgr.eventBus.Publish(types.DownloadEvent{Type: types.EventSystem, DownloadID: id, Processing: &running, HookError: message, Message: message})
		}
	}
	update(true, "")
	cmd, err := postDownloadProcess(command, path)
	if err == nil {
		err = cmd.Start()
	}
	report := func(err error) {
		message := ""
		if err != nil {
			message = fmt.Sprintf("Post-download command failed: %v", err)
			utils.Debug("%s: %s", id, message)
		}
		update(false, message)
	}
	if err != nil {
		report(err)
		return
	}
	go func() { report(cmd.Wait()) }()
}
