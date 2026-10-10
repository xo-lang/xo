// Package modtest builds local git repositories of Xo modules for tests
// of internal/mod and the xo command (decision 0086). Nothing touches the
// network: XO_GIT_BASE points module paths at the repositories.
package modtest

import (
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// Env holds the directories of a test module world.
type Env struct {
	Repos string // XO_GIT_BASE: <Repos>/<module path> are git repositories
	Cache string // XO_MODCACHE
}

// Git runs git in dir with a neutral configuration (no user or system
// config, fixed identity and dates).
func Git(t testing.TB, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"GIT_CONFIG_GLOBAL="+os.DevNull, "GIT_CONFIG_NOSYSTEM=1",
		"GIT_AUTHOR_NAME=xo", "GIT_AUTHOR_EMAIL=xo@example.invalid",
		"GIT_COMMITTER_NAME=xo", "GIT_COMMITTER_EMAIL=xo@example.invalid",
		"GIT_AUTHOR_DATE=2026-10-09T00:00:00Z", "GIT_COMMITTER_DATE=2026-10-09T00:00:00Z")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s in %s: %v\n%s", strings.Join(args, " "), dir, err, out)
	}
	return string(out)
}

// Setup creates one git repository per module from src, a directory of
// "<module path>@<version>" trees (testdata/repos): each version is a
// commit tagged with it, in version order. It sets XO_GIT_BASE and
// XO_MODCACHE (a temporary cache made writable again before removal) and
// clears XO_OFFLINE and XO_WORK.
func Setup(t testing.TB, src string, compare func(a, b string) int) *Env {
	t.Helper()
	root := t.TempDir()
	e := &Env{Repos: filepath.Join(root, "repos"), Cache: filepath.Join(root, "cache")}
	// the cache is read only: make it removable before TempDir's cleanup
	t.Cleanup(func() { MakeWritable(e.Cache) })
	t.Setenv("XO_GIT_BASE", e.Repos)
	t.Setenv("XO_GIT_REWRITE", "")
	t.Setenv("XO_MODCACHE", e.Cache)
	t.Setenv("XO_OFFLINE", "")
	t.Setenv("XO_WORK", "")
	t.Setenv("XO_NO_SERVE", "1")
	// fetch from git only, without a checksum log, whatever the
	// developer's environment says (decisions 0124, 0126)
	t.Setenv("XO_PROXY", "direct")
	t.Setenv("XO_NOPROXY", "")
	t.Setenv("XO_SUMDB", "off")
	t.Setenv("XO_NOSUMDB", "")
	versions := map[string][]string{}
	entries, err := findVersionDirs(src)
	if err != nil {
		t.Fatal(err)
	}
	for _, pv := range entries {
		p, v, _ := strings.Cut(pv, "@")
		versions[p] = append(versions[p], v)
	}
	var paths []string
	for p := range versions {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	for _, p := range paths {
		vs := versions[p]
		sort.Slice(vs, func(i, j int) bool { return compare(vs[i], vs[j]) < 0 })
		for _, v := range vs {
			e.Commit(t, p, v, filepath.Join(src, filepath.FromSlash(p)+"@"+v), false)
		}
	}
	return e
}

// findVersionDirs lists "<module path>@<version>" directories under src.
func findVersionDirs(src string) ([]string, error) {
	var out []string
	err := filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() && strings.Contains(d.Name(), "@") {
			rel, err := filepath.Rel(src, p)
			if err != nil {
				return err
			}
			out = append(out, filepath.ToSlash(rel))
			return filepath.SkipDir
		}
		return nil
	})
	return out, err
}

// Commit replaces the work tree of the repository of module path with
// the tree in dir, commits it, and tags it version (moving the tag when
// force is set).
func (e *Env) Commit(t testing.TB, path, version, dir string, force bool) {
	t.Helper()
	repo := filepath.Join(e.Repos, filepath.FromSlash(path))
	if _, err := os.Stat(filepath.Join(repo, ".git")); err != nil {
		if err := os.MkdirAll(repo, 0o755); err != nil {
			t.Fatal(err)
		}
		Git(t, repo, "init", "-q", "-b", "main")
	}
	entries, _ := os.ReadDir(repo)
	for _, en := range entries {
		if en.Name() != ".git" {
			os.RemoveAll(filepath.Join(repo, en.Name()))
		}
	}
	if err := CopyTree(dir, repo); err != nil {
		t.Fatal(err)
	}
	Git(t, repo, "add", "-A")
	Git(t, repo, "commit", "-q", "--allow-empty", "-m", path+" "+version)
	if force {
		Git(t, repo, "tag", "-f", version)
	} else {
		Git(t, repo, "tag", version)
	}
}

// CopyTree copies the regular files under src to dst.
func CopyTree(src, dst string) error {
	return filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, p)
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, 0o644)
	})
}

// WriteFiles writes files (slash paths relative to dir) under dir.
func WriteFiles(t testing.TB, dir string, files map[string]string) {
	t.Helper()
	for name, src := range files {
		p := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

// MakeWritable gives the owner write permission on everything under dir.
func MakeWritable(dir string) {
	filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			os.Chmod(p, 0o755)
		} else {
			os.Chmod(p, 0o644)
		}
		return nil
	})
}
