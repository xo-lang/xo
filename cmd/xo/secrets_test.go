package main

import (
	"path/filepath"
	"strings"
	"testing"
)

// TestFakeSecrets checks decision 0584 stage C on testdata/secrets: a
// test's os.secrets has an in-memory keyring on every OS (add, update,
// search, read, unlink), each test starts with it empty, only([]) drops
// it, and testing.run_main's Os shares the test's keyring; on the Go
// backend and every native one; stage D: the keychain fake (get, set,
// update, delete, only).
func TestFakeSecrets(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("builds and runs test binaries")
	}
	dir := filepath.Join("testdata", "secrets")
	check := func(name string, code int, out string) {
		t.Helper()
		if code != 0 || !strings.Contains(out, "ok: 6 passed, 0 failed, 0 skipped") {
			t.Errorf("%s: exit %d\n%s", name, code, out)
		}
	}
	code, out := xoTest(t, "--backend=go", dir)
	check("go", code, out)
	for _, be := range nativeBackends(t) {
		code, out := xoTest(t, "--backend="+be, dir)
		check(be, code, out)
	}
}
