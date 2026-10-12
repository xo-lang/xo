package build

import (
	"path/filepath"
	"strings"
	"testing"
)

// TestLoadZoneinfoNoGo checks the error of an embedded zone data build
// with no go command: it names XO_ZONEINFO and --tzdata=system.
func TestLoadZoneinfoNoGo(t *testing.T) {
	t.Setenv("XO_ZONEINFO", "")
	_, err := loadZoneinfo(filepath.Join(t.TempDir(), "no-such-go"))
	if err == nil || !strings.Contains(err.Error(), "no go command") || !strings.Contains(err.Error(), "XO_ZONEINFO") || !strings.Contains(err.Error(), "--tzdata=system") {
		t.Errorf("no go command: %v", err)
	}
	t.Setenv("XO_ZONEINFO", filepath.Join(t.TempDir(), "missing.zip"))
	if _, err := loadZoneinfo("go"); err == nil || !strings.Contains(err.Error(), "--tzdata=system") {
		t.Errorf("missing XO_ZONEINFO file: %v", err)
	}
}
