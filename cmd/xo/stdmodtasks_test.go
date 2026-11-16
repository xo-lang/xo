package main

import (
	"bytes"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/xo-lang/xo/internal/xotool"
)

// TestXoTestStdModuleOsTasks: a std module's test that uses os.tasks
// runs under `xo test --std-module` as under `xo test DIR`. The test
// build gave the root scope (os.tasks' group) only to programs whose
// non-std modules spell os.tasks, so a std module's own tests faulted
// with "os.tasks is not available here" (2026-11-16, std/ui).
func TestXoTestStdModuleOsTasks(t *testing.T) {
	t.Parallel()
	root, err := xotool.Root()
	if err != nil {
		t.Fatal(err)
	}
	src := filepath.Join(root, "std")
	std := filepath.Join(t.TempDir(), "std")
	err = filepath.WalkDir(src, func(p string, e fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, p)
		if e.IsDir() {
			if e.Name() == "testdata" || rel == "rt" || e.Name() == ".xo" {
				return filepath.SkipDir
			}
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
	files := map[string]string{
		"zztasks.xo": "// A probe module for TestXoTestStdModuleOsTasks.\n\npub fn seven() -> Int { 7 }\n",
		"zztasks_test.xo": "test \"os.tasks in a std module's test\" {\n" +
			"  let ch = Chan[Int].new(cap: 1)\n" +
			"  os.tasks.spawn_background(fn() { let _f = ch.try_send(seven()) })\n" +
			"  expect(ch.recv()) == Some(7)\n" +
			"}\n",
	}
	for name, text := range files {
		if err := os.WriteFile(filepath.Join(std, name), []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	setenv(t, "XO_STD", std)
	setenv(t, "XO_BACKEND_CACHE_DIR", t.TempDir())
	setenv(t, "XO_STD_EXPORT", "")
	var out, errOut bytes.Buffer
	code := run(t, []string{"test", "--std-module=zztasks"}, &out, &errOut)
	got := out.String() + errOut.String()
	if code != 0 || !strings.Contains(got, "1 passed") {
		t.Fatalf("xo test --std-module=zztasks: want its os.tasks test to pass, exit %d\n%s", code, got)
	}
}
