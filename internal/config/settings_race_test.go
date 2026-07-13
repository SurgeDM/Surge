package config

import (
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/pelletier/go-toml/v2"
)

func TestSaveSettingsRace(t *testing.T) {
	// 1. Setup a temporary directory for isolated settings
	tempDir := t.TempDir()

	// Temporarily override GetSettingsPath to point to our temp dir
	// We do this by mocking it or changing the base dir.
	// However, Surge relies on GetSurgeDir(). We can just test writeTOMLAtomic directly!
	// SaveSettings calls writeTOMLAtomic(GetSettingsPath(), raw)

	targetFile := filepath.Join(tempDir, "settings.toml")

	// Run concurrent saves
	var wg sync.WaitGroup
	numGoroutines := 50
	errs := make(chan error, numGoroutines)

	t.Logf("Running %d concurrent writeTOMLAtomic calls...", numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			// Slightly mutate data to ensure serialization varies a tiny bit
			localData := map[string]interface{}{
				"general": map[string]interface{}{
					"theme":      id % 3,
					"auto_start": id%2 == 0,
					"theme_path": "/some/long/path/to/theme/to/ensure/payload/is/large/enough/to/trigger/race/conditions/during/write.toml",
				},
				"network": map[string]interface{}{
					"max_concurrent_downloads": id,
					"user_agent":               "Mozilla/5.0 (Race Condition Tester) Surge/1.0",
				},
			}

			if err := writeTOMLAtomic(targetFile, localData); err != nil {
				errs <- err
			}
		}(i)
	}

	wg.Wait()
	close(errs)

	for err := range errs {
		t.Errorf("writeTOMLAtomic returned error: %v", err)
	}

	// Now try to load and parse the file to ensure it's not corrupted
	// The exact final state is non-deterministic, but it MUST be valid TOML.
	// We'll use the package's existing LoadSettings if we mock the dir,
	// or just unmarshal it directly.

	data, err := os.ReadFile(targetFile)
	if err != nil {
		t.Fatalf("Failed to read resulting file: %v", err)
	}

	var verify map[string]interface{}
	if err := toml.Unmarshal(data, &verify); err != nil {
		t.Fatalf("RACE CONDITION DETECTED! Resulting TOML is corrupted and cannot be parsed: %v", err)
	}

	t.Log("Successfully verified that concurrent saves resulted in a valid TOML file.")
}
