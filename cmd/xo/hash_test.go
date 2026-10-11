package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/xo-lang/xo/internal/mod/modtest"
)

const hashLib = `pub fn total(xs: List[Int]) -> Int { add_all(xs) }

fn add_all(xs: List[Int]) -> Int {
  var s = 0
  for x in xs {
    s += x
  }
  s
}

pub fn label(n: Int) -> Str {
  let text = "n=${n}"
  text
}
`

func queryHashOf(t *testing.T, dir, name string) string {
	t.Helper()
	r := xo(t, "query", "hash", "--json", "--dir="+dir, name).want(0)
	var res struct {
		List []struct {
			Hash string `json:"hash"`
			Own  string `json:"own"`
		} `json:"list"`
	}
	if err := json.Unmarshal([]byte(r.out), &res); err != nil || len(res.List) != 1 || !strings.HasPrefix(res.List[0].Hash, "d1:") {
		t.Fatalf("query hash %s: %v\n%s", name, err, r.out)
	}
	return res.List[0].Hash
}

// TestQueryHashAndPublish: `xo query hash` (core 4.3, decision 0125)
// keeps a definition's hash when locals are renamed and changes it with
// its meaning; `xo publish --check` lists the changed definitions,
// private ones and those changed through a callee, where the API diff
// sees nothing.
func TestQueryHashAndPublish(t *testing.T) {
	t.Setenv("XO_NO_SERVE", "1")
	t.Setenv("XO_WORK", "")
	repo := t.TempDir()
	modtest.Git(t, repo, "init", "-q", "-b", "main")
	modtest.WriteFiles(t, repo, map[string]string{"xo.mod": "module example.com/lib\nedition preview\n", "lib.xo": hashLib})
	modtest.Git(t, repo, "add", "-A")
	modtest.Git(t, repo, "commit", "-q", "-m", "v1.0.0")
	modtest.Git(t, repo, "tag", "v1.0.0")
	lib := filepath.Join(repo, "lib.xo")

	total, label := queryHashOf(t, repo, "total"), queryHashOf(t, repo, "label")
	// renamed locals: same hashes
	os.WriteFile(lib, []byte(strings.NewReplacer("var s = 0", "var acc = 0", "s += x", "acc += x", "  s\n}", "  acc\n}", "text", "out").Replace(hashLib)), 0o644)
	if queryHashOf(t, repo, "total") != total || queryHashOf(t, repo, "example.com/lib.label") != label {
		t.Errorf("renaming locals changed a hash")
	}
	p := publish(t, 0, "--check", "--dir", repo)
	if p.Bump != "patch" || len(p.Definitions) != 0 {
		t.Errorf("renamed locals: %+v", p)
	}

	// a private helper computes something else: the API is the same,
	// the definitions say what changed
	os.WriteFile(lib, []byte(strings.Replace(hashLib, "var s = 0", "var s = 1", 1)), 0o644)
	if queryHashOf(t, repo, "total") == total {
		t.Errorf("a changed callee kept the hash of total")
	}
	p = publish(t, 0, "--check", "--dir", repo)
	var got []string
	for _, d := range p.Definitions {
		got = append(got, d.Kind+" "+d.Name)
	}
	if p.Bump != "patch" || strings.Join(got, ", ") != "changed add_all, changed_via total" {
		t.Errorf("changed helper: bump %s, definitions %v", p.Bump, got)
	}
	r := xo(t, "publish", "--check", "--dir", repo).want(0)
	if !strings.Contains(r.out, "  fn example.com/lib.total changed through example.com/lib.add_all") {
		t.Errorf("text output:\n%s", r.out)
	}

	// an unknown name suggests near ones
	r = xo(t, "query", "hash", "--dir="+repo, "totl").want(1)
	if !strings.Contains(r.err, "did you mean total") {
		t.Errorf("unknown name: %s", r.err)
	}
}
