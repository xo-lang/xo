package mod

import (
	"crypto/sha256"
	"encoding/hex"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// Modules in subdirectories of a repository (decision 0086, as Go does):
// the module github.com/acme/tools/cli may live in the directory cli/ of
// the repository github.com/acme/tools. Its versions are the tags of that
// repository prefixed with the directory (`cli/v1.2.0`), and a version's
// tree is the directory's tree at the tag. A /vN major suffix stays out
// of both: github.com/acme/tools/cli/v2 is the directory cli/ with tags
// `cli/v2.x.y`.

// Location is where a module's versions live in git.
type Location struct {
	Repo   string // the module path of the repository root (no /vN suffix)
	Subdir string // the module's directory in the repository, "" at the root
}

// TagPrefix is what precedes vX.Y.Z in the module's tags: "cli/" for a
// module in the directory cli/, "" at the repository root.
func (l Location) TagPrefix() string {
	if l.Subdir == "" {
		return ""
	}
	return l.Subdir + "/"
}

// Tag is the git tag of version v of the module.
func (l Location) Tag(v string) string { return l.TagPrefix() + v }

// knownHosts have repositories at exactly host/owner/name, so the rest of
// a module path is a directory in the repository without asking git.
var knownHosts = map[string]bool{"github.com": true, "gitlab.com": true, "bitbucket.org": true}

// locations caches Locate for paths that needed git, by the git location
// of the whole path (tests change XO_GIT_BASE between worlds).
var locations sync.Map

// Locate finds the repository of module path and the module's directory
// in it. On github.com, gitlab.com, and bitbucket.org the repository is
// the first three path elements. Elsewhere the longest prefix of the path
// (without /vN) that is a git repository wins: a repository already in
// the module cache for the whole path, else one `git ls-remote` per
// prefix, longest first.
// When none answers (or offline) the module is taken to be at the root of
// the repository named by its whole path, so the fetch error names that
// location.
func Locate(path string) Location {
	p := repoPath(path)
	elems := strings.Split(p, "/")
	if knownHosts[elems[0]] {
		if len(elems) > 3 {
			return Location{Repo: strings.Join(elems[:3], "/"), Subdir: strings.Join(elems[3:], "/")}
		}
		return Location{Repo: p}
	}
	key := RepoURL(p)
	if v, ok := locations.Load(key); ok {
		return v.(Location)
	}
	// a cached clone of the whole path answers without git (a cached
	// clone of a shorter prefix does not: the path may be a repository
	// of its own that was never fetched)
	if _, err := os.Stat(filepath.Join(vcsDir(p), "HEAD")); err == nil {
		return Location{Repo: p}
	}
	if Offline() {
		return Location{Repo: p}
	}
	for n := len(elems); n >= 2; n-- {
		cand := strings.Join(elems[:n], "/")
		if _, err := git("", "ls-remote", "--quiet", RepoURL(cand), "HEAD"); err == nil {
			l := Location{Repo: cand, Subdir: strings.Join(elems[n:], "/")}
			locations.Store(key, l)
			return l
		}
	}
	return Location{Repo: p}
}

// vcsDir is the bare clone of the repository at module path repo in the
// module cache.
func vcsDir(repo string) string {
	sum := sha256.Sum256([]byte(RepoURL(repo)))
	return filepath.Join(CacheDir(), "cache", "vcs", hex.EncodeToString(sum[:16]))
}

// pruneNested removes every directory below dir that holds an xo.mod: a
// nested module is not part of the module around it (as in Go).
func pruneNested(dir string) error {
	var nested []string
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() && p != dir {
			if _, err := os.Stat(filepath.Join(p, "xo.mod")); err == nil {
				nested = append(nested, p)
				return filepath.SkipDir
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	for _, n := range nested {
		if err := os.RemoveAll(n); err != nil {
			return err
		}
	}
	return nil
}
