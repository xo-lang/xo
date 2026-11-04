package repotest

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// TestVtPublicOnlyInVerify keeps the variable time point multiplications of
// std/native/llrt/nistec.xo (pt_mul_vt_public, pt_mul_base_vt_public) out of
// signing, ECDH and key generation: they skip zero digits and index their
// table directly, which is only fine when every input is public, as in
// nistec.verify. Any other caller, in any std file, fails here.
func TestVtPublicOnlyInVerify(t *testing.T) {
	root := filepath.Join("..", "..")
	call := regexp.MustCompile(`\bpt_mul(_base)?_vt_public\(`)
	fnLine := regexp.MustCompile(`^(pub )?fn (\w+)\(`)
	var found []string
	err := filepath.Walk(filepath.Join(root, "std"), func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || !strings.HasSuffix(path, ".xo") {
			return err
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		cur := ""
		for i, line := range strings.Split(string(b), "\n") {
			if m := fnLine.FindStringSubmatch(line); m != nil {
				cur = m[2]
				if call.MatchString(line) {
					continue
				}
			}
			if strings.HasPrefix(strings.TrimSpace(line), "//") || !call.MatchString(line) {
				continue
			}
			rel, _ := filepath.Rel(root, path)
			ok := filepath.ToSlash(rel) == "std/native/llrt/nistec.xo" && (cur == "verify" || cur == "pt_mul_base_vt_public")
			if !ok {
				found = append(found, filepath.ToSlash(rel)+":"+itoa(i+1)+" in "+cur)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range found {
		t.Errorf("%s calls a variable time multiplication: only nistec.verify may (inputs must be public)", f)
	}
}
