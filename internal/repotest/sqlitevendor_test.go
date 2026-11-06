package repotest

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// TestSQLiteVendored holds the owner's vendoring conditions for std/sqlite
// (decision 0534 item 11): sqlite3.c and sqlite3.h are the files whose
// SHA-256 std/native/sqlite/VERSION records (an edit to the copy, or an
// update that forgets VERSION, fails here), and THIRD_PARTY_NOTICES names
// the release VERSION names.
func TestSQLiteVendored(t *testing.T) {
	dir := filepath.Join(root, "std", "native", "sqlite")
	version, err := os.ReadFile(filepath.Join(dir, "VERSION"))
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{"sqlite3.c", "sqlite3.h"} {
		m := regexp.MustCompile(`(?m)^SHA-256 of ` + regexp.QuoteMeta(f) + `: ([0-9a-f]{64})$`).FindSubmatch(version)
		if m == nil {
			t.Errorf("std/native/sqlite/VERSION has no `SHA-256 of %s: HEX` line", f)
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, f))
		if err != nil {
			t.Fatal(err)
		}
		sum := sha256.Sum256(data)
		if got := hex.EncodeToString(sum[:]); got != string(m[1]) {
			t.Errorf("std/native/sqlite/%s: SHA-256 %s, VERSION records %s: the vendored copy changed; update it only by decision 0534's procedure (a new amalgamation, VERSION and THIRD_PARTY_NOTICES in one commit)", f, got, m[1])
		}
	}
	rel := regexp.MustCompile(`^SQLite (\d+\.\d+\.\d+) amalgamation`).FindSubmatch(version)
	if rel == nil {
		t.Fatalf("std/native/sqlite/VERSION's first line does not name `SQLite X.Y.Z amalgamation`")
	}
	notices, err := os.ReadFile(filepath.Join(root, "THIRD_PARTY_NOTICES"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(notices), "amalgamation of SQLite "+string(rel[1])) {
		t.Errorf("THIRD_PARTY_NOTICES does not name SQLite %s (VERSION's release) in its SQLite item", rel[1])
	}
	if !strings.Contains(string(notices), "License: public domain. The SQLite authors") {
		t.Errorf("THIRD_PARTY_NOTICES's SQLite item does not state its license (public domain)")
	}
}
