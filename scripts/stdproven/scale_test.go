package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestParseScaleList: the list's fields, and the errors a scale program's
// author sees for a bad line.
func TestParseScaleList(t *testing.T) {
	cs, err := parseScaleList("json_deep n=1000 mem=6\n\ncsv_line n=50000 mem=3 stream\n")
	if err != nil {
		t.Fatal(err)
	}
	if len(cs) != 2 || cs[0] != (scaleCase{"json_deep", 1000, 6, false}) || cs[1] != (scaleCase{"csv_line", 50000, 3, true}) {
		t.Fatalf("got %+v", cs)
	}
	for _, c := range []struct{ text, want string }{
		{"", "names no case"},
		{"a n=0 mem=2", "not a positive count"},
		{"a n=5", "needs n=N and mem=X"},
		{"a n=5 mem=2 fast", "unknown field"},
	} {
		if _, err := parseScaleList(c.text); err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%q: %v, want %q", c.text, err, c.want)
		}
	}
}

// TestScaleJudge: each bar fails alone at its edge.
func TestScaleJudge(t *testing.T) {
	c := scaleCase{name: "x", n: 1000, mem: 4, stream: true}
	at1 := scaleRun{input: 1000, workNs: 1000, peak: 10 << 20}
	for _, k := range []struct {
		name  string
		at8   scaleRun
		bytes scaleRun
		want  string
	}{
		{"pass", scaleRun{8000, 12000, 10<<20 + 28000 + scaleMemSlack}, scaleRun{workNs: 4000}, ""},
		{"time", scaleRun{8000, 12001, 10 << 20}, scaleRun{workNs: 1000}, "time grew 12.0x"},
		{"memory", scaleRun{8000, 9000, 10<<20 + 28001 + scaleMemSlack}, scaleRun{workNs: 1000}, "peak memory grew"},
		{"stream", scaleRun{8000, 9000, 10 << 20}, scaleRun{workNs: 4001}, "1-byte writes took 4.0x"},
	} {
		by := k.bytes
		line, fails := scaleJudge(c, at1, k.at8, &by)
		if k.want == "" {
			if len(fails) != 0 || !strings.HasSuffix(line, ", pass") {
				t.Errorf("%s: %q %v", k.name, line, fails)
			}
			continue
		}
		if len(fails) != 1 || !strings.HasPrefix(fails[0], k.want) || !strings.Contains(line, "FAIL") {
			t.Errorf("%s: %q %v, want one failure %q", k.name, line, fails, k.want)
		}
	}
}

// TestScalePlantedQuadraticFails (task #351): the kind catches a planted
// quadratic loop and passes the same program's linear cases, on the native
// backend with the checkout's xo.
func TestScalePlantedQuadraticFails(t *testing.T) {
	if testing.Short() {
		t.Skip("builds and runs a program")
	}
	root, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	xo := filepath.Join(root, "bin", "xo")
	if _, err := os.Stat(xo); err != nil {
		t.Skip("no bin/xo (go run ./scripts/xobuild)")
	}
	lines, ok, err := scaleCheck(xo, filepath.Join("scripts", "stdproven", "testdata", "scaleplant"), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if ok || len(lines) != 3 {
		t.Fatalf("ok %v, lines:\n%s", ok, strings.Join(lines, "\n"))
	}
	for _, l := range lines {
		if strings.HasPrefix(l, "- quadratic:") {
			if !strings.Contains(l, "FAIL (time grew") {
				t.Errorf("the planted quadratic loop passed: %s", l)
			}
		} else if !strings.HasSuffix(l, ", pass") {
			t.Errorf("a linear case failed: %s", l)
		}
	}
}
