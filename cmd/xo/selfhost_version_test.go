package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestSelfhostVersion checks that the self hosted CLI (selfhost/cli/core,
// decision 0178 stage 2) prints the same toolchain version as this one:
// `pub const VERSION` there equals version here.
func TestSelfhostVersion(t *testing.T) {
	src, err := os.ReadFile(filepath.Join("..", "..", "selfhost", "cli", "core", "cli.xo"))
	if err != nil {
		t.Fatal(err)
	}
	want := `pub const VERSION = "` + version + `"`
	if !strings.Contains(string(src), want) {
		t.Errorf("selfhost/cli/core/cli.xo has no %s: raise both versions together", want)
	}
}
