package main

import (
	"bytes"
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
