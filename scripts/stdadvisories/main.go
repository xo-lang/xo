// Command stdadvisories lists the Go vulnerability database's standard
// library advisories that a std module's change log does not mention
// (task #365):
//
//	go run ./scripts/stdadvisories [-db DIR] [-since 1.26.0] [-repo DIR]
//
// Std ports Go's standard library (decision 0276), so a fix in Go's
// package is a question for every std module that ports it: affected
// (port the fix and its test), or not (say why). Either answer is a
// std/changes/MODULE.md entry naming the advisory's ID, or a line of the
// module's decision 0616 review (std/evidence/MODULE/review.md). An
// advisory is listed when a Go package it affects maps to std modules (the
// "Go packages" table of knowledgebase/infrastructure/std-coverage.md) and
// no change log or review of those modules names its ID. Only advisories fixed after
// -since count (the Go release std was ported from), and a package under
// internal/ counts as its parent (net/http/internal/http2 is net/http;
// internal/syscall/unix has none and is skipped).
//
// The database is vuln.go.dev's OSV layout (decision 0647):
// index/modules.json and ID/GO-YYYY-NNNN.json. -db reads a local copy;
// without it the files are fetched from https://vuln.go.dev into the user
// cache (xo/vulndb), an ID's file again only when the index says it
// changed. The output is one line an advisory, `ID PACKAGE MODULES
// SUMMARY`; the exit status is 1 when any is listed.
package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

const base = "https://vuln.go.dev"

type indexModule struct {
	Path  string `json:"path"`
	Vulns []struct {
		ID       string `json:"id"`
		Modified string `json:"modified"`
		Fixed    string `json:"fixed"`
	} `json:"vulns"`
}

type entry struct {
	ID       string `json:"id"`
	Summary  string `json:"summary"`
	Affected []struct {
		Package struct {
			Name string `json:"name"`
		} `json:"package"`
		Ranges []struct {
			Events []map[string]string `json:"events"`
		} `json:"ranges"`
		EcosystemSpecific struct {
			Imports []struct {
				Path string `json:"path"`
			} `json:"imports"`
		} `json:"ecosystem_specific"`
	} `json:"affected"`
}

// finding is one advisory no mapped change log names.
type finding struct {
	ID, Pkg, Summary string
	Modules          []string
}

func main() {
	db := flag.String("db", "", "a local copy of vuln.go.dev (index/modules.json, ID/*.json); empty fetches into the user cache")
	since := flag.String("since", "1.26.0", "the Go release std was ported from: advisories fixed at or before it are skipped")
	repo := flag.String("repo", ".", "the Xo checkout")
	flag.Parse()
	src := source{dir: *db}
	if *db == "" {
		c, err := os.UserCacheDir()
		if err != nil {
			fail(err)
		}
		src = source{dir: filepath.Join(c, "xo", "vulndb"), fetch: true}
	}
	out, err := run(src, *repo, *since)
	if err != nil {
		fail(err)
	}
	for _, f := range out {
		fmt.Printf("%s %s %s %s\n", f.ID, f.Pkg, strings.Join(f.Modules, ","), f.Summary)
	}
	if len(out) > 0 {
		os.Exit(1)
	}
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "stdadvisories:", err)
	os.Exit(2)
}

// source reads the database from dir, first fetching what is missing or
// changed when fetch is set.
type source struct {
	dir   string
	fetch bool
}

func (s source) index() ([]byte, error) {
	p := filepath.Join(s.dir, "index", "modules.json")
	if s.fetch {
		if err := download(base+"/index/modules.json", p); err != nil {
			return nil, err
		}
	}
	return os.ReadFile(p)
}

// entry reads ID's file, fetching it when it is missing or older than the
// index's modified time.
func (s source) entry(id, modified string) (*entry, error) {
	p := filepath.Join(s.dir, "ID", id+".json")
	if s.fetch {
		st, err := os.Stat(p)
		m, perr := time.Parse(time.RFC3339, modified)
		if err != nil || perr != nil || st.ModTime().Before(m) {
			if err := download(base+"/ID/"+id+".json", p); err != nil {
				return nil, err
			}
		}
	}
	b, err := os.ReadFile(p)
	if err != nil {
		return nil, err
	}
	var e entry
	if err := json.Unmarshal(b, &e); err != nil {
		return nil, fmt.Errorf("%s: %v", p, err)
	}
	return &e, nil
}

func download(url, dst string) error {
	c := &http.Client{Timeout: time.Minute}
	r, err := c.Get(url)
	if err != nil {
		return err
	}
	defer r.Body.Close()
	if r.StatusCode != 200 {
		return fmt.Errorf("%s: %s", url, r.Status)
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	tmp := dst + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	if _, err := io.Copy(f, r.Body); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, dst)
}

func run(src source, repo, since string) ([]finding, error) {
	pkgs, err := mapping(filepath.Join(repo, "knowledgebase", "infrastructure", "std-coverage.md"))
	if err != nil {
		return nil, err
	}
	b, err := src.index()
	if err != nil {
		return nil, err
	}
	var idx []indexModule
	if err := json.Unmarshal(b, &idx); err != nil {
		return nil, fmt.Errorf("index/modules.json: %v", err)
	}
	min := version(since)
	logs := map[string]string{}
	logOf := func(m string) string {
		if s, ok := logs[m]; ok {
			return s
		}
		b, _ := os.ReadFile(filepath.Join(repo, "std", "changes", m+".md"))
		r, _ := os.ReadFile(filepath.Join(repo, "std", "evidence", m, "review.md"))
		logs[m] = string(b) + string(r)
		return logs[m]
	}
	seen := map[string]bool{}
	var out []finding
	for _, m := range idx {
		if m.Path != "stdlib" {
			continue
		}
		for _, v := range m.Vulns {
			// an ID has a line per fixed release; one past since is enough
			if seen[v.ID] || (v.Fixed != "" && !less(min, version(v.Fixed))) {
				continue
			}
			seen[v.ID] = true
			e, err := src.entry(v.ID, v.Modified)
			if err != nil {
				return nil, err
			}
			out = append(out, check(e, pkgs, logOf)...)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].ID != out[j].ID {
			return out[i].ID < out[j].ID
		}
		return out[i].Pkg < out[j].Pkg
	})
	return out, nil
}

// check returns a finding for each mapped package of e whose modules'
// change logs and reviews all lack e's ID. Two packages mapping to the same modules
// (net/http and its internal http2) give one finding.
func check(e *entry, pkgs map[string][]string, logOf func(string) string) []finding {
	var out []finding
	done := map[string]bool{}
	for _, a := range e.Affected {
		if a.Package.Name != "stdlib" {
			continue
		}
		for _, imp := range a.EcosystemSpecific.Imports {
			p := parent(imp.Path)
			mods := pkgs[p]
			if len(mods) == 0 || done[p] {
				continue
			}
			done[p] = true
			named := false
			for _, m := range mods {
				if strings.Contains(logOf(m), e.ID) {
					named = true
				}
			}
			if !named {
				out = append(out, finding{ID: e.ID, Pkg: p, Modules: mods, Summary: e.Summary})
			}
		}
	}
	return out
}

// parent is the package an internal package belongs to: the path before
// "/internal/", or "" for a top level internal package.
func parent(p string) string {
	if strings.HasPrefix(p, "internal/") {
		return ""
	}
	if i := strings.Index(p, "/internal/"); i >= 0 {
		return p[:i]
	}
	return p
}

var (
	stdModule = regexp.MustCompile(`\bstd/([a-z0-9_]+)\b`)
	prelude   = regexp.MustCompile(`^prelude\b`)
)

// mapping reads the "## Go packages" table: the first column's packages
// ("crypto/x509, crypto/x509/pkix", "net/http (server)") map to the std
// modules the second column names (std/NAME, or the prelude when the cell
// starts with "prelude"). std/native is the compiler's runtime, not a
// module with a change log, and is left out.
func mapping(md string) (map[string][]string, error) {
	f, err := os.Open(md)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	out := map[string][]string{}
	add := func(p, m string) {
		for _, x := range out[p] {
			if x == m {
				return
			}
		}
		out[p] = append(out[p], m)
	}
	in := false
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for sc.Scan() {
		l := sc.Text()
		if strings.HasPrefix(l, "## ") {
			in = l == "## Go packages"
			continue
		}
		if !in || !strings.HasPrefix(l, "| ") {
			continue
		}
		cols := strings.Split(l, "|")
		if len(cols) < 4 {
			continue
		}
		cell, mods := strings.TrimSpace(cols[1]), strings.TrimSpace(cols[2])
		var ms []string
		if prelude.MatchString(mods) {
			ms = append(ms, "prelude")
		}
		for _, sm := range stdModule.FindAllStringSubmatch(mods, -1) {
			if sm[1] != "native" {
				ms = append(ms, sm[1])
			}
		}
		for _, p := range strings.Split(cell, ",") {
			p = strings.Trim(strings.TrimSpace(p), "`")
			if i := strings.Index(p, " ("); i > 0 {
				p = p[:i]
			}
			if p == "" || strings.ContainsAny(p, " *") {
				continue
			}
			for _, m := range ms {
				add(p, m)
			}
		}
	}
	return out, sc.Err()
}

// version reads "1.26.4" or "1.27.0-rc.1" as its numbers (a release
// candidate as its release).
func version(s string) [3]int {
	var v [3]int
	for i, p := range strings.SplitN(strings.TrimPrefix(s, "go"), ".", 3) {
		if j := strings.IndexAny(p, "-+"); j >= 0 {
			p = p[:j]
		}
		v[i], _ = strconv.Atoi(p)
	}
	return v
}

func less(a, b [3]int) bool {
	for i := range a {
		if a[i] != b[i] {
			return a[i] < b[i]
		}
	}
	return false
}
