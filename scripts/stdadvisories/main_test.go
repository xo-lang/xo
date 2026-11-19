package main

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

const coverage = `# Std coverage

## Go packages

| Go package | Xo | Status | Notes |
|---|---|---|---|
| net/http (server) | std/http | done | x |
| net/http (client) | std/http Client | done | x |
| net | prelude Net, std/udp, std/dns | done | x |
| crypto/tls | std/tls, std/http (with_tls) | done | x |
| encoding/asn1 | std/asn1 (std/native/llrt/asn1.xo) | done | x |

## Other

| net/mail | std/mail | done | x |
`

// repo writes a checkout with the coverage table and the given files.
func repo(t *testing.T, files map[string]string) string {
	t.Helper()
	d := t.TempDir()
	files["knowledgebase/infrastructure/std-coverage.md"] = coverage
	for p, s := range files {
		p = filepath.Join(d, p)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(s), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return d
}

func ids(fs []finding) []string {
	var out []string
	for _, f := range fs {
		out = append(out, f.ID+" "+f.Pkg)
	}
	return out
}

// TestUnloggedAdvisory: GO-2026-6613 (net/http, fixed in 1.27.2) is listed
// while std/changes/http.md does not name it, and not once it does. The
// testdata database also holds an advisory fixed before -since, one in a
// top level internal package, and one in net/http and its internal http2
// (one finding), none of which may be listed wrongly.
func TestUnloggedAdvisory(t *testing.T) {
	src := source{dir: filepath.Join("testdata", "db")}
	r := repo(t, map[string]string{"std/changes/http.md": "# http\n\n- Go reference: GO-2026-0003\n"})
	got, err := run(src, r, "1.26.0")
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"GO-2026-6613 net/http"}; !reflect.DeepEqual(ids(got), want) {
		t.Fatalf("listed %q, want %q", ids(got), want)
	}
	if !reflect.DeepEqual(got[0].Modules, []string{"http"}) || got[0].Summary != "a net/http advisory" {
		t.Errorf("finding %+v", got[0])
	}

	r = repo(t, map[string]string{"std/changes/http.md": "# http\n\n- Go reference: GO-2026-0003, GO-2026-6613\n"})
	if got, err = run(src, r, "1.26.0"); err != nil || len(got) != 0 {
		t.Fatalf("with GO-2026-6613 in http.md: %q, %v", ids(got), err)
	}

	// a review naming the ID counts as the module's answer too
	r = repo(t, map[string]string{
		"std/changes/http.md":         "# http\n\n- Go reference: GO-2026-0003\n",
		"std/evidence/http/review.md": "| GO-2026-6613 | not affected |\n",
	})
	if got, err = run(src, r, "1.26.0"); err != nil || len(got) != 0 {
		t.Fatalf("with GO-2026-6613 in review.md: %q, %v", ids(got), err)
	}

	// an older port asks about GO-2025-0001 too
	r = repo(t, map[string]string{"std/changes/http.md": "GO-2026-0003 GO-2026-6613"})
	if got, err = run(src, r, "1.25.0"); err != nil || !reflect.DeepEqual(ids(got), []string{"GO-2025-0001 net/http"}) {
		t.Fatalf("since 1.25.0: %q, %v", ids(got), err)
	}
}

func TestMapping(t *testing.T) {
	r := repo(t, map[string]string{})
	got, err := mapping(filepath.Join(r, "knowledgebase", "infrastructure", "std-coverage.md"))
	if err != nil {
		t.Fatal(err)
	}
	want := map[string][]string{
		"net/http":      {"http"},
		"net":           {"prelude", "udp", "dns"},
		"crypto/tls":    {"tls", "http"},
		"encoding/asn1": {"asn1"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("mapping %v, want %v", got, want)
	}
	for in, want := range map[string]string{"net/http/internal/http2": "net/http", "internal/syscall/unix": "", "net/url": "net/url"} {
		if got := parent(in); got != want {
			t.Errorf("parent(%q) = %q, want %q", in, got, want)
		}
	}
	if !less(version("1.26.0"), version("1.27.0-rc.2")) || less(version("1.26.3"), version("1.26.3")) {
		t.Error("version order")
	}
}
