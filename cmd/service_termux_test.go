//go:build android

package cmd

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSvServiceName(t *testing.T) {
	assert.Equal(t, "surge", svServiceName())
}

func TestTermuxServiceDirSVDIRFallback(t *testing.T) {
	origSVDIR := os.Getenv("SVDIR")
	origSurgeSvDir := os.Getenv("SURGE_SV_DIR")
	origPrefix := os.Getenv("PREFIX")
	t.Cleanup(func() {
		os.Setenv("SVDIR", origSVDIR)
		os.Setenv("SURGE_SV_DIR", origSurgeSvDir)
		os.Setenv("PREFIX", origPrefix)
	})

	os.Unsetenv("SURGE_SV_DIR")
	os.Setenv("SVDIR", "/custom/sv")
	os.Setenv("PREFIX", "/data/data/com.termux/files/usr")
	assert.Equal(t, "/custom/sv/surge", termuxServiceDir())
}

func TestTermuxServiceDirSurgeSvDirOverride(t *testing.T) {
	origSVDIR := os.Getenv("SVDIR")
	origSurgeSvDir := os.Getenv("SURGE_SV_DIR")
	t.Cleanup(func() {
		os.Setenv("SVDIR", origSVDIR)
		os.Setenv("SURGE_SV_DIR", origSurgeSvDir)
	})

	os.Setenv("SURGE_SV_DIR", "/override/sv")
	os.Setenv("SVDIR", "/should/be/overridden")
	assert.Equal(t, "/override/sv/surge", termuxServiceDir())
}

func TestTermuxServiceDirDefault(t *testing.T) {
	origSVDIR := os.Getenv("SVDIR")
	origSurgeSvDir := os.Getenv("SURGE_SV_DIR")
	origPrefix := os.Getenv("PREFIX")
	t.Cleanup(func() {
		os.Setenv("SVDIR", origSVDIR)
		os.Setenv("SURGE_SV_DIR", origSurgeSvDir)
		os.Setenv("PREFIX", origPrefix)
	})

	os.Unsetenv("SURGE_SV_DIR")
	os.Unsetenv("SVDIR")
	os.Setenv("PREFIX", "/data/data/com.termux/files/usr")
	result := termuxServiceDir()
	assert.Equal(t, "/data/data/com.termux/files/usr/var/service/surge", result)
}
