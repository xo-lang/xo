package repotest

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"
	"testing"

	"github.com/xo-lang/xo/internal/stdgen"
)

// TestStdRTBuildTags builds std/rt under each xo_* build tag its files
// name (found from their //go:build lines), one tag at a time, so a file
// that calls into a tagged file without a stub for the other side fails
// here instead of in every Go-backend build that sets the tag (4b6058367:
// http_client_stream.go called http_net.go's `do`, which xo_nohttp
// builds leave out; gotchas/go-rt-nohttp-tag.md). A few seconds: the
// builds share Go's build cache.
func TestStdRTBuildTags(t *testing.T) {
	files, err := filepath.Glob(filepath.Join(root, "std/rt/*.go"))
	if err != nil {
		t.Fatal(err)
	}
	line := regexp.MustCompile(`(?m)^//go:build (.*)$`)
	tag := regexp.MustCompile(`\bxo_[a-z0-9_]+\b`)
	seen := map[string]bool{}
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		src, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		for _, m := range line.FindAllStringSubmatch(string(src), -1) {
			for _, x := range tag.FindAllString(m[1], -1) {
				seen[x] = true
			}
		}
	}
	if !seen["xo_nohttp"] {
		t.Fatal("no //go:build line in std/rt names xo_nohttp: the scan is broken")
	}
	// std/go_optional.txt (task #429 lever C): each listed module's tag
	// must leave something out, and all of them at once (a program that
	// reaches none of the modules) must build too
	opt, err := os.ReadFile(filepath.Join(root, "std/go_optional.txt"))
	if err != nil {
		t.Fatal(err)
	}
	all := []string{"xo_nohttp", "xo_gotimers"}
	var optional []string
	for _, l := range strings.Split(string(opt), "\n") {
		l = strings.TrimSpace(l)
		if l == "" || strings.HasPrefix(l, "#") {
			continue
		}
		x := "xo_no_" + strings.TrimPrefix(l, "std/")
		if !seen[x] {
			t.Errorf("std/go_optional.txt lists %s, but no //go:build line in std/rt names %s", l, x)
		}
		all = append(all, x)
		optional = append(optional, strings.TrimPrefix(l, "std/"))
	}
	// a program that does not reach module m reaches no module that uses
	// m either (stdgen.Reaching), so m's tag is built with theirs: alone,
	// xo_no_tls would leave out types that std/http's Go names
	with := func(m string) string {
		r := stdgen.Reaching(root, m)
		var ts []string
		if r["http"] {
			ts = append(ts, "xo_nohttp")
		}
		for _, o := range optional {
			if r[o] {
				ts = append(ts, "xo_no_"+o)
			}
		}
		return strings.Join(ts, ",")
	}
	tags := []string{"", strings.Join(all, ",")}
	for x := range seen {
		switch m, ok := strings.CutPrefix(x, "xo_no_"); {
		case x == "xo_nohttp":
			tags = append(tags, with("http"))
		case ok && slices.Contains(optional, m):
			tags = append(tags, with(m))
		default:
			tags = append(tags, x)
		}
	}
	sort.Strings(tags)
	for _, x := range tags {
		args := []string{"build"}
		if x != "" {
			args = append(args, "-tags", x)
		}
		cmd := exec.Command("go", append(args, "./std/rt")...)
		cmd.Dir = root
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Errorf("go build -tags %q ./std/rt: %v\n%s", x, err, out)
		}
	}
}

// TestStdRTNoHTTPDeps checks that std/rt built for a Go backend program
// that does not reach std/http (compiler/gen/gen.xo's xo_nohttp) imports
// no net/http package, and one that does not reach std/tls, std/x509 or
// std/crypto (their xo_no_ tags, with the tags of the modules reaching
// them) no crypto/tls or crypto/x509: each package's init ran at every
// such program's start (net/http about 0.08 ms, crypto/tls 0.06, x509
// 0.04 on linux/arm64 host 2, task #429). Before task #472 the untagged
// files (testing.go's fake network, replay.go's servers, attenuate.go's
// dispatch, the std/http, std/tls and std/x509 methods) imported them.
func TestStdRTNoHTTPDeps(t *testing.T) {
	gates := stdgen.Gates(root)
	// a program that does not reach m reaches no module that uses m
	with := func(m string) string {
		r := stdgen.Reaching(root, m)
		var ts []string
		for _, g := range gates {
			if r[g.Mod] {
				ts = append(ts, g.Tag)
			}
		}
		return strings.Join(ts, ",")
	}
	for _, c := range []struct{ mod, pkg string }{
		{"http", "net/http"},
		{"tls", "crypto/tls"},
		// std/crypto's key parsing (cryptokeys.go) is crypto/x509's, so
		// only a program not reaching std/crypto leaves it out
		{"crypto", "crypto/x509"},
	} {
		tags := with(c.mod)
		cmd := exec.Command("go", "list", "-deps", "-tags", tags, "./std/rt")
		cmd.Dir = root
		out, err := cmd.Output()
		if err != nil {
			t.Fatalf("go list -deps -tags %s ./std/rt: %v", tags, err)
		}
		var bad []string
		for _, p := range strings.Fields(string(out)) {
			if p == c.pkg || strings.HasPrefix(p, c.pkg+"/") {
				bad = append(bad, p)
			}
		}
		if len(bad) > 0 {
			t.Errorf("std/rt under %s (a program not reaching std/%s) imports %s: a file that uses the module's types or %s needs its //go:build line, with a stand in under the tag for what the rest calls", tags, c.mod, strings.Join(bad, ", "), c.pkg)
		}
	}
}
