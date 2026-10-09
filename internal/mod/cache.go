package mod

import (
	"archive/tar"
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/xo-lang/xo/internal/diag"
)

// CacheDir is the module cache: $XO_MODCACHE, or <user cache dir>/xo/mod.
// Module versions are extracted to <cache>/<escaped path>@<version> and
// made read only; bare git repositories live in <cache>/cache/vcs.
func CacheDir() string {
	if d := os.Getenv("XO_MODCACHE"); d != "" {
		if abs, err := filepath.Abs(d); err == nil {
			return abs
		}
		return d
	}
	if d, err := os.UserCacheDir(); err == nil {
		return filepath.Join(d, "xo", "mod")
	}
	return filepath.Join(os.TempDir(), "xo-mod")
}

// Offline reports whether fetching is disabled: XO_OFFLINE=1 (set by
// `--offline`).
func Offline() bool {
	v := os.Getenv("XO_OFFLINE")
	return v != "" && v != "0" && v != "false"
}

// EscapePath escapes a module path for the file system: every upper case
// letter becomes `!` and its lower case, so module paths that differ only
// in case do not collide on case insensitive file systems (macOS,
// Windows), as in Go's module cache.
func EscapePath(p string) string {
	var b strings.Builder
	for _, r := range p {
		if r >= 'A' && r <= 'Z' {
			b.WriteByte('!')
			b.WriteRune(r + ('a' - 'A'))
		} else {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// VersionDir is the cache directory of a module version (it may not exist).
func VersionDir(path, version string) string {
	return filepath.Join(CacheDir(), filepath.FromSlash(EscapePath(path))+"@"+version)
}

// RepoURL is the git location of a module: $XO_GIT_BASE/<path> when
// XO_GIT_BASE is set (a directory or a URL prefix; tests and mirrors), a
// matching rewrite of XO_GIT_REWRITE ("prefix=url,prefix=url"), or
// https://<path> (https://<path>.git for github.com, gitlab.com, and
// bitbucket.org). A /vN major suffix is not part of the location: every
// major version of a module lives in one repository.
func RepoURL(path string) string {
	path = repoPath(path)
	if rw := os.Getenv("XO_GIT_REWRITE"); rw != "" {
		best, url := "", ""
		for _, part := range strings.Split(rw, ",") {
			from, to, ok := strings.Cut(strings.TrimSpace(part), "=")
			if !ok || from == "" {
				continue
			}
			if (path == from || strings.HasPrefix(path, from+"/")) && len(from) > len(best) {
				best, url = from, to+strings.TrimPrefix(path, from)
			}
		}
		if best != "" {
			return url
		}
	}
	if base := os.Getenv("XO_GIT_BASE"); base != "" {
		return strings.TrimRight(base, "/") + "/" + path
	}
	host, _, _ := strings.Cut(path, "/")
	switch host {
	case "github.com", "gitlab.com", "bitbucket.org":
		return "https://" + path + ".git"
	}
	return "https://" + path
}

// git runs git with no prompts and no hooks; dir "" is the working
// directory. stdout is returned; a failure carries stderr.
func git(dir string, args ...string) ([]byte, error) {
	full := append([]string{"-c", "core.hooksPath=" + os.DevNull, "-c", "core.fsmonitor=false", "-c", "protocol.file.allow=always"}, args...)
	cmd := exec.Command("git", full...)
	if dir != "" {
		cmd.Dir = dir
	}
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0", "GIT_ASKPASS=", "SSH_ASKPASS=", "GIT_CONFIG_NOSYSTEM=1")
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return nil, errors.New(msg)
	}
	return stdout.Bytes(), nil
}

// repoDir is the bare clone of the repository at module path repo (a
// Location's Repo) in the cache, created or refreshed (tags fetched) when
// refresh is set.
func repoDir(repo string, refresh bool) (string, error) {
	url := RepoURL(repo)
	dir := vcsDir(repo)
	if _, err := os.Stat(filepath.Join(dir, "HEAD")); err == nil {
		if refresh {
			if _, err := git(dir, "fetch", "--quiet", "--force", "--prune", url, "+refs/tags/*:refs/tags/*"); err != nil {
				return "", fmt.Errorf("git fetch %s: %v", url, err)
			}
		}
		return dir, nil
	}
	if !refresh {
		return "", fs.ErrNotExist
	}
	if err := os.MkdirAll(filepath.Dir(dir), 0o755); err != nil {
		return "", err
	}
	tmp, err := os.MkdirTemp(filepath.Dir(dir), "clone-")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(tmp)
	if _, err := git("", "clone", "--quiet", "--bare", "--no-local", url, tmp); err != nil {
		return "", fmt.Errorf("git clone %s: %v", url, err)
	}
	if err := os.Rename(tmp, dir); err != nil {
		if _, err2 := os.Stat(filepath.Join(dir, "HEAD")); err2 != nil {
			return "", err
		}
	}
	return dir, nil
}

// Versions lists the versions of a module, lowest first: the vX.Y.Z tags
// of its repository, or the <dir>/vX.Y.Z tags for a module in directory
// <dir> of the repository. Offline, it lists the versions in the module
// cache.
func Versions(path string) ([]string, error) {
	var vs []string
	if Offline() {
		entries, err := os.ReadDir(filepath.Dir(VersionDir(path, "v0.0.0")))
		if err != nil {
			return nil, nil
		}
		prefix := filepath.Base(filepath.FromSlash(EscapePath(path))) + "@"
		for _, e := range entries {
			if v, ok := strings.CutPrefix(e.Name(), prefix); ok && ValidVersion(v) {
				vs = append(vs, v)
			}
		}
	} else {
		loc := Locate(path)
		dir, err := repoDir(loc.Repo, true)
		if err != nil {
			return nil, err
		}
		out, err := git(dir, "tag", "--list", loc.TagPrefix()+"v*")
		if err != nil {
			return nil, err
		}
		for _, t := range strings.Fields(string(out)) {
			t, ok := strings.CutPrefix(t, loc.TagPrefix())
			if ok && ValidVersion(t) && MajorMismatch(path, t) == "" {
				vs = append(vs, t)
			}
		}
	}
	sort.Slice(vs, func(i, j int) bool { return Compare(vs[i], vs[j]) < 0 })
	return vs, nil
}

// Latest is the highest release version of a module (prereleases only
// when there is no release).
func Latest(path string) (string, error) {
	vs, err := Versions(path)
	if err != nil {
		return "", err
	}
	best := ""
	for _, v := range vs {
		if !strings.Contains(v, "-") {
			best = v
		}
	}
	if best == "" && len(vs) > 0 {
		best = vs[len(vs)-1]
	}
	if best == "" {
		loc := Locate(path)
		return "", fmt.Errorf("no %svX.Y.Z tags in %s", loc.TagPrefix(), RepoURL(loc.Repo))
	}
	return best, nil
}

// Download makes sure a module version is in the cache and returns its
// directory. It fetches the tag from git unless offline (XO1303 then).
// The tree is `git archive` of the tag (of the module's directory at the
// tag `<dir>/vX.Y.Z` for a module in a subdirectory): nothing in it runs.
// Nested modules (directories with an xo.mod) are left out. The
// extracted tree is checked to declare the requested module path and made
// read only.
func Download(path, version string) (string, *Problem) {
	dir := VersionDir(path, version)
	if _, err := os.Stat(filepath.Join(dir, "xo.mod")); err == nil {
		return dir, nil
	}
	if Offline() {
		return "", &Problem{Code: diag.ModuleOffline, Module: path, Msg: fmt.Sprintf("%s %s is not in the module cache and the build is offline", path, version),
			Notes: []string{"fetch it once without XO_OFFLINE / --offline (`xo mod tidy` or `xo get " + path + "@" + version + "`), or vendor it with `xo vendor`", "module cache: " + CacheDir()}}
	}
	loc := Locate(path)
	tag := loc.Tag(version)
	fetchErr := func(err error) *Problem {
		where := "the module path is its git location: " + RepoURL(loc.Repo)
		if loc.Subdir != "" {
			where = "the module is the directory " + loc.Subdir + "/ of the repository " + RepoURL(loc.Repo) + ", versions are tags " + loc.TagPrefix() + "vX.Y.Z"
		}
		return &Problem{Code: diag.ModuleFetch, Module: path, Msg: fmt.Sprintf("cannot fetch %s %s: %v", path, version, err),
			Notes: []string{where + " (XO_GIT_BASE or XO_GIT_REWRITE change it)"}}
	}
	repo, err := repoDir(loc.Repo, false)
	if err == nil {
		if _, err := git(repo, "rev-parse", "--verify", "--quiet", "refs/tags/"+tag+"^{commit}"); err != nil {
			repo, err = repoDir(loc.Repo, true)
		}
	} else {
		repo, err = repoDir(loc.Repo, true)
	}
	if err != nil {
		return "", fetchErr(err)
	}
	if _, err := git(repo, "rev-parse", "--verify", "--quiet", "refs/tags/"+tag+"^{commit}"); err != nil {
		return "", fetchErr(fmt.Errorf("no tag %s", tag))
	}
	treeish := "refs/tags/" + tag
	if loc.Subdir != "" {
		treeish += ":" + loc.Subdir
	}
	data, err := git(repo, "archive", "--format=tar", treeish)
	if err != nil {
		if loc.Subdir != "" {
			err = fmt.Errorf("the tag %s has no directory %s/", tag, loc.Subdir)
		}
		return "", fetchErr(err)
	}
	if err := os.MkdirAll(filepath.Dir(dir), 0o755); err != nil {
		return "", fetchErr(err)
	}
	tmp, err := os.MkdirTemp(filepath.Dir(dir), ".tmp-")
	if err != nil {
		return "", fetchErr(err)
	}
	cleanup := func() { makeWritable(tmp); os.RemoveAll(tmp) }
	if err := untar(data, tmp); err != nil {
		cleanup()
		return "", fetchErr(err)
	}
	if err := pruneNested(tmp); err != nil {
		cleanup()
		return "", fetchErr(err)
	}
	mf, probs := ReadFile(tmp)
	if len(probs) > 0 && (mf == nil || mf.Path == "") {
		cleanup()
		where := "at the repository root"
		if loc.Subdir != "" {
			where = "in " + loc.Subdir + "/"
		}
		return "", fetchErr(fmt.Errorf("the tag has no valid xo.mod %s", where))
	}
	if mf.Path != path {
		cleanup()
		return "", &Problem{Code: diag.ModuleFetch, Module: path, Msg: fmt.Sprintf("%s %s declares module %s in its xo.mod, not %s", path, version, mf.Path, path)}
	}
	makeReadOnly(tmp)
	if err := os.Rename(tmp, dir); err != nil {
		cleanup()
		if _, err2 := os.Stat(filepath.Join(dir, "xo.mod")); err2 != nil {
			return "", fetchErr(err)
		}
	}
	return dir, nil
}

// untar extracts regular files and directories of a tar stream into dir,
// refusing paths that leave it. Symbolic links and other entries are
// skipped.
func untar(data []byte, dir string) error {
	tr := tar.NewReader(bytes.NewReader(data))
	for {
		h, err := tr.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		name := filepath.FromSlash(h.Name)
		if !filepath.IsLocal(name) {
			return fmt.Errorf("archive entry %q leaves the module", h.Name)
		}
		target := filepath.Join(dir, name)
		switch h.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return err
			}
			body, err := io.ReadAll(tr)
			if err != nil {
				return err
			}
			if err := os.WriteFile(target, body, 0o644); err != nil {
				return err
			}
		}
	}
}

// makeReadOnly removes write permission from every file and directory
// under dir (the cache is read only after extraction).
func makeReadOnly(dir string) {
	var dirs []string
	filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			dirs = append(dirs, p)
			return nil
		}
		os.Chmod(p, 0o444)
		return nil
	})
	// directories last, deepest first, so the walk could still list them
	for i := len(dirs) - 1; i >= 0; i-- {
		os.Chmod(dirs[i], 0o555)
	}
}

// makeWritable gives the owner write permission on everything under dir,
// so it can be removed (tests, `xo mod clean`).
func makeWritable(dir string) {
	os.Chmod(dir, 0o755)
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

// CleanCache removes the whole module cache.
func CleanCache() error {
	dir := CacheDir()
	makeWritable(dir)
	return os.RemoveAll(dir)
}
