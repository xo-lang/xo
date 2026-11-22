package stdtest

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/xo-lang/xo/internal/testtmp"
	"github.com/xo-lang/xo/internal/xotool"
)

// TestStressRssMedians: the stress programs' memory rule (decision 0616 as
// amended 2026-11-17, owner): every std/evidence/*/stress/main.xo holds the
// same rss_medians, and it compares the median of minutes 15 to 17 (decision
// 0717, owner 2026-11-18; minutes 5 to 7 before) with the median of the
// last 3 minutes. Every program also
// takes its mark at minute 15 of a 30 minute run. amqp's llvm run (2026-11-17, host 2) failed
// the old rule on one dip at the mark (211 MB, then a 217 to 230 MB
// plateau, 12% from the dip, about 6% from the plateau); a single dip now
// passes, and steady growth still fails.
// stressMark0717: the mark every stress program takes (decision 0717).
const stressMark0717 = "  let mark_s = if minutes >= 30 { 900 } else { minutes * 30 }   // decision 0717: minute 15 of a 30 minute run\n  let mark = start + Duration.from_secs(Float(mark_s))\n"

// stressMarkHalfway: the mark of a program that also runs task #330's
// level-off guard (60 minutes, 5%): half way through any run, which is
// still minute 15 of a 30 minute run.
const stressMarkHalfway = "  let mark_s = minutes * 30\n  let bound = if minutes > 30 { 5.0 } else { 10.0 }\n  let mark = start + Duration.from_secs(Float(mark_s))\n"

// stressHalfway: the programs that take stressMarkHalfway's mark.
var stressHalfway = map[string]bool{"grpcreflect": true}

func TestStressRssMedians(t *testing.T) {
	root, err := xotool.Root()
	if err != nil {
		t.Fatal(err)
	}
	progs, _ := filepath.Glob(filepath.Join(root, "std", "evidence", "*", "stress", "main.xo"))
	if len(progs) < 80 {
		t.Fatalf("found %d stress programs", len(progs))
	}
	helper := ""
	for _, p := range progs {
		b, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		s := string(b)
		i := strings.Index(s, "\n/// The resident set early and late")
		if i < 0 || !strings.Contains(s, "let (at_mark, last) = rss_medians(") {
			t.Errorf("%s: no rss_medians (decision 0616's median rule)", p)
			continue
		}
		if mod := filepath.Base(filepath.Dir(filepath.Dir(p))); stressHalfway[mod] {
			if !strings.Contains(s, stressMarkHalfway) {
				t.Errorf("%s: its mark is not the level-off guard's half way mark (task #330)", p)
			}
		} else if !strings.Contains(s, stressMark0717) {
			t.Errorf("%s: its mark is not decision 0717's (minute 15 of a 30 minute run)", p)
		}
		h := s[i:]
		if helper == "" {
			helper = h
		} else if h != helper {
			t.Errorf("%s: its rss_medians differs from the others", p)
		}
	}
	if helper == "" {
		t.Fatal("no helper")
	}
	if testing.Short() {
		return
	}
	xo, err := xotool.Path()
	if err != nil {
		t.Fatal(err)
	}
	work, err := testtmp.Dir("xo-stressrss-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(work)
	// 25 minutes after the mark, a sample every 10 s, in KiB
	dip := make([]string, 150)
	grow := make([]string, 150)
	for i := range dip {
		v := 217 + 13*i/150 // a plateau creeping 217 to 230 MB
		if i == 0 {
			v = 211 // the one dip at the mark
		}
		if i == len(dip)-1 {
			v = 237 // and a spike at the end
		}
		dip[i] = strconv.Itoa(v * 1024)
		grow[i] = strconv.Itoa((200 + 40*i/150) * 1024) // 200 to 240 MB
	}
	prog := "fn main(os: Os) {\n  for xs in [[" + strings.Join(dip, ", ") + "], [" + strings.Join(grow, ", ") + "]] {\n    let (a, b) = rss_medians(xs)\n    os.stdio.println(\"${a} ${b}\")\n  }\n}\n" + helper
	dir := filepath.Join(work, "p")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "main.xo"), []byte(prog), 0o644); err != nil {
		t.Fatal(err)
	}
	cmd := testtmp.Guard(exec.Command(xo, "run", dir))
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("xo run: %v\n%s", err, out)
	}
	lines := strings.Fields(string(out))
	if len(lines) != 4 {
		t.Fatalf("output: %q", out)
	}
	grew := func(a, b string) bool { // the programs' rule: under 10% or 4 MB passes
		x, _ := strconv.Atoi(a)
		y, _ := strconv.Atoi(b)
		return !(float64(y-x)*100/float64(x) < 10 || y-x < 4096)
	}
	if !grew(dip[0], dip[len(dip)-1]) {
		t.Errorf("the old rule (one sample at the mark, the last at the end) should fail the dip series")
	}
	if grew(lines[0], lines[1]) {
		t.Errorf("one dip at the mark failed the median rule: early %s, late %s KiB", lines[0], lines[1])
	}
	if !grew(lines[2], lines[3]) {
		t.Errorf("steady growth passed the median rule: early %s, late %s KiB", lines[2], lines[3])
	}
}
