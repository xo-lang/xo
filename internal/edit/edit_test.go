package edit

import (
	"bytes"
	"encoding/json"
	"flag"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

var update = flag.Bool("update", false, "rewrite the testdata/*.golden files")

// copyTree copies testdata/base into a temporary directory.
func copyTree(t *testing.T, from string) string {
	t.Helper()
	dir := t.TempDir()
	err := filepath.WalkDir(from, func(p string, e fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(from, p)
		if e.IsDir() {
			return os.MkdirAll(filepath.Join(dir, rel), 0o755)
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		return os.WriteFile(filepath.Join(dir, rel), b, 0o644)
	})
	if err != nil {
		t.Fatal(err)
	}
	return dir
}

func snapshot(t *testing.T, dir string) map[string]string {
	t.Helper()
	out := map[string]string{}
	_ = filepath.WalkDir(dir, func(p string, e fs.DirEntry, err error) error {
		if err == nil && !e.IsDir() {
			b, _ := os.ReadFile(p)
			rel, _ := filepath.Rel(dir, p)
			out[filepath.ToSlash(rel)] = string(b)
		}
		return nil
	})
	return out
}

// TestGolden runs each edit on a copy of testdata/base and compares the
// JSON summary and the changed files with testdata/<name>.golden.
func TestGolden(t *testing.T) {
	cases := []struct {
		name string
		run  func(Options) (*Result, error)
		ok   bool
	}{
		{"rename_fn", func(o Options) (*Result, error) { return Rename(o, "users.fetch_user", "users.load_user") }, true},
		{"rename_method", func(o Options) (*Result, error) { return Rename(o, "users.User.display_name", "label") }, true},
		{"rename_const", func(o Options) (*Result, error) { return Rename(o, "users.MAX_ID", "LIMIT") }, true},
		{"rename_clash", func(o Options) (*Result, error) { return Rename(o, "users.fetch_user", "lookup") }, false},
		{"add_field", func(o Options) (*Result, error) { return AddField(o, "users.User", "created: Int = 0") }, true},
		{"add_field_bad_default", func(o Options) (*Result, error) { return AddField(o, "users.User", `created: Int = "now"`) }, false},
		{"add_field_no_default", func(o Options) (*Result, error) { return AddField(o, "users.User", "created: Int") }, false},
		{"change_sig_default", func(o Options) (*Result, error) {
			return ChangeSig(o, "users.lookup", "(id: Int, name: Str, active: Bool = true) -> User")
		}, true},
		{"change_sig_reorder", func(o Options) (*Result, error) {
			return ChangeSig(o, "users.lookup", "(name: Str, id: Int) -> User")
		}, true},
		{"change_sig_rename_param", func(o Options) (*Result, error) {
			return ChangeSig(o, "users.fetch_user", "(uid: Int) -> User")
		}, true},
		{"change_sig_required", func(o Options) (*Result, error) {
			return ChangeSig(o, "users.lookup", "(id: Int, name: Str, active: Bool) -> User")
		}, false},
		{"change_sig_removed", func(o Options) (*Result, error) {
			return ChangeSig(o, "users.lookup", "(id: Int) -> User")
		}, false},
		{"change_sig_type", func(o Options) (*Result, error) {
			return ChangeSig(o, "users.fetch_user", "(id: Str) -> User")
		}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			dir := copyTree(t, filepath.Join("testdata", "base"))
			before := snapshot(t, dir)
			res, err := c.run(Options{Dirs: []string{dir}})
			if err != nil {
				t.Fatal(err)
			}
			if res.OK != c.ok {
				t.Fatalf("ok = %v, want %v: %+v", res.OK, c.ok, res)
			}
			after := snapshot(t, dir)
			if !res.OK && !equalMaps(before, after) {
				t.Fatal("a rejected edit changed files")
			}
			if res.OK != res.Written {
				t.Errorf("written = %v", res.Written)
			}
			js, _ := json.MarshalIndent(res, "", "  ")
			var b bytes.Buffer
			b.Write(bytes.ReplaceAll(js, []byte(dir+string(filepath.Separator)), nil))
			b.WriteString("\n")
			var names []string
			for n := range after {
				if after[n] != before[n] {
					names = append(names, n)
				}
			}
			sort.Strings(names)
			for _, n := range names {
				b.WriteString("-- " + n + " --\n" + after[n])
			}
			golden := filepath.Join("testdata", c.name+".golden")
			if *update {
				if err := os.WriteFile(golden, b.Bytes(), 0o644); err != nil {
					t.Fatal(err)
				}
				return
			}
			want, err := os.ReadFile(golden)
			if err != nil {
				t.Fatalf("%v (run with -update)", err)
			}
			if string(want) != b.String() {
				t.Errorf("mismatch with %s:\n%s", golden, b.String())
			}
		})
	}
}

func equalMaps(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if b[k] != v {
			return false
		}
	}
	return true
}

func TestDryRun(t *testing.T) {
	dir := copyTree(t, filepath.Join("testdata", "base"))
	before := snapshot(t, dir)
	res, err := Rename(Options{Dirs: []string{dir}, DryRun: true}, "users.fetch_user", "load_user")
	if err != nil || !res.OK || res.Written || len(res.Files) != 2 {
		t.Fatalf("res = %+v, err = %v", res, err)
	}
	if !equalMaps(before, snapshot(t, dir)) {
		t.Fatal("dry run changed files")
	}
}

func TestUsageErrors(t *testing.T) {
	dir := copyTree(t, filepath.Join("testdata", "base"))
	o := Options{Dirs: []string{dir}}
	for _, c := range []struct {
		name string
		run  func() (*Result, error)
		want string
	}{
		{"unknown", func() (*Result, error) { return Rename(o, "users.nosuch", "x") }, "does not name"},
		{"bad name", func() (*Result, error) { return Rename(o, "users.fetch_user", "1x") }, "not an identifier"},
		{"type", func() (*Result, error) { return Rename(o, "users.User", "Person") }, "not types"},
		{"std", func() (*Result, error) { return Rename(o, "Str.len", "size") }, ""},
		{"not struct", func() (*Result, error) { return AddField(o, "users.fetch_user", "x: Int = 1") }, "not a struct"},
		{"dup field", func() (*Result, error) { return AddField(o, "users.User", "id: Int = 1") }, "already has"},
		{"bad field", func() (*Result, error) { return AddField(o, "users.User", "x Int") }, "does not parse"},
		{"bad sig", func() (*Result, error) { return ChangeSig(o, "users.lookup", "-> Int") }, "must start"},
		{"clauses", func() (*Result, error) { return ChangeSig(o, "users.lookup", "(id: Int) -> User uses net") }, "clauses"},
	} {
		_, err := c.run()
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: err = %v, want %q", c.name, err, c.want)
		}
	}
}

func TestRejectsBrokenProgram(t *testing.T) {
	dir := copyTree(t, filepath.Join("testdata", "base"))
	p := filepath.Join(dir, "api", "api.xo")
	b, _ := os.ReadFile(p)
	if err := os.WriteFile(p, append(b, []byte("\nfn bad() -> Int { \"s\" }\n")...), 0o644); err != nil {
		t.Fatal(err)
	}
	res, err := Rename(Options{Dirs: []string{dir}}, "users.fetch_user", "load_user")
	if err != nil {
		t.Fatal(err)
	}
	if res.OK || len(res.Diagnostics) == 0 || !strings.Contains(res.Reason, "before the edit") {
		t.Fatalf("res = %+v", res)
	}
}
