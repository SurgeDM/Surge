package tui

import (
	"context"
	"testing"

	"charm.land/bubbles/v2/viewport"
	"github.com/SurgeDM/Surge/internal/config"
	"github.com/SurgeDM/Surge/internal/power"
	"github.com/SurgeDM/Surge/internal/types"
)

type fakePowerController struct {
	shutdowns int
	inhibits  int
	releases  int
}

func (f *fakePowerController) Shutdown(context.Context) error {
	f.shutdowns++
	return nil
}

func (f *fakePowerController) AcquireInhibitor(string) (power.ReleaseFunc, error) {
	f.inhibits++
	return func() error {
		f.releases++
		return nil
	}, nil
}

func newAutoShutdownTestModel(power *fakePowerController, downloads ...*DownloadModel) RootModel {
	settings := config.DefaultSettings()
	settings.General.AutoShutdownAfterDownloads.Value = true
	return RootModel{
		Settings:        settings,
		downloads:       downloads,
		list:            NewDownloadList(80, 20),
		logViewport:     viewport.New(viewport.WithWidth(40), viewport.WithHeight(5)),
		powerController: power,
	}
}

func TestAutoShutdown_TriggersWhenLastDownloadCompletes(t *testing.T) {
	power := &fakePowerController{}
	dm := NewDownloadModel("id-1", "https://example.com/file.bin", "file.bin", 100)
	dm.started = true
	m := newAutoShutdownTestModel(power, dm)

	m.applyAutoShutdownSettingChange()
	if !m.autoShutdownArmed {
		t.Fatal("expected auto-shutdown to arm when pending download exists")
	}
	if power.inhibits != 1 {
		t.Fatalf("inhibits = %d, want 1", power.inhibits)
	}

	updated, cmd := m.Update(types.DownloadEvent{
		Type:       types.EventComplete,
		DownloadID: "id-1",
		Filename:   "file.bin",
		Total:      100,
	})
	m2 := updated.(RootModel)
	if !m2.autoShutdownTriggered {
		t.Fatal("expected auto-shutdown to be marked triggered")
	}
	if power.releases != 1 {
		t.Fatalf("releases = %d, want 1", power.releases)
	}
	if cmd == nil {
		t.Fatal("expected shutdown command")
	}
	executeCmds(cmd)
	if power.shutdowns != 1 {
		t.Fatalf("shutdowns = %d, want 1", power.shutdowns)
	}

	updated, cmd = m2.Update(types.DownloadEvent{
		Type:       types.EventComplete,
		DownloadID: "id-1",
		Filename:   "file.bin",
		Total:      100,
	})
	_ = updated
	executeCmds(cmd)
	if power.shutdowns != 1 {
		t.Fatalf("shutdowns after duplicate event = %d, want 1", power.shutdowns)
	}
}

func TestAutoShutdown_PausedDownloadBlocksShutdown(t *testing.T) {
	power := &fakePowerController{}
	active := NewDownloadModel("id-active", "https://example.com/active.bin", "active.bin", 100)
	active.started = true
	paused := NewDownloadModel("id-paused", "https://example.com/paused.bin", "paused.bin", 100)
	paused.paused = true
	m := newAutoShutdownTestModel(power, active, paused)
	m.applyAutoShutdownSettingChange()

	updated, cmd := m.Update(types.DownloadEvent{
		Type:       types.EventComplete,
		DownloadID: "id-active",
		Filename:   "active.bin",
		Total:      100,
	})
	m2 := updated.(RootModel)

	if m2.autoShutdownTriggered {
		t.Fatal("expected paused download to block shutdown")
	}
	executeCmds(cmd)
	if power.shutdowns != 0 {
		t.Fatalf("shutdowns = %d, want 0", power.shutdowns)
	}
}

func TestAutoShutdown_DisablingSettingReleasesInhibitor(t *testing.T) {
	power := &fakePowerController{}
	dm := NewDownloadModel("id-1", "https://example.com/file.bin", "file.bin", 100)
	m := newAutoShutdownTestModel(power, dm)
	m.applyAutoShutdownSettingChange()

	if power.inhibits != 1 {
		t.Fatalf("inhibits = %d, want 1", power.inhibits)
	}

	if err := m.setSettingValue("General", "auto_shutdown_after_downloads", "false"); err != nil {
		t.Fatalf("setSettingValue failed: %v", err)
	}
	if m.autoShutdownArmed {
		t.Fatal("expected auto-shutdown to disarm")
	}
	if power.releases != 1 {
		t.Fatalf("releases = %d, want 1", power.releases)
	}
	if config.Resolve[bool](m.Settings.General.AutoShutdownAfterDownloads) {
		t.Fatal("expected setting to be false")
	}
}
