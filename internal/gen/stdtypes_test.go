package gen

import (
	"bytes"
	"flag"
	"os"
	"path/filepath"
	"testing"
)

var update = flag.Bool("update", false, "rewrite generated files and golden outputs")

// TestStdTypes checks that std/rt/std_gen.go matches the std declaration
// files.
func TestStdTypes(t *testing.T) {
	got, err := StdTypes()
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join("..", "..", "std", "rt", "std_gen.go")
	if *update {
		if err := os.WriteFile(path, got, 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("std/rt/std_gen.go is out of date; run: go test ./internal/gen -run TestStdTypes -update")
	}
}
