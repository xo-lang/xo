package repotest

import (
	"bytes"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// wordIntrinsic matches a call of the intrinsics Int's word methods replace.
var wordIntrinsic = regexp.MustCompile(`\brt\.(int_wrap|u64_mul_hi)\(`)

// TestStdUsesWordMethods fails when std calls rt.int_wrap or
// rt.u64_mul_hi outside the word methods' own bodies (std/native/prelude.xo)
// and their declarations (std/native/rt.xo): decision 0260 step 3 moved
// std to wrapping_add, shr_logical, mul_hi_unsigned and the rest, which
// lower to the same intrinsics, so new code reads as methods and the
// field generators (scripts/gen/x25519fe64, nistecmul4p, keccaklazy)
// emit them too. Comments are skipped.
func TestStdUsesWordMethods(t *testing.T) {
	allowed := map[string]bool{"std/native/prelude.xo": true, "std/native/rt.xo": true}
	err := filepath.WalkDir(filepath.Join(root, "std"), func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".xo") {
			return err
		}
		rel, _ := filepath.Rel(root, path)
		rel = filepath.ToSlash(rel)
		if allowed[rel] {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for i, line := range bytes.Split(data, []byte("\n")) {
			if c := bytes.Index(line, []byte("//")); c >= 0 {
				line = line[:c]
			}
			if m := wordIntrinsic.Find(line); m != nil {
				t.Errorf("%s:%d: %s...): use Int's word methods (spec/std.md; wrapping_add, shr_logical, mul_hi_unsigned, ...)", rel, i+1, m[:len(m)-1])
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
