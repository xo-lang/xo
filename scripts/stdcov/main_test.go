package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func day(s string) time.Time {
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		panic(err)
	}
	return t
}

// TestFresh: decision 0616 section 3, "for 90 days and one minor release
// after the run, whichever is later".
func TestFresh(t *testing.T) {
	minors, minorsRead = []time.Time{day("2026-08-01"), day("2026-11-10"), day("2027-03-01")}, true
	today = day("2026-12-31")
	for _, c := range []struct {
		name, source, date string
		want               bool
	}{
		{"hash matches, old", "h", "2025-01-01", true},
		{"stale hash, 30 days", "x", "2026-12-01", true},
		{"stale hash, 90 days exactly", "x", "2026-10-02", true},
		{"stale hash, 120 days, a minor release after it", "x", "2026-09-01", false},
		{"stale hash, 120 days, no minor release after it yet", "x", "2026-09-01", true},
		{"stale hash, no date", "x", "", false},
	} {
		if c.name == "stale hash, 120 days, no minor release after it yet" {
			minors = []time.Time{day("2026-08-01"), day("2027-03-01")} // the next one is after today
		}
		hdr := map[string]string{"source": c.source}
		if c.date != "" {
			hdr["date"] = c.date
		}
		if got := fresh(hdr, "h"); got != c.want {
			t.Errorf("%s: fresh = %v, want %v", c.name, got, c.want)
		}
	}
}

func TestDecimals(t *testing.T) {
	for in, want := range map[string][]float64{
		"decode 1.04, encode 0.94 of Go's wall": {1.04, 0.94},
		"v0.35.0 and 2026-11-02 and 1/2.5":      {0.35},
		"x1.25y -0.5 12.75":                     {1.25, 12.75},
		"1.2.3":                                 {1.2},
	} {
		// the answers are Python's re.findall(r"(?<![\d./-])\d+\.\d+(?![\d])") on each
		if got := decimals(in); !reflect.DeepEqual(got, want) {
			t.Errorf("decimals(%q) = %v, want %v", in, got, want)
		}
	}
}

// TestFeatAuditGaps: decision 0655, a module whose rows are all done is
// Proven only when its Go packages have no open stdaudit gap and no row
// outside Go's standard library lacks its audit.
func TestFeatAuditGaps(t *testing.T) {
	rowsByMod = map[string][]string{"utf": {"done"}, "clean": {"done", "done"}, "part": {"done", "partial"}, "cbor": {"unaudited"}, "pg": {"done"}}
	pkgsByMod = map[string][]string{"utf": {"utf8"}, "clean": {"base64", "utf16"}, "part": {"csv"}, "pg": {"pgtype"}}
	gapsByPkg = map[string][]string{"utf8": {"utf8.FullRune", "utf8.DecodeLastRune"}, "csv": {"csv.Reader.FieldPos"}}
	// rule 8: categorized not planned rows are resolved and counted
	npByPkg = map[string][]string{"pgtype": {"pgtype.Point", "pgtype.Box"}}
	for _, c := range []struct{ mod, level, text string }{
		{"utf", "Solid", "all 1 mapped Go rows done, 2 open stdaudit gaps (utf8.DecodeLastRune, utf8.FullRune)"},
		{"clean", "Proven", "all 2 mapped Go rows done, no open stdaudit gap"},
		{"part", "Solid", "1 of 2 mapped Go rows done, 1 partial, 1 open stdaudit gaps"},
		{"cbor", "Solid", "all 1 mapped rows done, 1 outside Go's standard library without a symbol audit yet (decision 0655)"},
		{"pg", "Proven", "all 1 mapped Go rows done, no open stdaudit gap (2 not planned by category, decision 0655 rule 8)"},
	} {
		if f := feat(c.mod); f.level != c.level || f.text != c.text {
			t.Errorf("feat(%s) = %s %q, want %s %q", c.mod, f.level, f.text, c.level, c.text)
		}
	}
}

// TestNotPlannedCategory: decision 0655 rule 8's `not planned: CATEGORY:
// reason`: a known category is resolved, a design choice must name its
// decision, an older reason has no category.
func TestNotPlannedCategory(t *testing.T) {
	for _, c := range []struct {
		reason, cat string
		bad         bool
	}{
		{"platform-specific: GSSAPI needs a system library", "platform-specific", false},
		{"design choice: decision 0278: text format results", "design choice", false},
		{"design choice: text format results", "", true},
		{"plug-in extension points: Go's codec interface", "plug-in extension points", false},
		{"the scheduler (decision 0474)", "", false},
		{"see x: y", "", false},
	} {
		cat, err := npCategory(c.reason)
		if cat != c.cat || (err != nil) != c.bad {
			t.Errorf("npCategory(%q) = %q, %v; want %q, bad %v", c.reason, cat, err, c.cat, c.bad)
		}
	}
}

// TestPerfHelpers: an internal helper takes the lowest perf mark among
// its users (lead, 2026-11-15), and says whose.
func TestPerfHelpers(t *testing.T) {
	for h, users := range perfHelpers {
		got := perf(h)
		low := 2
		for _, u := range users {
			if l := lv[perf(u).level]; l < low {
				low = l
			}
		}
		if got.level != nm[low] {
			t.Errorf("perf(%q) = %s, want %s (the lowest of %v)", h, got.level, nm[low], users)
		}
		if !strings.Contains(got.text, users[0]) {
			t.Errorf("perf(%q) text %q does not name its users", h, got.text)
		}
	}
}

// TestMarksJSON: the -json snapshot's shape, which xw std (workstream
// std.xo) parses.
func TestMarksJSON(t *testing.T) {
	data, err := marksJSON("abc", "2026-11-15T18:00:00-05:00", []modMarks{{Module: "std/json", Features: "Proven", Testing: "Solid", Performance: "Early", Overall: "Early"}})
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	want := map[string]any{"sha": "abc", "at": "2026-11-15T18:00:00-05:00", "modules": []any{map[string]any{"module": "std/json", "features": "Proven", "testing": "Solid", "performance": "Early", "overall": "Early"}}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

// TestMissing: what a module lacks for Proven on each axis below it (xw
// #93); a Proven axis has no entry.
func TestMissing(t *testing.T) {
	rowsByMod = map[string][]string{"zzpart": {"done", "partial", "unaudited"}, "zzdone": {"done"}}
	pkgsByMod = map[string][]string{"zzpart": {"zzcsv"}}
	gapsByPkg = map[string][]string{"zzcsv": {"csv.Reader.FieldPos", "csv.Writer.UseCRLF"}}
	got := missing("zzpart", mark{level: "Solid"}, mark{level: "Early"}, mark{level: "Early", text: "x"})
	want := map[string][]string{
		"features":    {"1 partial Go rows", "1 rows without a symbol audit (decision 0655)", "2 open map.txt gaps: csv.Reader.FieldPos, csv.Writer.UseCRLF"},
		"testing":     {"race evidence", "stress evidence", "fuzz evidence"},
		"performance": {"no committed row against Go: a Go twin and a timing row"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("missing(zzpart) = %v, want %v", got, want)
	}
	if m := missing("zzdone", mark{level: "Proven"}, mark{level: "Proven"}, mark{level: "Proven"}); m != nil {
		t.Fatalf("missing of an all-Proven module = %v, want nil", m)
	}
}

// TestFeatAudit: decision 0691: a module no Go package maps to is Proven
// when std/evidence/MODULE/features.md lists every feature with a test that
// exists and none open; Solid otherwise, naming what is open.
func TestFeatAudit(t *testing.T) {
	old, oldBG := repo, beyondGo
	defer func() { repo, beyondGo = old, oldBG }()
	repo = t.TempDir()
	rowsByMod = map[string][]string{}
	beyondGo = map[string]string{"ws": "Go has no websocket in std"}
	write := func(p, s string) {
		p = filepath.Join(repo, p)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(s), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("test/golden/ui_live/main.xo", "")
	write("std/ui_test.xo", "")
	write("std/evidence/ui/features.md", "# std/ui features\n\n- [x] live pages | decision 0689 | test/golden/ui_live\n- [x] slots | decision 0670 | `std/ui_test.xo:test_slot`\n")
	write("std/evidence/half/features.md", "- [x] a | decision 1 | std/ui_test.xo\n- [ ] b | decision 2 | streaming not done\n- [x] c | decision 3 |\n- [x] d | decision 4 | std/gone_test.xo\n")
	write("std/evidence/empty/features.md", "# nothing yet\n")
	for _, c := range []struct{ mod, level, text string }{
		{"ui", "Proven", "feature audit: all 2 features tested, none open (decision 0691)"},
		{"half", "Solid", "feature audit: 1 of 4 features tested, 1 open (b), 2 done without a test (c (no test), d (no file std/gone_test.xo))"},
		{"empty", "Solid", "feature audit lists no feature (decision 0691)"},
		{"ws", "Solid", "beyond Go: Go has no websocket in std; no feature audit yet (decision 0691)"},
		{"none", "Solid", "no Go package maps to it; no feature audit yet (decision 0691)"},
	} {
		if f := feat(c.mod); f.level != c.level || f.text != c.text {
			t.Errorf("feat(%s) = %s %q, want %s %q", c.mod, f.level, f.text, c.level, c.text)
		}
	}
}

// TestPerfEvidenceHosts: perf evidence counts from the hosts ADR 0712 (as
// amended, 9985b92af; task #301) times it on: the amd64 host and host 2. A
// host 1 row (or an unlabelled linux/arm64 one, as json's and time's were)
// dated on or before 2026-11-17 that meets the bar stays; a newer host 1 row,
// and any row from hosts 3 or 4 (containers on the Mac Studio) or the Mac,
// does not count. serverperf's measure line ("worst Xo / Go cost N") counts
// as perf's does ("worst Xo / Go N").
func TestPerfEvidenceHosts(t *testing.T) {
	const pm = "4 cases of 1 twin directories, worst Xo / Go 1.020 over the cases above 5 ms (Proven at or under 1.05), 3m0s"
	// commit times: a host 1 row counts when committed at or before ADR
	// 0712's commit (task #304), whatever its UTC date line says
	const old, at01, at03, none = 1794571200, 1794963600, 1794970800, 0
	keep := evCommitted
	defer func() { evCommitted = keep }()
	for _, c := range []struct {
		m, host, date, measure string
		committed              int64
		want                   string
	}{
		{"zz-amd64", "linux/amd64 host", "2026-11-18", pm, at03, "Proven"},
		{"zz-amd64-uncommitted", "linux/amd64 host", "2026-11-18", pm, none, "Proven"},
		{"zz-host2", "linux/arm64 host 2", "2026-11-18", pm, at03, "Proven"},
		{"zz-host1-old", "linux/arm64 host 1", "2026-11-13", pm, old, "Proven"},
		{"zz-bare-old", "linux/arm64", "2026-11-14", pm, old, "Proven"},
		{"zz-host1-0100z", "linux/arm64 host 1", "2026-11-18", pm, at01, "Proven"},
		{"zz-bare-0100z", "linux/arm64", "2026-11-18", pm, at01, "Proven"},
		{"zz-host1-0300z", "linux/arm64 host 1", "2026-11-18", pm, at03, "Early"},
		{"zz-host1-uncommitted", "linux/arm64 host 1", "2026-11-17", pm, none, "Early"},
		{"zz-bare-0300z", "linux/arm64", "2026-11-18", pm, at03, "Early"},
		{"zz-host3", "linux/arm64 host 3", "2026-11-18", pm, at03, "Early"},
		{"zz-host4", "linux/arm64 host 4", "2026-11-18", pm, at03, "Early"},
		{"zz-host4-old", "linux/arm64 host 4", "2026-11-13", pm, old, "Early"},
		{"zz-mac", "darwin/arm64", "2026-11-18", pm, at03, "Early"},
		{"zz-serverperf-amd64", "linux/amd64 host", "2026-11-18", "6 workload and backend figures, worst Xo / Go cost 1.040 (the worse of throughput and CPU or wall per figure; Proven at or under 1.05), 9m0s", at03, "Proven"},
		{"zz-serverperf-slow", "linux/amd64 host", "2026-11-18", "6 workload and backend figures, worst Xo / Go cost 1.200 (the worse of throughput and CPU or wall per figure; Proven at or under 1.05), 9m0s", at03, "Solid"},
		{"zz-amd64-slow", "linux/amd64 host", "2026-11-19", "4 cases of 1 twin directories, worst Xo / Go 1.409 over the cases above 5 ms (Proven at or under 1.05), 3m0s", at03, "Solid"},
		{"zz-host2-veryslow", "linux/arm64 host 2", "2026-11-19", "4 cases of 1 twin directories, worst Xo / Go 1.591 over the cases above 5 ms (Proven at or under 1.05), 3m0s", at03, "Early"},
	} {
		path := "std/evidence/" + c.m + "/perf.md"
		evCommitted = func(p string) int64 {
			if p != path {
				t.Fatalf("%s: commit time asked for %s, want %s", c.m, p, path)
			}
			return c.committed
		}
		evCache[c.m] = map[string]evFile{"perf": {hdr: map[string]string{"kind": "perf", "result": "pass", "host": c.host, "date": c.date, "measure": c.measure}, path: path}}
		if got := perf(c.m); got.level != c.want {
			t.Errorf("%s (%s, %s): perf = %s %q, want %s", c.m, c.host, c.date, got.level, got.text, c.want)
		}
		if c.want == "Early" && c.host != "linux/amd64 host" && c.host != "linux/arm64 host 2" {
			if xs := missing(c.m, mark{level: "Proven"}, mark{level: "Proven"}, perf(c.m))["performance"]; len(xs) == 0 || !strings.Contains(xs[0], "ADR 0712") {
				t.Errorf("%s: missing performance = %q, want the host named against ADR 0712", c.m, xs)
			}
		}
		delete(evCache, c.m)
	}
}

// TestGotestsEvidence: decision 0720's gotests kind. A module with a
// gotests.md needs it to pass (a failed run is a mismatch with Go) for a
// Proven testing mark, as a review.md is needed once it exists; one
// without needs none while gotestsRequired is off; a counted run's cases
// show in the mark, and lift a module with no other Go differential from
// Early to Solid.
func TestGotestsEvidence(t *testing.T) {
	old, oldCache, oldCorpus, oldDiff, oldParsers := repo, evCache, corpus, godiff, parsers
	defer func() { repo, evCache, corpus, godiff, parsers = old, oldCache, oldCorpus, oldDiff, oldParsers }()
	repo = t.TempDir()
	corpus, godiff, parsers = map[string]bool{}, map[string]evText{}, map[string]bool{}
	for _, m := range []string{"zzg", "zzf", "zze"} {
		p := filepath.Join(repo, "std/evidence", m)
		if err := os.MkdirAll(p, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(p, "gotests.md"), []byte("kind: gotests\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	base := func() map[string]evFile {
		return map[string]evFile{
			"race":   {map[string]string{}, ""},
			"stress": {map[string]string{}, ""},
			"fuzz":   {map[string]string{"measure": "fz"}, ""},
		}
	}
	gt := evFile{map[string]string{"measure": "307 cases from 10 tables of Go's tests (go1.26.11), 2 backends (arm64,llvm), 0 mismatches with Go"}, ""}
	zzg := base()
	zzg["gotests"] = gt
	evCache = map[string]map[string]evFile{"zzg": zzg, "zzf": base(), "zzn": base(), "zze": {"gotests": gt}}
	for _, c := range []struct{ mod, level, text string }{
		{"zzg", "Proven", "Proven evidence (race, stress, fuzz, gotests; decision 0616): fz; Go test tables: 307 cases from 10 tables"},
		{"zzn", "Proven", "Proven evidence (race, stress, fuzz; decision 0616): fz"},
		{"zze", "Solid", "Go test tables: 307 cases from 10 tables"},
	} {
		if g := test(c.mod); g.level != c.level || g.text != c.text {
			t.Errorf("test(%s) = %s %q, want %s %q", c.mod, g.level, g.text, c.level, c.text)
		}
	}
	if g := test("zzf"); g.level == "Proven" {
		t.Errorf("test(zzf) = Proven with a gotests.md that does not count")
	}
	if got := missing("zzf", mark{level: "Proven"}, mark{level: "Solid"}, mark{level: "Proven"})["testing"]; !reflect.DeepEqual(got, []string{"gotests evidence"}) {
		t.Errorf("missing(zzf) testing = %v, want [gotests evidence]", got)
	}
}
