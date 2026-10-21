package main

import (
	"bytes"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestNativeStdCheckErrorsAsThemselves: a check error in a native std
// body, checked when the build reaches it (decision 0247), is reported
// with its own code at its own place, not as XO1201 "not available
// without a garbage collector" (2026-10-20 poll, Xo Lang 3: an XO0605 in
// std/native read as XO1201).
func TestNativeStdCheckErrorsAsThemselves(t *testing.T) {
	t.Parallel()
	std := filepath.Join(t.TempDir(), "std")
	src := filepath.Join(repoRoot, "std")
	err := filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, p)
		if d.IsDir() {
			return os.MkdirAll(filepath.Join(std, rel), 0o755)
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		return os.WriteFile(filepath.Join(std, rel), b, 0o644)
	})
	if err != nil {
		t.Fatal(err)
	}
	impl := filepath.Join(std, "native", "csv.xo")
	b, err := os.ReadFile(impl)
	if err != nil {
		t.Fatal(err)
	}
	head := "fn parse(text: Str, sep: Rune = ',', at: rt.Loc) -> List[List[Str]] ! csv.CsvErr {\n"
	if !bytes.Contains(b, []byte(head)) {
		t.Fatalf("%s has no %q", impl, head)
	}
	b = bytes.Replace(b, []byte(head), []byte(head+"  let wrong: Int = \"not an Int\"\n"), 1)
	if err := os.WriteFile(impl, b, 0o644); err != nil {
		t.Fatal(err)
	}
	dir := writeFiles(t, map[string]string{"main.xo": "use std/csv\n\nfn main(os: Os) {\n  match csv.parse(\"a,b\") {\n    Ok(rows) => os.stdio.println(\"${rows.len()}\")\n    Err(e) => os.stdio.println(e.display())\n  }\n}\n"})
	var out, errOut bytes.Buffer
	code := run(t, []string{"build", "--std=" + std, "-o", filepath.Join(t.TempDir(), "prog"), dir}, &out, &errOut)
	all := out.String() + errOut.String()
	if code == 0 {
		t.Fatalf("the build passed with a type error in std/native/csv.xo:\n%s", all)
	}
	if strings.Contains(all, "XO1201") || !strings.Contains(all, "native/csv.xo") || !strings.Contains(all, "error XO0") {
		t.Errorf("want the error at std/native/csv.xo with its own code, not XO1201:\n%s", all)
	}
}
