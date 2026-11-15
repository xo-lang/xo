package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/xo-lang/xo/internal/mod/modtest"
	"github.com/xo-lang/xo/internal/xotool"
)

// vulnReport is `xo vuln --json`'s output (decision 0647).
type vulnReport struct {
	Database  string `json:"database"`
	Toolchain string `json:"toolchain"`
	Findings  []struct {
		Code     string   `json:"code"`
		ID       string   `json:"id"`
		Module   string   `json:"module"`
		Imports  []string `json:"imports"`
		Version  string   `json:"version"`
		Affected string   `json:"affected"`
		Fix      string   `json:"fix"`
		Backends []string `json:"backends"`
	} `json:"findings"`
}

// vulnDB writes a database in vuln.xolang.org's layout (index/modules.json
// and ID/*.json, OSV 1.6) under dir: each entry is (id, module, the
// SEMVER events as introduced/fixed/last_affected pairs, std imports,
// backends).
func vulnDB(t *testing.T, dir string, entries []vulnEntry) {
	t.Helper()
	mods := map[string][]map[string]string{}
	for _, e := range entries {
		ev := []map[string]string{{"introduced": e.introduced}}
		if e.fixed != "" {
			ev = append(ev, map[string]string{"fixed": e.fixed})
		} else {
			ev = append(ev, map[string]string{"last_affected": e.last})
		}
		aff := map[string]any{
			"package": map[string]string{"ecosystem": "Xo", "name": e.module},
			"ranges":  []any{map[string]any{"type": "SEMVER", "events": ev}},
		}
		if len(e.imports) > 0 {
			var imps []any
			for _, p := range e.imports {
				imps = append(imps, map[string]any{"path": p, "symbols": []string{}})
			}
			aff["ecosystem_specific"] = map[string]any{"imports": imps}
		}
		if len(e.backends) > 0 {
			aff["database_specific"] = map[string]any{"backends": e.backends}
		}
		osv := map[string]any{"schema_version": "1.6.0", "id": e.id, "modified": "2026-11-15T00:00:00Z", "published": "2026-11-15T00:00:00Z", "summary": e.id + " summary", "details": "d", "affected": []any{aff}}
		b, _ := json.Marshal(osv)
		modtest.WriteFiles(t, dir, map[string]string{"ID/" + e.id + ".json": string(b)})
		mods[e.module] = append(mods[e.module], map[string]string{"id": e.id, "modified": "2026-11-15T00:00:00Z"})
	}
	var paths []string
	for p := range mods {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	var idx []any
	for _, p := range paths {
		idx = append(idx, map[string]any{"path": p, "vulns": mods[p]})
	}
	b, _ := json.Marshal(idx)
	modtest.WriteFiles(t, dir, map[string]string{"index/modules.json": string(b)})
}

type vulnEntry struct {
	id, module, introduced, fixed, last string
	imports, backends                   []string
}

// TestVuln checks `xo vuln` stage A (decision 0647) against a file://
// database: a std entry is reported only when the program imports a std
// module it names and the toolchain's version is in its range; a
// toolchain entry for every program; a required module's entry by its
// selected version (XO1322); exit 3 with findings, 0 without, 1 for a
// database that cannot be read (XO1323), 0 for --db=off.
func TestVuln(t *testing.T) {
	t.Parallel()
	root, err := xotool.Root()
	if err != nil {
		t.Fatal(err)
	}
	vb, err := os.ReadFile(filepath.Join(root, "std", "VERSION"))
	if err != nil {
		t.Fatal(err)
	}
	tv := strings.TrimSpace(string(vb))
	repoWorld(t, map[string]string{
		"github.com/acme/kv@v1.0.0/xo.mod": "module github.com/acme/kv\nedition preview\n",
		"github.com/acme/kv@v1.0.0/kv.xo":  "pub fn get(k: Str) -> Str { k }\n",
	})
	app := filepath.Join(t.TempDir(), "app")
	modtest.WriteFiles(t, app, map[string]string{
		"xo.mod": "module example.com/app\nedition preview\n\nrequire github.com/acme/kv v1.0.0\n",
		"main.xo": `use std/csv
use github.com/acme/kv

fn main() uses stdio {
  let rows = csv.parse(kv.get("a,b\n")).ok() ?? []
  println("${rows.len()}")
}
`,
	})
	xo(t, "mod", "tidy", "--dir", app).want(0)
	db := filepath.Join(t.TempDir(), "db")
	vulnDB(t, db, []vulnEntry{
		{id: "XO-2026-0001", module: "std", introduced: "0", fixed: "999.0.0", imports: []string{"std/csv"}},
		{id: "XO-2026-0002", module: "std", introduced: "0", fixed: "999.0.0", imports: []string{"std/png"}},
		{id: "XO-2026-0003", module: "std", introduced: "0", fixed: "0.1.0", imports: []string{"std/csv"}},
		{id: "XO-2026-0004", module: "toolchain", introduced: "0", last: tv, backends: []string{"native"}},
		{id: "XO-2026-0005", module: "github.com/acme/kv", introduced: "0.9.0", fixed: "1.0.1"},
		{id: "XO-2026-0006", module: "github.com/acme/kv", introduced: "0", fixed: "1.0.0"},
		{id: "XO-2026-0007", module: "github.com/acme/other", introduced: "0", fixed: "9.0.0"},
	})
	r := xo(t, "vuln", "--json", "--db=file://"+db, app).want(3)
	var rep vulnReport
	if err := json.Unmarshal([]byte(r.out), &rep); err != nil {
		t.Fatalf("bad JSON: %v\n%s", err, r.out)
	}
	if rep.Toolchain != tv {
		t.Errorf("toolchain %q, want %q", rep.Toolchain, tv)
	}
	var got []string
	for _, f := range rep.Findings {
		got = append(got, fmt.Sprintf("%s %s %s %s [%s]", f.Code, f.ID, f.Module, f.Version, strings.Join(f.Imports, ",")))
	}
	want := []string{
		"XO1321 XO-2026-0001 std " + tv + " [std/csv]",
		"XO1321 XO-2026-0004 toolchain " + tv + " []",
		"XO1322 XO-2026-0005 github.com/acme/kv v1.0.0 []",
	}
	sort.Strings(got)
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("findings:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	for _, f := range rep.Findings {
		switch f.ID {
		case "XO-2026-0001":
			if f.Fix != "upgrade xo to 999.0.0 or later" {
				t.Errorf("std fix %q", f.Fix)
			}
		case "XO-2026-0004":
			if !strings.HasPrefix(f.Fix, "no release has the fix yet") || strings.Join(f.Backends, ",") != "native" {
				t.Errorf("toolchain finding %+v", f)
			}
		case "XO-2026-0005":
			if f.Affected != "0.9.0 before 1.0.1" || f.Fix != "upgrade github.com/acme/kv to 1.0.1 or later" {
				t.Errorf("module finding %+v", f)
			}
		}
	}

	// nothing found: exit 0
	clean := filepath.Join(t.TempDir(), "db")
	vulnDB(t, clean, []vulnEntry{{id: "XO-2026-0001", module: "std", introduced: "0", fixed: "999.0.0", imports: []string{"std/png"}}})
	r = xo(t, "vuln", "--json", "--db=file://"+clean, app).want(0)
	if !strings.Contains(r.out, `"findings": []`) {
		t.Errorf("clean report: %s", r.out)
	}
	// a database that cannot be read: exit 1, XO1323
	r = xo(t, "vuln", "--db=file://"+filepath.Join(t.TempDir(), "none"), app).want(1)
	if !strings.Contains(r.err, "XO1323") {
		t.Errorf("missing database: %s", r.err)
	}
	// XO_VULNDB=off and --db=off check nothing
	xo(t, "vuln", "--db=off", app).want(0)
	// a db that is not https, file:// or off is a usage error
	xo(t, "vuln", "--db=ftp://x", app).want(2)
}
