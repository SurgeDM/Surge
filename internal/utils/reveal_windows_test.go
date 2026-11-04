//go:build windows

package utils

import "testing"

func TestBuildRevealCommand_SelectsFileInExplorer(t *testing.T) {
	cmd := buildRevealCommand(`D:\Download\My Files\report 1.pdf`)
	if cmd == nil || cmd.SysProcAttr == nil {
		t.Fatal("expected an Explorer command with a raw command line")
	}
	want := `explorer.exe /select,"D:\Download\My Files\report 1.pdf"`
	if cmd.SysProcAttr.CmdLine != want {
		t.Errorf("CmdLine = %q, want %q", cmd.SysProcAttr.CmdLine, want)
	}
}
