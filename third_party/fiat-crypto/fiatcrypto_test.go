package fiatcrypto

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"strings"
	"testing"
)

// TestVendored requires every vendored file to match SHA256SUMS (written
// by scripts/vendor-fiat-crypto.sh from its pinned list) and the
// embedded files to be the ones on disk.
func TestVendored(t *testing.T) {
	sums, err := os.ReadFile("SHA256SUMS")
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, line := range strings.Split(strings.TrimSpace(string(sums)), "\n") {
		f := strings.Fields(line)
		if len(f) != 2 {
			t.Fatalf("SHA256SUMS: bad line %q", line)
		}
		data, err := os.ReadFile(f[1])
		if err != nil {
			t.Fatal(err)
		}
		h := sha256.Sum256(data)
		if got := hex.EncodeToString(h[:]); got != f[0] {
			t.Errorf("%s: SHA-256 %s, want %s (vendored files are unmodified)", f[1], got, f[0])
		}
		if strings.HasPrefix(f[1], "src/") {
			emb, err := Files.ReadFile(f[1])
			if err != nil || string(emb) != string(data) {
				t.Errorf("%s: embedded copy differs (%v)", f[1], err)
			}
			n++
		}
	}
	if n != 2 {
		t.Fatalf("SHA256SUMS lists %d C files, want 2", n)
	}
}
