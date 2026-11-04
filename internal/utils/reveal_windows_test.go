//go:build windows

package utils

import "testing"

func TestBuildRevealCommand_SelectsFileInExplorer(t *testing.T) {
	for _, path := range []string{
		`D:\Download\My Files\report 1.pdf`,
		`D:/Download/My Files/report 1.pdf`,
	} {
		cmd := buildRevealCommand(path)
		if cmd == nil || cmd.SysProcAttr == nil {
			t.Fatal("expected an Explorer command with a raw command line")
		}
		want := `explorer.exe /select,"D:\Download\My Files\report 1.pdf"`
		if cmd.SysProcAttr.CmdLine != want {
			t.Errorf("CmdLine for %q = %q, want %q", path, cmd.SysProcAttr.CmdLine, want)
		}
	}
}
