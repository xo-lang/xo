package main

import (
	"encoding/json"
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
