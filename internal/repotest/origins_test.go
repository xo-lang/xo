package repotest

import (
	"bufio"
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// originProjects are the outside projects whose name in a shipped
// file's comment needs an `Origin:` marker of the same key in that file,
// or an allowlist line (decision 0302). zlib counts only as the project
// ("zlib's", madler): Go's compress/zlib and the zlib format (RFC 1950)
// are named everywhere in std/compress. freebsd counts only as FreeBSD's
// code (its libc, libm or msun, its source tree): FreeBSD is a target
// (decision 0573), so its name alone describes the OS, as Linux's does.
var originProjects = map[string]*regexp.Regexp{
	"musl":       regexp.MustCompile(`(?i)\bmusl\b`),
	"glibc":      regexp.MustCompile(`(?i)\bglibc\b`),
	"bionic":     regexp.MustCompile(`(?i)\bbionic\b`),
	"freebsd":    regexp.MustCompile(`(?i)\bfreebsd(?:'s)?\s+(?:libc|libm|msun|src|sources?|source tree)\b|\bmsun\b|freebsd\.org/src`),
	"openbsd":    regexp.MustCompile(`(?i)\bopenbsd\b`),
	"fdlibm":     regexp.MustCompile(`(?i)\bfdlibm\b`),
	"cephes":     regexp.MustCompile(`(?i)\bcephes\b`),
	"boringssl":  regexp.MustCompile(`(?i)\bboringssl\b`),
	"wycheproof": regexp.MustCompile(`(?i)\bwycheproof\b`),
	"mozilla":    regexp.MustCompile(`(?i)\bmozilla\b`),
	"zlib":       regexp.MustCompile(`(?i)\bzlib's\b|\bmadler\b`),
	"sqlite":     regexp.MustCompile(`(?i)\bsqlite\b`),
}

var (
	originMarker = regexp.MustCompile(`^\s*(?://+|#+|--|\*|<!--)?\s*Origin: ([a-z0-9-]+)\b`)
	noticesItem  = regexp.MustCompile(`^(\d+)\. (.*)$`)
	noticesKey   = regexp.MustCompile(`\[([a-z0-9-]+)\]`)
)

// TestThirdPartyOrigins keeps THIRD_PARTY_NOTICES complete (decision
// 0302): every notices item carries its keys on its heading line
// (`5. musl libc [musl]`); every `Origin: KEY` marker in compiler/, std/,
// scripts/, and cmd/ names an item's key; every item is named by a
// marker, or is `[binary]` (shipped by the release script, not in the
// tree) or `[go]` (Go is one blanket item: the files ported from Go say
// which Go code they follow); and a shipped source file (compiler/, std/,
// cmd/; not tests or testdata) whose comments name an outside project
// (originProjects) has that project's marker, or the line is in
// testdata/origins-allow.txt, the reviewed references that copy nothing.
func TestThirdPartyOrigins(t *testing.T) {
	notices, err := os.ReadFile(filepath.Join(root, "THIRD_PARTY_NOTICES"))
	if err != nil {
		t.Fatal(err)
	}
	keys := map[string]string{} // key to its item's heading
	for _, line := range strings.Split(string(notices), "\n") {
		m := noticesItem.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		ks := noticesKey.FindAllStringSubmatch(m[2], -1)
		if len(ks) == 0 {
			t.Errorf("THIRD_PARTY_NOTICES item %q has no [key] on its heading line (decision 0302)", line)
		}
		for _, k := range ks {
			keys[k[1]] = line
		}
	}
	out, err := exec.Command("git", "-C", root, "ls-files", "-z", "compiler", "std", "scripts", "cmd").Output()
	if err != nil {
		t.Skipf("git ls-files: %v", err)
	}
	allow := readOriginsAllow(t)
	// a README.md's `Origin: KEY` line names the unchanged copies beside
	// it (std/native/sqlite's amalgamation): their own comments are the
	// outside project's, not Xo's, so they are not scanned
	readmes := map[string]string{}
	for _, name := range bytes.Split(out, []byte{0}) {
		f := string(name)
		if filepath.Base(f) != "README.md" {
			continue
		}
		if data, err := os.ReadFile(filepath.Join(root, f)); err == nil {
			readmes[f] = string(data)
		}
	}
	vendored := originVendored(readmes)
	used := map[string]bool{}
	allowUsed := map[string]bool{}
	for _, name := range bytes.Split(out, []byte{0}) {
		f := string(name)
		if f == "" {
			continue
		}
		data, err := os.ReadFile(filepath.Join(root, f))
		if err != nil || bytes.IndexByte(data, 0) >= 0 {
			continue
		}
		lines := strings.Split(string(data), "\n")
		marked := map[string]bool{}
		for i, line := range lines {
			if m := originMarker.FindStringSubmatch(line); m != nil {
				if _, ok := keys[m[1]]; !ok {
					t.Errorf("%s:%d: Origin key %q names no THIRD_PARTY_NOTICES item (add the item's [key], decision 0302)", f, i+1, m[1])
				}
				marked[m[1]] = true
				used[m[1]] = true
			}
		}
		if !originShipped(f) || vendored[f] {
			continue
		}
		hash := strings.HasSuffix(f, ".py") || strings.HasSuffix(f, ".sh")
		for i, line := range lines {
			if strings.HasPrefix(strings.TrimSpace(line), "//go:build") {
				continue
			}
			k := strings.Index(line, "//")
			if hash {
				k = strings.Index(line, "#")
			}
			if k < 0 {
				continue
			}
			c := strings.TrimSpace(line[k:])
			for p, rx := range originProjects {
				if !rx.MatchString(c) || marked[p] {
					continue
				}
				if allow[f+"\t"+c] {
					allowUsed[f+"\t"+c] = true
					continue
				}
				t.Errorf("%s:%d: names %s with no `Origin: %s` marker in the file: mark a copy or port (and its THIRD_PARTY_NOTICES item), or add a reviewed reference to internal/repotest/testdata/origins-allow.txt (decision 0302): %s", f, i+1, p, p, c)
			}
		}
	}
	for k, line := range keys {
		if !used[k] && k != "binary" && k != "go" {
			t.Errorf("THIRD_PARTY_NOTICES %q: no `Origin: %s` marker names it (decision 0302)", line, k)
		}
	}
	for k := range allow {
		if !allowUsed[k] {
			t.Errorf("internal/repotest/testdata/origins-allow.txt: %q matches no line; remove it", strings.Replace(k, "\t", " | ", 1))
		}
	}
}

// originVendored returns the files that README.md files (path to text)
// name on an `Origin: KEY` line, beside the README: the unchanged copies
// whose own comments are not scanned. A copy no README's Origin line names
// is scanned as any file.
func originVendored(readmes map[string]string) map[string]bool {
	vendored := map[string]bool{}
	for f, text := range readmes {
		for _, line := range strings.Split(text, "\n") {
			if !originMarker.MatchString(line) {
				continue
			}
			for _, w := range strings.FieldsFunc(line, func(r rune) bool { return r == ' ' || r == ',' || r == '(' || r == ')' }) {
				vendored[filepath.ToSlash(filepath.Dir(f))+"/"+w] = true
			}
		}
	}
	return vendored
}

// TestOriginVendored: only a file a README's Origin line names counts as
// a vendored copy; a file beside it the line does not name, a file in
// another directory, and a README whose mention is not an Origin line
// mark nothing, so an unmarked copy still fails TestThirdPartyOrigins.
func TestOriginVendored(t *testing.T) {
	v := originVendored(map[string]string{
		"std/native/lib/README.md":   "# lib\n\nOrigin: sqlite the amalgamation (lib.c, lib.h), copied unchanged\n\n- [extra.c](extra.c): ours\n",
		"std/native/other/README.md": "# other\n\nCopied from SQLite (other.c), no marker line\n",
	})
	for f, want := range map[string]bool{
		"std/native/lib/lib.c":     true,
		"std/native/lib/lib.h":     true,
		"std/native/lib/extra.c":   false,
		"std/native/x/lib.c":       false,
		"std/native/other/other.c": false,
	} {
		if v[f] != want {
			t.Errorf("originVendored: %s = %v, want %v", f, v[f], want)
		}
	}
}

// originShipped reports whether f is a source file the release ships or
// builds into xo (decision 0302): compiler/, std/, cmd/, not tests or
// testdata.
func originShipped(f string) bool {
	if !(strings.HasPrefix(f, "compiler/") || strings.HasPrefix(f, "std/") || strings.HasPrefix(f, "cmd/")) {
		return false
	}
	if strings.Contains(f, "/testdata/") || strings.HasSuffix(f, "_test.xo") || strings.HasSuffix(f, "_test.go") {
		return false
	}
	for _, e := range []string{".xo", ".go", ".c", ".h", ".s", ".py", ".sh"} {
		if strings.HasSuffix(f, e) {
			return true
		}
	}
	return false
}

// readOriginsAllow reads testdata/origins-allow.txt: `path<TAB>comment`
// lines (the comment as the file has it, trimmed), each after a `#`
// line saying why it copies nothing.
func readOriginsAllow(t *testing.T) map[string]bool {
	f, err := os.Open(filepath.Join(root, "internal", "repotest", "testdata", "origins-allow.txt"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	allow := map[string]bool{}
	s := bufio.NewScanner(f)
	for s.Scan() {
		line := s.Text()
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if !strings.Contains(line, "\t") {
			t.Errorf("origins-allow.txt: %q is not path<TAB>comment", line)
			continue
		}
		allow[line] = true
	}
	return allow
}
