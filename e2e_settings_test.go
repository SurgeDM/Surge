package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestSettingsPersistenceAfterRebuild(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skipf("Skipping test because 'go' executable is not found in PATH: %v", err)
	}

	binName := "surge_test_bin"
	if runtime.GOOS == "windows" {
		binName += ".exe"
	}

	// 1. Build surge
	t.Log("Building Surge...")
	buildCmd := exec.Command("go", "build", "-o", binName, "main.go")
	if err := buildCmd.Run(); err != nil {
		t.Fatalf("Failed to build surge: %v", err)
	}
	defer func() { _ = os.Remove(binName) }()

	// Ensure clean state and avoid nuking user settings by using a temporary home dir
	tempDir := t.TempDir()

	// Prepare custom environment for child processes
	customEnv := append(os.Environ(), "XDG_CONFIG_HOME="+tempDir, "XDG_DATA_HOME="+tempDir, "SURGE_HOME="+tempDir, "APPDATA="+tempDir)

	// 2. Starts it & 3. Changes a setting & 4. Closes it
	// Using `surge_test_bin config General.Theme 2` accomplishes all of these,
	// as it spins up the config manager, changes the setting, and exits.
	t.Log("Changing General.Theme to 2...")
	configCmd := exec.Command(filepath.Join(".", binName), "config", "General.Theme", "2")
	configCmd.Env = customEnv
	if err := configCmd.Run(); err != nil {
		t.Fatalf("Failed to change setting: %v", err)
	}

	// Verify it was set
	verifyCmd := exec.Command(filepath.Join(".", binName), "config", "General.Theme")
	verifyCmd.Env = customEnv
	verifyOut, err := verifyCmd.CombinedOutput()
	if err != nil {
		t.Fatalf("Failed to verify setting: %v", err)
	}
	if !strings.Contains(string(verifyOut), "2") {
		t.Fatalf("Expected setting to be '2' initially, got: %s", string(verifyOut))
	}

	// 5. Changes one line in code
	t.Log("Changing one line in code (main.go)...")
	mainContent, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatalf("Failed to read main.go: %v", err)
	}

	// Add a comment to the end of main.go
	modifiedContent := append(mainContent, []byte("\n// Test modification for persistence check\n")...)
	if err := os.WriteFile("main.go", modifiedContent, 0644); err != nil {
		t.Fatalf("Failed to modify main.go: %v", err)
	}

	// Revert the change at the end of the test
	defer func() {
		if err := os.WriteFile("main.go", mainContent, 0644); err != nil {
			t.Errorf("failed to restore main.go: %v", err)
		}
	}()

	// 6. Builds it again
	t.Log("Building Surge again after code change...")
	rebuildCmd := exec.Command("go", "build", "-o", binName, "main.go")
	if err := rebuildCmd.Run(); err != nil {
		t.Fatalf("Failed to rebuild surge: %v", err)
	}

	// 7. Opens surge & 8. Checks if the setting it changed before is there or not
	t.Log("Checking if setting persisted...")
	checkCmd := exec.Command(filepath.Join(".", binName), "config", "General.Theme")
	checkCmd.Env = customEnv
	checkOut, err := checkCmd.CombinedOutput()
	if err != nil {
		t.Fatalf("Failed to check setting after rebuild: %v", err)
	}

	if !strings.Contains(string(checkOut), "2") {
		t.Fatalf("Setting did NOT persist! Expected to find '2', got:\n%s", string(checkOut))
	}

	t.Log("Success: Setting persisted after modifying code and rebuilding!")
}
