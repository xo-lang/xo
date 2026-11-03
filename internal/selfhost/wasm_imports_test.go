package selfhost

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/xo-lang/xo/internal/testtmp"
	"github.com/xo-lang/xo/internal/xotool"
)

// TestWasmCommandImports pins which WASI interfaces a wasip2/wasm command
// imports (decision 0447 R2h): the component imports only what its core
// module calls, so a program without sockets asks the host for none and a
// UDP only program for no TCP (the host grants the network from the
// imports, wasmNetFlags). The runtime's import blocks list whole
// interfaces; gen.xo drop_unused_imports keeps the called ones (a UDP
// program imported TCP until 2026-11-03). Needs wasm-tools (needWasmTools).
func TestWasmCommandImports(t *testing.T) {
	if testing.Short() {
		t.Skip("builds programs")
	}
	needWasmTools(t, "wasm-tools")
	xo, err := xotool.Path()
	if err != nil {
		t.Fatal(err)
	}
	root := repoRoot(t)
	for _, c := range []struct {
		name, src string
		want, not []string
	}{
		{"hello", "fn main(os: Os) {\n  os.stdio.println(\"hi\")\n}\n",
			[]string{"wasi:cli/stdout@"}, []string{"wasi:sockets/"}},
		{"udp", "use std/udp\n\nfn main(os: Os) {\n  match udp.listen(os.net, \"127.0.0.1:0\") {\n    Ok(c) => os.stdio.println(c.local_addr())\n    Err(e) => os.stdio.println(\"${e}\")\n  }\n}\n",
			[]string{"wasi:sockets/udp@", "wasi:sockets/udp-create-socket@"}, []string{"wasi:sockets/tcp@", "wasi:sockets/tcp-create-socket@"}},
		{"tcp", "fn main(os: Os) {\n  match os.net.listen(\"127.0.0.1:0\") {\n    Ok(l) => os.stdio.println(\"${l.addr()}\")\n    Err(e) => os.stdio.println(\"${e}\")\n  }\n}\n",
			[]string{"wasi:sockets/tcp@", "wasi:sockets/tcp-create-socket@"}, []string{"wasi:sockets/udp@", "wasi:sockets/udp-create-socket@"}},
	} {
		t.Run(c.name, func(t *testing.T) {
			dir := t.TempDir()
			if err := os.WriteFile(filepath.Join(dir, "main.xo"), []byte(c.src), 0o644); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(dir, "xo.mod"), []byte("module m\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			out := filepath.Join(t.TempDir(), "m.wasm")
			cmd := testtmp.Guard(exec.Command(xo, "build", "--targets=wasip2/wasm", "-o", out, "."))
			cmd.Dir = dir
			cmd.Env = append(os.Environ(), "XO_STD="+filepath.Join(root, "std"), "XO_BIN_CACHE=off")
			if b, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("%v\n%s", err, b)
			}
			wit, err := testtmp.Guard(exec.Command("wasm-tools", "component", "wit", out)).CombinedOutput()
			if err != nil {
				t.Fatalf("wasm-tools component wit: %v\n%s", err, wit)
			}
			var imports []string
			for _, l := range strings.Split(string(wit), "\n") {
				if l = strings.TrimSpace(l); strings.HasPrefix(l, "import ") {
					imports = append(imports, strings.TrimSuffix(strings.TrimPrefix(l, "import "), ";"))
				}
			}
			has := func(p string) bool {
				for _, i := range imports {
					if strings.HasPrefix(i, p) {
						return true
					}
				}
				return false
			}
			for _, p := range c.want {
				if !has(p) {
					t.Errorf("imports no %s; imports:\n%s", p, strings.Join(imports, "\n"))
				}
			}
			for _, p := range c.not {
				if has(p) {
					t.Errorf("imports %s; imports:\n%s", p, strings.Join(imports, "\n"))
				}
			}
		})
	}
}
