package main

import (
	"crypto/rand"
	"encoding/hex"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// TestRealKeychain is decision 0584 section 8's real Keychain check (owner's
// go, 2026-11-10): std/native/llrt/userfs_test.xo's round trip against a
// throwaway keychain file this test creates in a temporary directory with
// `security create-keychain` and deletes after, never the login keychain.
// It checks that the user's keychain search list and default keychain are
// the same before, during and after, and finds the item the round trip left
// in the file (attributes only: `security dump-keychain` without -d, which
// reads no secret and so prompts for nothing). macOS only, and only with
// XO_TEST_REAL_KEYCHAIN=1, so routine suites never touch the Security
// framework's keychains.
func TestRealKeychain(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("macOS only")
	}
	if os.Getenv("XO_TEST_REAL_KEYCHAIN") != "1" {
		t.Skip("set XO_TEST_REAL_KEYCHAIN=1 (decision 0584 section 8)")
	}
	sec := func(args ...string) string {
		t.Helper()
		out, err := exec.Command("/usr/bin/security", args...).CombinedOutput()
		if err != nil {
			t.Fatalf("security %s: %v\n%s", strings.Join(args, " "), err, out)
		}
		return string(out)
	}
	list := func() string { return sec("list-keychains", "-d", "user") }
	def := func() string { return sec("default-keychain", "-d", "user") }
	list0, def0 := list(), def()
	for _, be := range nativeBackends(t) {
		// a keychain of its own per backend: an item's access list trusts
		// the binary that made it, so another test binary updating it would
		// raise an access prompt (2026-11-10: the LLVM run of a keychain the
		// arm64 run had written waited on one until its 1 minute limit)
		roundTrip(t, sec, list0, def0, be)
	}
	if l, d := list(), def(); l != list0 || d != def0 {
		t.Errorf("the search list or default keychain changed:\nbefore %s %s\nafter %s %s", list0, def0, l, d)
	}
}

// roundTrip creates a throwaway keychain, runs userfs_test.xo on backend be
// against it, finds the item it left, and deletes the keychain.
func roundTrip(t *testing.T, sec func(...string) string, list0, def0, be string) {
	t.Helper()
	list := func() string { return sec("list-keychains", "-d", "user") }
	var pwb [16]byte
	if _, err := rand.Read(pwb[:]); err != nil {
		t.Fatal(err)
	}
	pw := hex.EncodeToString(pwb[:])
	path := filepath.Join(t.TempDir(), "xo-0584-"+be+".keychain-db")
	sec("create-keychain", "-p", pw, path)
	deleted := false
	del := func() {
		if !deleted {
			deleted = true
			sec("delete-keychain", path)
		}
	}
	defer del()
	if l := list(); l != list0 {
		t.Fatalf("create-keychain changed the search list:\nbefore %s\nafter %s", list0, l)
	}
	t.Setenv("XO_TEST_KEYCHAIN", path)
	t.Setenv("XO_TEST_KEYCHAIN_PW", pw)
	code, out := xoTest(t, "--backend="+be, "--std-module=native/llrt/userfs")
	if code != 0 || !strings.Contains(out, "1 passed, 0 failed") {
		t.Errorf("%s: exit %d\n%s", be, code, out)
	}
	sec("unlock-keychain", "-p", pw, path)
	dump := sec("dump-keychain", path)
	if !strings.Contains(dump, `"xo-test-0584"`) || !strings.Contains(dump, `"left"`) {
		t.Errorf("%s: the item the round trip left is not in %s:\n%s", be, path, dump)
	}
	del()
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("%s still exists after delete-keychain: %v", path, err)
	}
	if l := list(); l != list0 {
		t.Errorf("delete-keychain changed the search list:\nbefore %s\nafter %s", list0, l)
	}
}
