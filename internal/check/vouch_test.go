package check

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/xo-lang/xo/internal/mod"
)

// TestVouchVendored checks XO1316 (decision 0156) on a vendored
// dependency whose `use go` of an impure package needs the consumer's
// vouch; a pure package needs none, and a dependency's own vouch line does
// not count. cmd/xo's TestVouches covers fetched modules, stale vouches,
// and `use c`.
func TestVouchVendored(t *testing.T) {
	t.Setenv("XO_WORK", "off")
	cases := []struct {
		name   string
		vouch  string // consumer's vouch line
		dep    string // the dependency's source
		depMod string
		want   []string
	}{
		{name: "impure use go unvouched", dep: "use go \"os\"\n\npub fn pid() -> Int uses go { os.getpid() }\n", want: []string{"XO1316"}},
		{name: "impure use go vouched", vouch: "vouch example.com/osx v1.0.0 go \"os\"", dep: "use go \"os\"\n\npub fn pid() -> Int uses go { os.getpid() }\n"},
		{name: "vouch for another version", vouch: "vouch example.com/osx v0.9.0 go \"os\"", dep: "use go \"os\"\n\npub fn pid() -> Int uses go { os.getpid() }\n", want: []string{"XO1316"}},
		{name: "the dependency vouches for itself", depMod: "\nvouch example.com/osx v1.0.0 go \"os\"\n", dep: "use go \"os\"\n\npub fn pid() -> Int uses go { os.getpid() }\n", want: []string{"XO1316"}},
		{name: "pure use go", dep: "use go \"strings\"\n\npub fn pid() -> Int uses go { strings.count(\"aa\", \"a\") }\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			files := map[string]string{
				"xo.mod":                        "module example.com/app\nedition preview\n\nrequire example.com/osx v1.0.0\n" + tc.vouch + "\n",
				"main.xo":                       "use example.com/osx\n\nfn main(os: Os) {\n  os.stdio.println(\"${osx.pid()}\")\n}\n",
				"vendor/modules.txt":            "# vendor/modules.txt\n# example.com/osx v1.0.0\n",
				"vendor/example.com/osx/xo.mod": "module example.com/osx\nedition preview\n" + tc.depMod,
				"vendor/example.com/osx/osx.xo": tc.dep,
			}
			for name, src := range files {
				p := filepath.Join(dir, name)
				os.MkdirAll(filepath.Dir(p), 0o755)
				os.WriteFile(p, []byte(src), 0o644)
			}
			h, err := mod.HashDir(filepath.Join(dir, "vendor", "example.com", "osx"))
			if err != nil {
				t.Fatal(err)
			}
			os.WriteFile(filepath.Join(dir, "xo.lock"), []byte("example.com/osx v1.0.0 "+h+"\n"), 0o644)
			res, err := Check([]string{dir})
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
