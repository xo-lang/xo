package main

import (
	"bytes"
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func TestVersion(t *testing.T) {
	t.Parallel()
	var out, errb bytes.Buffer
	if code := run(t, []string{"version"}, &out, &errb); code != 0 {
		t.Fatalf("exit %d: %s", code, errb.String())
	}
	if !strings.HasPrefix(out.String(), "xo "+version) {
		t.Fatalf("got %q", out.String())
	}
	if code := run(t, []string{"version", "x"}, &out, &errb); code != 2 {
		t.Fatalf("extra argument: exit %d, want 2", code)
	}
}

// TestVersionStamp: `xo version --json` and `-v` print the build stamp
// (decision 0264); the xo the tests run is internal/xotool's build, so
// it is stamped with a source key; `xo version` alone is unchanged.
func TestVersionStamp(t *testing.T) {
	t.Parallel()
	var out, errb bytes.Buffer
	if code := run(t, []string{"version", "--json"}, &out, &errb); code != 0 {
		t.Fatalf("--json: exit %d: %s", code, errb.String())
	}
	var v struct {
		Version   string `json:"version"`
		Goos      string `json:"goos"`
		Goarch    string `json:"goarch"`
		Commit    string `json:"commit"`
		Dirty     bool   `json:"dirty"`
		DiffHash  string `json:"diff_hash"`
		SourceKey string `json:"source_key"`
		BuiltBy   string `json:"built_by"`
	}
	if err := json.Unmarshal(out.Bytes(), &v); err != nil {
		t.Fatalf("--json: %v\n%s", err, out.String())
	}
	if v.Version != version || v.Goos == "" || v.Goarch == "" {
		t.Errorf("--json: %+v", v)
	}
	if os.Getenv("XO_CLI") == "" && len(v.SourceKey) != 16 {
		t.Errorf("internal/xotool's build has no source key: %+v", v)
	}
	out.Reset()
	if code := run(t, []string{"version", "-v"}, &out, &errb); code != 0 || !strings.HasPrefix(out.String(), "xo "+version+" ") {
		t.Errorf("-v: exit %d: %q", code, out.String())
	}
}
