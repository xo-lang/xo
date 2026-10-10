package mod

import (
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/xo-lang/xo/internal/diag"
	"github.com/xo-lang/xo/internal/sumdb"
)

// Fetching (decisions 0086, 0124, 0126): a module version comes from the
// sources XO_PROXY lists, in order, as Go's GOPROXY: a proxy URL, `direct`
// (git), or `off`. After a comma the next source is tried only when the
// previous one answered "not found" (404 or 410); after a pipe, on any
// error. The default is `direct`. XO_NOPROXY patterns name modules that
// always come from git. Whatever the source, the tree must declare the
// module path; when XO_SUMDB names a checksum log it must also agree with
// the log (checked once per version, on its first fetch, unless
// XO_NOSUMDB matches the path). Every build then verifies the cached tree
// against xo.lock as before.

// source is one entry of XO_PROXY.
type source struct {
	URL    string // a proxy's base URL, "" for direct and off
	Direct bool
	Off    bool
	// AnyError is set when a pipe follows the entry: the next source is
	// tried after any error, not only "not found".
	AnyError bool
}

func (s source) String() string {
	switch {
	case s.Direct:
		return "direct"
	case s.Off:
		return "off"
	}
	return s.URL
}

// ParseProxy parses an XO_PROXY value; "" is `direct`.
func ParseProxy(s string) ([]source, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return []source{{Direct: true}}, nil
	}
	var out []source
	for s != "" {
		item, sep := s, byte(0)
		if i := strings.IndexAny(s, ",|"); i >= 0 {
			item, sep, s = s[:i], s[i], s[i+1:]
			if strings.TrimSpace(s) == "" {
				return nil, fmt.Errorf("XO_PROXY ends with %q", string(sep))
			}
		} else {
			s = ""
		}
		item = strings.TrimSpace(item)
		e := source{AnyError: sep == '|'}
		switch {
		case item == "direct":
			e.Direct = true
		case item == "off":
			e.Off = true
		case strings.HasPrefix(item, "https://") || strings.HasPrefix(item, "http://"):
			e.URL = strings.TrimRight(item, "/")
		default:
			return nil, fmt.Errorf("XO_PROXY entry %q: want a http(s) URL, direct, or off", item)
		}
		out = append(out, e)
	}
	return out, nil
}

// MatchPatterns reports whether a module path matches one of the comma
// separated glob patterns of XO_NOPROXY or XO_NOSUMDB: a pattern matches
// the path when it matches (path.Match) as many leading elements of the
// path as it has (`*.corp.example.com`, `github.com/acme/*`,
// `github.com/acme/private` all match `github.com/acme/private/sub`).
func MatchPatterns(patterns, modPath string) bool {
	elems := strings.Split(modPath, "/")
	for _, g := range strings.Split(patterns, ",") {
		g = strings.Trim(strings.TrimSpace(g), "/")
		if g == "" {
			continue
		}
		n := strings.Count(g, "/") + 1
		if len(elems) < n {
			continue
		}
		if ok, _ := path.Match(g, strings.Join(elems[:n], "/")); ok {
			return true
		}
	}
	return false
}

// settings are the sources and the checksum log for one module path.
type settings struct {
	sources []source
	sum     *sumdb.Client // nil: no checksum log for this path
	err     error         // malformed settings (XO1314)
}

// directOnly fetches from git and consults no log (a proxy server
// filling its cache, and log servers computing records).
var directOnly = &settings{sources: []source{{Direct: true}}}

// settingsFor reads XO_PROXY, XO_NOPROXY, XO_SUMDB, and XO_NOSUMDB for
// the module path; the client keeps the log's latest tree head under the
// module cache at root.
func settingsFor(root, modPath string) *settings {
	st := &settings{}
	if MatchPatterns(os.Getenv("XO_NOPROXY"), modPath) {
		st.sources = []source{{Direct: true}}
	} else {
		st.sources, st.err = ParseProxy(os.Getenv("XO_PROXY"))
	}
	if st.err != nil {
		return st
	}
	cfg := strings.TrimSpace(os.Getenv("XO_SUMDB"))
	if cfg == "" || cfg == "off" || MatchPatterns(os.Getenv("XO_NOSUMDB"), modPath) {
		return st
	}
	v, url, err := sumdb.ParseConfig(cfg)
	if err != nil {
		st.err = err
		return st
	}
	st.sum = &sumdb.Client{URL: url, Verifier: v, StateDir: filepath.Join(root, "cache", "sumdb")}
	return st
}

func settingsProblem(modPath string, err error) *Problem {
	return &Problem{Code: diag.FetchSettings, Module: modPath, Msg: err.Error(),
		Notes: []string{"XO_PROXY is a list of proxy URLs, direct, and off (default direct); XO_SUMDB is off (the default) or <name>+<key id>+<public key> [<URL>]; XO_NOPROXY and XO_NOSUMDB are comma separated path patterns"}}
}

// cacheVersions lists the versions of a module extracted in the module
// cache at root.
func cacheVersions(root, modPath string) []string {
	var vs []string
	entries, err := os.ReadDir(filepath.Dir(versionDirIn(root, modPath, "v0.0.0")))
	if err != nil {
		return nil
	}
	prefix := filepath.Base(filepath.FromSlash(EscapePath(modPath))) + "@"
	for _, e := range entries {
		if v, ok := strings.CutPrefix(e.Name(), prefix); ok && ValidVersion(v) {
			if _, err := os.Stat(filepath.Join(filepath.Dir(versionDirIn(root, modPath, v)), e.Name(), "xo.mod")); err == nil {
				vs = append(vs, v)
			}
		}
	}
	return vs
}

// gitVersions lists the version tags of a module's repository.
func gitVersions(root, modPath string) ([]string, error) {
	loc := locateIn(root, modPath)
	dir, err := repoDir(root, loc.Repo, true)
	if err != nil {
		return nil, err
	}
	out, err := git(dir, "tag", "--list", loc.TagPrefix()+"v*")
	if err != nil {
		return nil, err
	}
	var vs []string
	for _, t := range strings.Fields(string(out)) {
		if t, ok := strings.CutPrefix(t, loc.TagPrefix()); ok {
			vs = append(vs, t)
		}
	}
	return vs, nil
}

// Versions lists the versions of a module, lowest first: from the first
// XO_PROXY source that knows the module (the vX.Y.Z tags of its
// repository, or the <dir>/vX.Y.Z tags for a module in directory <dir>,
// when direct). Offline, it lists the versions in the module cache.
func Versions(modPath string) ([]string, error) {
	root := CacheDir()
	if Offline() {
		return sortVersions(modPath, cacheVersions(root, modPath)), nil
	}
	st := settingsFor(root, modPath)
	if st.err != nil {
		return nil, st.err
	}
	return versionsFrom(root, modPath, st)
}

func versionsFrom(root, modPath string, st *settings) ([]string, error) {
	var errs []string
	for _, s := range st.sources {
		var vs []string
		var err error
		notFound := false
		switch {
		case s.Off:
			err = errors.New("module lookup is turned off (XO_PROXY=off)")
		case s.Direct:
			vs, err = gitVersions(root, modPath)
		default:
			vs, notFound, err = proxyList(s.URL, modPath)
		}
		if err == nil {
			return sortVersions(modPath, vs), nil
		}
		errs = append(errs, err.Error())
		if s.Off || !(notFound || s.AnyError) {
			break
		}
	}
	return nil, errors.New(strings.Join(errs, "; "))
}

// sortVersions keeps the valid versions the module path's major admits,
// lowest first.
func sortVersions(modPath string, vs []string) []string {
	var out []string
	seen := map[string]bool{}
	for _, v := range vs {
		if ValidVersion(v) && MajorMismatch(modPath, v) == "" && !seen[v] {
			seen[v] = true
			out = append(out, v)
		}
	}
	sort.Slice(out, func(i, j int) bool { return Compare(out[i], out[j]) < 0 })
	return out
}

// Latest is the highest release version of a module (prereleases only
// when there is no release).
func Latest(modPath string) (string, error) {
	vs, err := Versions(modPath)
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
		loc := Locate(modPath)
		return "", fmt.Errorf("no %svX.Y.Z tags in %s", loc.TagPrefix(), RepoURL(loc.Repo))
	}
	return best, nil
}

// gitFetch extracts the tree of a module version from its repository
// into dst: `git archive` of the tag (of the module's directory at the
// tag `<dir>/vX.Y.Z` for a module in a subdirectory). Nothing in it runs.
func gitFetch(root, modPath, version, dst string) error {
	loc := locateIn(root, modPath)
	tag := loc.Tag(version)
	repo, err := repoDir(root, loc.Repo, false)
	if err == nil {
		if _, err := git(repo, "rev-parse", "--verify", "--quiet", "refs/tags/"+tag+"^{commit}"); err != nil {
			repo, err = repoDir(root, loc.Repo, true)
		}
	} else {
		repo, err = repoDir(root, loc.Repo, true)
	}
	if err != nil {
		return err
	}
	if _, err := git(repo, "rev-parse", "--verify", "--quiet", "refs/tags/"+tag+"^{commit}"); err != nil {
		return fmt.Errorf("no tag %s", tag)
	}
	treeish := "refs/tags/" + tag
	if loc.Subdir != "" {
		treeish += ":" + loc.Subdir
	}
	data, err := git(repo, "archive", "--format=tar", treeish)
	if err != nil {
		if loc.Subdir != "" {
			return fmt.Errorf("the tag %s has no directory %s/", tag, loc.Subdir)
		}
		return err
	}
	return untar(data, dst)
}

// Download makes sure a module version is in the module cache and
// returns its directory. Unless offline (XO1303 then), it fetches the
// version from the XO_PROXY sources. Nested modules (directories with an
// xo.mod) are left out. The tree is checked to declare the requested
// module path and, with XO_SUMDB, against the checksum log, then made
// read only.
func Download(modPath, version string) (string, *Problem) {
	return download(CacheDir(), modPath, version, nil)
}

// download is Download into the module cache at root; st nil reads the
// settings from the environment.
func download(root, modPath, version string, st *settings) (string, *Problem) {
	dir := versionDirIn(root, modPath, version)
	if _, err := os.Stat(filepath.Join(dir, "xo.mod")); err == nil {
		return dir, nil
	}
	if Offline() {
		return "", &Problem{Code: diag.ModuleOffline, Module: modPath, Msg: fmt.Sprintf("%s %s is not in the module cache and the build is offline", modPath, version),
			Notes: []string{"fetch it once without XO_OFFLINE / --offline (`xo mod tidy` or `xo get " + modPath + "@" + version + "`), or vendor it with `xo vendor`", "module cache: " + root}}
	}
	if st == nil {
		st = settingsFor(root, modPath)
	}
	if st.err != nil {
		return "", settingsProblem(modPath, st.err)
	}
	loc := locateIn(root, modPath)
	var notes []string
	fetchErr := func(err error) *Problem {
		return &Problem{Code: diag.ModuleFetch, Module: modPath, Msg: fmt.Sprintf("cannot fetch %s %s: %v", modPath, version, err), Notes: notes}
	}
	if err := os.MkdirAll(filepath.Dir(dir), 0o755); err != nil {
		return "", fetchErr(err)
	}
	tmp, err := os.MkdirTemp(filepath.Dir(dir), ".tmp-")
	if err != nil {
		return "", fetchErr(err)
	}
	cleanup := func() { makeWritable(tmp); os.RemoveAll(tmp) }
	var errs []string
	from := ""
	for _, s := range st.sources {
		emptyDir(tmp)
		var err error
		notFound := false
		switch {
		case s.Off:
			err = errors.New("module fetching is turned off (XO_PROXY=off)")
		case s.Direct:
			err = gitFetch(root, modPath, version, tmp)
			where := "the module path is its git location: " + RepoURL(loc.Repo)
			if loc.Subdir != "" {
				where = "the module is the directory " + loc.Subdir + "/ of the repository " + RepoURL(loc.Repo) + ", versions are tags " + loc.TagPrefix() + "vX.Y.Z"
			}
			notes = append(notes, where+" (XO_GIT_BASE or XO_GIT_REWRITE change it)")
		default:
			notFound, err = proxyFetch(s.URL, modPath, version, tmp)
			notes = append(notes, "module proxy "+s.URL+" (XO_PROXY)")
		}
		if err == nil {
			from = s.String()
			break
		}
		if len(st.sources) > 1 {
			err = fmt.Errorf("%s: %v", s, err)
		}
		errs = append(errs, err.Error())
		if s.Off || !(notFound || s.AnyError) {
			break
		}
	}
	if from == "" {
		cleanup()
		return "", fetchErr(errors.New(strings.Join(errs, "; ")))
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
		if from != "direct" {
			where = "in the tree from " + from
		}
		return "", fetchErr(fmt.Errorf("the version has no valid xo.mod %s", where))
	}
	if mf.Path != modPath {
		cleanup()
		return "", &Problem{Code: diag.ModuleFetch, Module: modPath, Msg: fmt.Sprintf("%s %s declares module %s in its xo.mod, not %s", modPath, version, mf.Path, modPath)}
	}
	if st.sum != nil {
		if p := checkLog(st.sum, tmp, modPath, version, from); p != nil {
			cleanup()
			return "", p
		}
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

// Record is the checksum log record of the module version tree in dir:
// its two xo.lock lines.
func Record(dir, modPath, version string) ([]string, error) {
	tree, err := HashDir(dir)
	if err != nil {
		return nil, err
	}
	modh, err := HashModFile(dir)
	if err != nil {
		return nil, err
	}
	return []string{modPath + " " + version + " " + tree, modPath + " " + version + "/xo.mod " + modh}, nil
}

// LogRecord fetches a module version from git into the module cache at
// root (no proxy, no log) and returns its checksum log record, newline
// terminated: what a log server adds for a version it is asked about.
func LogRecord(root, modPath, version string) ([]byte, error) {
	dir, p := download(root, modPath, version, directOnly)
	if p != nil {
		return nil, errors.New(p.Msg)
	}
	lines, err := Record(dir, modPath, version)
	if err != nil {
		return nil, err
	}
	return []byte(strings.Join(lines, "\n") + "\n"), nil
}

// checkLog verifies the fetched tree in dir against the checksum log.
func checkLog(c *sumdb.Client, dir, modPath, version, from string) *Problem {
	want, err := Record(dir, modPath, version)
	if err != nil {
		return &Problem{Code: diag.ModuleFetch, Module: modPath, Msg: fmt.Sprintf("cannot hash %s %s: %v", modPath, version, err)}
	}
	err = c.Check(modPath, version, want)
	if err == nil {
		return nil
	}
	var se *sumdb.Error
	if errors.As(err, &se) && se.Security {
		return &Problem{Code: diag.ChecksumLog, Module: modPath, Msg: fmt.Sprintf("verifying %s %s (from %s): %v", modPath, version, from, se.Msg),
			Notes: []string{"nothing was cached or built: the source, the log, or the network path to them may be compromised", "if the module is private and never in the public log, add it to XO_NOSUMDB"}}
	}
	return &Problem{Code: diag.FetchSettings, Module: modPath, Msg: fmt.Sprintf("verifying %s %s: %v", modPath, version, err),
		Notes: []string{"the checksum log is named by XO_SUMDB; set XO_NOSUMDB for private modules, or XO_SUMDB=off to fetch without it"}}
}

// emptyDir removes everything inside dir.
func emptyDir(dir string) {
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		p := filepath.Join(dir, e.Name())
		makeWritable(p)
		os.RemoveAll(p)
	}
}
