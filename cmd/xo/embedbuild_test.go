package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestEmbedBuild checks decision 0663: `embed.build` puts a package of
// the module, built for wasip1/wasm, in a Bytes constant, and an edit to
// that package alone builds the program again (the run cache keys on the
// nested build's inputs; before, a warm build kept the old client).
func TestEmbedBuild(t *testing.T) {
	t.Parallel()
	dir := writeFiles(t, map[string]string{
		"app/xo.mod":         "module app\n",
		"app/client/main.xo": "fn main(os: Os) {\n  os.stdio.println(\"v1\")\n}\n",
		"app/server/main.xo": "const CLIENT: Bytes = embed.build(\"app/client\", \"wasip1/wasm\")\n\nfn main(os: Os) {\n  os.stdio.println(\"${CLIENT.len()} ${CLIENT[..4].to_hex()}\")\n}\n",
	})
	app := filepath.Join(dir, "app")
	runServer := func() string {
		t.Helper()
		var out, errb bytes.Buffer
		if code := run(t, []string{"run", filepath.Join(app, "server")}, &out, &errb); code != 0 {
			t.Fatalf("run: exit %d\n%s%s", code, out.String(), errb.String())
		}
		return strings.TrimSpace(out.String())
	}
	first := runServer()
	if !strings.HasSuffix(first, " 0061736d") {
		t.Fatalf("the constant is not a wasm module: %q", first)
	}
	if again := runServer(); again != first {
		t.Fatalf("a second run printed %q, want %q", again, first)
	}
	if err := os.WriteFile(filepath.Join(app, "client/main.xo"), []byte("fn main(os: Os) {\n  os.stdio.println(\"version two of the client, longer\")\n}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if edited := runServer(); edited == first {
		t.Errorf("after an edit to the client the server still embeds the old module (%q)", edited)
	}
}

// TestEmbedBuildErrors checks XO1519: a target other than wasip1/wasm,
// a package outside the module, a constant not typed Bytes, and an
// embed.build inside a package that embed.build builds.
func TestEmbedBuildErrors(t *testing.T) {
	t.Parallel()
	dir := writeFiles(t, map[string]string{
		"app/xo.mod":         "module app\n",
		"app/inner/main.xo":  "const X: Bytes = embed.build(\"app/leaf\", \"wasip1/wasm\")\n\nfn main(os: Os) {\n  os.stdio.println(\"${X.len()}\")\n}\n",
		"app/leaf/main.xo":   "fn main(os: Os) {\n}\n",
		"app/server/main.xo": "const A: Bytes = embed.build(\"app/leaf\", \"linux/arm64\")\nconst B: Bytes = embed.build(\"other/leaf\", \"wasip1/wasm\")\nconst C: Str = embed.build(\"app/leaf\", \"wasip1/wasm\")\nconst D: Bytes = embed.build(\"app/inner\", \"wasip1/wasm\")\n\nfn main(os: Os) {\n  os.stdio.println(\"${A.len()} ${B.len()} ${C} ${D.len()}\")\n}\n",
	})
	var out, errb bytes.Buffer
	code := run(t, []string{"build", "-o", filepath.Join(dir, "server.bin"), filepath.Join(dir, "app", "server")}, &out, &errb)
	got := out.String() + errb.String()
	if code == 0 {
		t.Fatalf("build: exit 0, want XO1519\n%s", got)
	}
	for _, want := range []string{"builds for wasip1/wasm, not `linux/arm64`", "must be in this module", "is `Bytes`"} {
		if !strings.Contains(got, want) {
			t.Errorf("no XO1519 %q in\n%s", want, got)
		}
	}
	if strings.Count(got, "XO1519") < 3 {
		t.Errorf("want 3 XO1519 errors:\n%s", got)
	}
	// the three errors stop the build before D's nested build; alone, D's
	// build of app/inner reports the nested embed.build
	if err := os.WriteFile(filepath.Join(dir, "app/server/main.xo"), []byte("const D: Bytes = embed.build(\"app/inner\", \"wasip1/wasm\")\n\nfn main(os: Os) {\n  os.stdio.println(\"${D.len()}\")\n}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	errb.Reset()
	code = run(t, []string{"build", "-o", filepath.Join(dir, "server.bin"), filepath.Join(dir, "app", "server")}, &out, &errb)
	got = out.String() + errb.String()
	if code == 0 || !strings.Contains(got, "XO1519") || !strings.Contains(got, "one level only") {
		t.Errorf("nested: exit %d, want XO1519 one level only\n%s", code, got)
	}
}
