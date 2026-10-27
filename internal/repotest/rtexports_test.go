package repotest

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// TestRtExportsImportedByPrelude: every C ABI export the native emitters
// call for a runtime call (compiler/ssa exports.xo rt_export, `xo_x_NAME`)
// is defined in a native/llrt/cabi*.xo module that std/native/prelude.xo
// imports, so every program has it to link. A program using os.net without
// std/http linked only while every build loaded std/testing, which reached
// cabi_net through std/http (2026-10-27,
// gotchas/implicit-std-testing-hid-prelude-import.md).
func TestRtExportsImportedByPrelude(t *testing.T) {
	exports, err := os.ReadFile(filepath.Join(root, "compiler", "ssa", "exports.xo"))
	if err != nil {
		t.Fatal(err)
	}
	prelude, err := os.ReadFile(filepath.Join(root, "std", "native", "prelude.xo"))
	if err != nil {
		t.Fatal(err)
	}
	imported := map[string]bool{}
	for _, m := range regexp.MustCompile(`(?m)^use native/llrt/(\w+)`).FindAllStringSubmatch(string(prelude), -1) {
		imported[m[1]] = true
	}
	cabis, err := filepath.Glob(filepath.Join(root, "std", "native", "llrt", "cabi*.xo"))
	if err != nil {
		t.Fatal(err)
	}
	defined := map[string]string{}
	fnRe := regexp.MustCompile(`(?m)^(?:pub )?fn (\w+)\(`)
	for _, p := range cabis {
		if strings.HasSuffix(p, "_test.xo") {
			continue
		}
		src, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		mod := strings.TrimSuffix(filepath.Base(p), ".xo")
		for _, m := range fnRe.FindAllStringSubmatch(string(src), -1) {
			defined[m[1]] = mod
		}
	}
	names := regexp.MustCompile(`"xo_x_(\w+)"`).FindAllStringSubmatch(string(exports), -1)
	if len(names) == 0 {
		t.Fatal("no xo_x_ names in compiler/ssa/exports.xo: the pattern is stale")
	}
	for _, m := range names {
		mod, ok := defined[m[1]]
		if !ok {
			t.Errorf("xo_x_%s (compiler/ssa/exports.xo) is defined in no std/native/llrt/cabi*.xo module", m[1])
			continue
		}
		if !imported[mod] {
			t.Errorf("xo_x_%s is in native/llrt/%s, which std/native/prelude.xo does not import: a program that makes the runtime call does not link", m[1], mod)
		}
	}
}
