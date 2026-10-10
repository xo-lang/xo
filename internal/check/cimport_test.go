package check

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/xo-lang/xo/internal/cbind"
)

const cimportHeader = `typedef struct box box;
struct pt { int x; int y; };
int add(int a, int b);
box *box_new(long v);
long box_get(const box *b);
struct pt pt_swap(struct pt p);
void out(int *p);
`

// TestUseC checks `use c` (decision 0116) on the native check: effects,
// `uses` ceilings, skipped names, bind failures, handles that are not
// comparable, and XO1201 without a native backend.
func TestUseC(t *testing.T) {
	if _, err := cbind.Clang(); err != nil {
		t.Skip(err)
	}
	t.Setenv("XO_CBIND_DIR", "")
	cases := []struct {
		name   string
		src    string
		native bool
		want   []string
	}{
		{name: "calls", native: true, src: "use c \"lib.h\"\n\nfn main(os: Os) {\n  let p = lib.pt_swap(lib.Pt{x: 1, y: 2})\n  os.stdio.println(\"${lib.add(1, 2)} ${p.x}\")\n  if let Some(b) = lib.box_new(3) {\n    os.stdio.println(\"${lib.box_get(b)}\")\n  }\n}\n"},
		{name: "Go backend", native: false, src: "use c \"lib.h\"\n\nfn main(os: Os) {\n  os.stdio.println(\"${lib.add(1, 2)}\")\n}\n", want: []string{"XO1201"}},
		{name: "pub needs ffi", native: true, src: "use c \"lib.h\"\n\npub fn sum(a: Int32) -> Int32 {\n  lib.add(a, a)\n}\n", want: []string{"XO0701"}},
		{name: "pub declares ffi", native: true, src: "use c \"lib.h\"\n\npub fn sum(a: Int32) -> Int32 uses ffi {\n  lib.add(a, a)\n}\n"},
		{name: "uses none", native: true, src: "use c \"lib.h\" uses none\n\npub fn sum(a: Int32) -> Int32 {\n  lib.add(a, a)\n}\n"},
		{name: "uses a narrower effect", native: true, src: "use c \"lib.h\" uses clock\n\npub fn sum(a: Int32) -> Int32 uses clock {\n  lib.add(a, a)\n}\n"},
		{name: "unknown effect", native: true, src: "use c \"lib.h\" uses telepathy\n\nfn main(os: Os) {\n  os.stdio.println(\"x\")\n}\n", want: []string{"XO1104"}},
		{name: "missing header", native: true, src: "use c \"nope_xo.h\"\n\nfn main(os: Os) {\n  os.stdio.println(\"x\")\n}\n", want: []string{"XO1104"}},
		{name: "missing link file", native: true, src: "use c \"lib.h\" link \"nope.c\"\n\nfn main(os: Os) {\n  os.stdio.println(\"x\")\n}\n", want: []string{"XO1104"}},
		{name: "skipped function", native: true, src: "use c \"lib.h\"\n\nfn main(os: Os) {\n  lib.out(1)\n  os.stdio.println(\"x\")\n}\n", want: []string{"XO1105"}},
		{name: "handles are not comparable", native: true, src: "use c \"lib.h\"\n\nfn same(a: lib.Box, b: lib.Box) -> Bool {\n  a == b\n}\n", want: []string{"XO0404"}},
		{name: "a handle cannot be built", native: true, src: "use c \"lib.h\"\n\nfn f() -> lib.Box {\n  lib.Box{}\n}\n", want: []string{"XO0605"}},
		{name: "alias", native: true, src: "use c \"lib.h\" as clib\n\nfn main(os: Os) {\n  os.stdio.println(\"${clib.add(1, 2)}\")\n}\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			m := filepath.Join(dir, "m")
			os.MkdirAll(m, 0o755)
			os.WriteFile(filepath.Join(m, "lib.h"), []byte(cimportHeader), 0o644)
			os.WriteFile(filepath.Join(m, "main.xo"), []byte(tc.src), 0o644)
			res, err := CheckOpts([]string{m}, Options{Native: tc.native})
			if err != nil {
				t.Fatal(err)
			}
			got := codes(res.Diagnostics)
			want := append([]string(nil), tc.want...)
			sort.Strings(want)
			if strings.Join(got, ",") != strings.Join(want, ",") {
				t.Errorf("codes = %v, want %v\n%s", got, want, humanAll(res.Diagnostics))
			}
		})
	}
}
