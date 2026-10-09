package mod_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/xo-lang/xo/internal/mod"
	"github.com/xo-lang/xo/internal/mod/modtest"
)

// TestLocate finds the repository of a module path (decision 0086,
// modules in subdirectories): the first three elements on known hosts,
// else the longest prefix git answers for.
func TestLocate(t *testing.T) {
	env := modtest.Setup(t, t.TempDir(), mod.Compare)
	for path, want := range map[string]mod.Location{
		"github.com/acme/tools":        {Repo: "github.com/acme/tools"},
		"github.com/acme/tools/cli":    {Repo: "github.com/acme/tools", Subdir: "cli"},
		"github.com/acme/tools/cli/v2": {Repo: "github.com/acme/tools", Subdir: "cli"},
		"gitlab.com/a/b/c/d":           {Repo: "gitlab.com/a/b", Subdir: "c/d"},
	} {
		if got := mod.Locate(path); got != want {
			t.Errorf("Locate(%s) = %+v, want %+v", path, got, want)
		}
	}
	// another host: git is asked, longest prefix first
	repo := filepath.Join(env.Repos, "example.com", "mono")
	os.MkdirAll(repo, 0o755)
	modtest.Git(t, repo, "init", "-q", "-b", "main")
	modtest.WriteFiles(t, repo, map[string]string{
		"xo.mod":         "module example.com/mono\nedition 2026\n",
		"svc/xo.mod":     "module example.com/mono/svc\nedition 2026\n",
		"svc/api/api.xo": "pub fn port() -> Int { 8080 }\n",
		"svc/sub/xo.mod": "module example.com/mono/svc/sub\nedition 2026\n",
		"svc/sub/s.xo":   "pub fn s() -> Int { 1 }\n",
	})
	modtest.Git(t, repo, "add", "-A")
	modtest.Git(t, repo, "commit", "-q", "-m", "svc")
	modtest.Git(t, repo, "tag", "svc/v0.3.0")
	modtest.Git(t, repo, "tag", "v0.1.0")
	if got := mod.Locate("example.com/mono/svc"); got != (mod.Location{Repo: "example.com/mono", Subdir: "svc"}) {
		t.Errorf("Locate(example.com/mono/svc) = %+v", got)
	}
	if got := mod.Locate("example.com/mono"); got != (mod.Location{Repo: "example.com/mono"}) {
		t.Errorf("Locate(example.com/mono) = %+v", got)
	}
	vs, err := mod.Versions("example.com/mono/svc")
	if err != nil || strings.Join(vs, " ") != "v0.3.0" {
		t.Fatalf("Versions = %v, %v", vs, err)
	}
	if vs, _ := mod.Versions("example.com/mono"); strings.Join(vs, " ") != "v0.1.0" {
		t.Errorf("root Versions = %v", vs)
	}
	dir, prob := mod.Download("example.com/mono/svc", "v0.3.0")
	if prob != nil {
		t.Fatal(prob)
	}
	// the tree is svc/ at the tag, without the nested module svc/sub
	if _, err := os.Stat(filepath.Join(dir, "api", "api.xo")); err != nil {
		t.Error(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "sub")); err == nil {
		t.Error("the nested module svc/sub is in the tree")
	}
	// HashDir leaves nested modules out, so a vendored copy that holds
	// one hashes as the fetched tree
	h1, _ := mod.HashDir(dir)
	cp := filepath.Join(t.TempDir(), "svc")
	modtest.CopyTree(dir, cp)
	modtest.WriteFiles(t, cp, map[string]string{"sub/xo.mod": "module example.com/mono/svc/sub\n", "sub/s.xo": "x\n"})
	if h2, _ := mod.HashDir(cp); h1 != h2 {
		t.Errorf("HashDir counts a nested module: %s vs %s", h1, h2)
	}
	if _, prob := mod.Download("example.com/mono/svc", "v0.9.0"); prob == nil || !strings.Contains(prob.Msg, "no tag svc/v0.9.0") {
		t.Errorf("missing tag: %v", prob)
	}
}
