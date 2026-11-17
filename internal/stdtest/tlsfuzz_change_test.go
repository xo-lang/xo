package stdtest

import (
	"math/rand"
	"testing"
)

// TestTlfChangeNoSuites: every change applies to a hello with no cipher
// suites (seed 2 of TestTlsFuzz drew "odd suites" on one and the harness
// panicked with slice bounds out of range [:-1] before any case ran).
func TestTlfChangeNoSuites(t *testing.T) {
	h := &tlfCH{vers: []byte{3, 3}, random: make([]byte, 32), comp: []byte{0}, hasExts: true}
	rec := tlfEncode(h)
	for seed := int64(0); seed < 200; seed++ {
		r := rand.New(rand.NewSource(seed))
		if _, ok := tlfParse(rec); !ok {
			t.Fatal("the hello does not parse")
		}
		tlfChange(r, rec)
	}
}
