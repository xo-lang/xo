// Command scorecard scores a compiler change across the board (decision
// 0727): two xo builds, A (the base) and B (the change), compared on
//
//  1. every std bench twin (scripts/twincheck -time N -xo-b): each
//     row's Xo/Go ratio for A and for B, read as decision 0726 says (the
//     median of per-round ratios over at least N interleaved rounds);
//
//  2. the codegen corpus (test/codegen): each program's own functions'
//     instruction count in `xo asm`, A and B, and which functions changed;
//
//  3. compile time: `xo build --phase-times` of compiler/xo with the
//     caches off, A and B alternating, wall and phase medians.
//
//     XO_CLI=A/bin/xo XO_STD=A/std go run ./scripts/scorecard -b B/bin/xo -std-b B/std [-rounds 30] [twin dirs...]
//
// It builds and times programs: an agent runs it on host 2 through
// scripts/lead/ab-host.sh's scorecard mode (OFFLOAD_ENV=offload-arm64b.env
// ab-host.sh BASE NEW NAME -- scorecard [DIR...]), which builds both xos
// at their revisions, or locally through heavy.sh.
package main

import (
	"bufio"
	"bytes"
	"flag"
	"fmt"
	"io"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/xo-lang/xo/internal/xotool"
)

func main() {
	xoB := flag.String("b", "", "B's xo (the change); A is $XO_CLI, else the checkout's bin/xo")
	stdB := flag.String("std-b", "", "B's std directory (XO_STD for B); A's is $XO_STD")
	rounds := flag.Int("rounds", 30, "the least interleaved rounds per twin row (decision 0726)")
	corpus := flag.String("corpus", "test/codegen", "the codegen corpus directory (\"\" skips it)")
	self := flag.String("self", "compiler/xo", "the program whose build is timed (\"\" skips it)")
	selfRounds := flag.Int("self-rounds", 3, "builds of -self per side")
	twins := flag.Bool("twins", true, "time the bench twins")
	corpusRun := flag.String("corpus-run", "", "only corpus programs whose directory name matches this regexp")
	flag.Parse()
	if *xoB == "" {
		fmt.Fprintln(os.Stderr, "scorecard: -b B_XO is required")
		os.Exit(2)
	}
	match, err := regexp.Compile(*corpusRun)
	if err != nil {
		fmt.Fprintln(os.Stderr, "scorecard: -corpus-run:", err)
		os.Exit(2)
	}
	if err := run(os.Stdout, *xoB, *stdB, *rounds, *corpus, match, *self, *selfRounds, *twins, flag.Args()); err != nil {
		fmt.Fprintln(os.Stderr, "scorecard:", err)
		os.Exit(1)
	}
}

func run(w io.Writer, xoB, stdB string, rounds int, corpus string, match *regexp.Regexp, self string, selfRounds int, twins bool, dirs []string) error {
	root, err := xotool.Root()
	if err != nil {
		return err
	}
	// A is another revision's xo on purpose (built with its own std,
	// $XO_STD), run from B's tree, which has this tool: a deliberate
	// cross version run, for this process and the twincheck it starts.
	os.Setenv("XO_CLI_STALE_OK", "1")
	xoA, err := xotool.Command()
	if err != nil {
		return err
	}
	a := side{name: "A", xo: xoA, std: os.Getenv("XO_STD")}
	b := side{name: "B", xo: strings.Fields(xoB), std: stdB}
	fmt.Fprintf(w, "scorecard: A %s (std %s), B %s (std %s), %s\n", strings.Join(a.xo, " "), orEnv(a.std), xoB, orEnv(b.std), time.Now().Format("2006-01-02 15:04"))
	failed := 0
	if twins {
		rows, bad, err := twinRows(root, b, rounds, dirs)
		if err != nil {
			return err
		}
		failed += len(bad)
		writeTwins(w, rows, bad)
	}
	if corpus != "" {
		rows, err := corpusRows(filepath.Join(root, corpus), match, a, b)
		if err != nil {
			return err
		}
		writeCorpus(w, corpus, rows)
	}
	if self != "" {
		st, err := selfTimes(filepath.Join(root, self), a, b, selfRounds)
		if err != nil {
			return err
		}
		writeSelf(w, self, st)
	}
	if failed > 0 {
		return fmt.Errorf("%d twin rows failed (above)", failed)
	}
	return nil
}

func orEnv(s string) string {
	if s == "" {
		return "the environment's"
	}
	return s
}

type side struct {
	name string
	xo   []string
	std  string
}

func (s side) cmd(args ...string) *exec.Cmd {
	c := exec.Command(s.xo[0], append(append([]string{}, s.xo[1:]...), args...)...)
	c.Env = os.Environ()
	if s.std != "" {
		c.Env = append(c.Env, "XO_STD="+s.std)
	}
	return c
}

// twinRow is one `ab` line of twincheck -xo-b.
type twinRow struct {
	name           string
	goMs, aMs, bMs float64
	ra, rb         float64
	rounds         int
}

// parseAB reads twincheck's `ab NAME go MS a MS b MS ra R rb R rounds N`
// lines; other lines that name a failure go to bad.
func parseAB(out string) (rows []twinRow, bad []string) {
	for _, l := range strings.Split(out, "\n") {
		f := strings.Fields(l)
		if len(f) == 14 && f[0] == "ab" && f[2] == "go" && f[4] == "a" && f[6] == "b" && f[8] == "ra" && f[10] == "rb" && f[12] == "rounds" {
			var r twinRow
			r.name = f[1]
			nums := []*float64{&r.goMs, &r.aMs, &r.bMs, &r.ra, &r.rb}
			ok := true
			for i, p := range nums {
				v, err := strconv.ParseFloat(f[3+2*i], 64)
				if err != nil {
					ok = false
				}
				*p = v
			}
			n, err := strconv.Atoi(f[13])
			if !ok || err != nil {
				bad = append(bad, l)
				continue
			}
			r.rounds = n
			rows = append(rows, r)
			continue
		}
		if strings.Contains(l, ": FAIL ") || strings.Contains(l, ": MISMATCH ") || strings.Contains(l, ": time: ") {
			bad = append(bad, l)
		}
	}
	return rows, bad
}

func twinRows(root string, b side, rounds int, dirs []string) ([]twinRow, []string, error) {
	args := []string{"run", "./scripts/twincheck", "-time", strconv.Itoa(rounds), "-xo-b", strings.Join(b.xo, " ")}
	if b.std != "" {
		args = append(args, "-std-b", b.std)
	}
	cmd := exec.Command("go", append(args, dirs...)...)
	cmd.Dir = root
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	err := cmd.Run()
	rows, bad := parseAB(out.String())
	if err != nil && len(rows) == 0 && len(bad) == 0 {
		return nil, nil, fmt.Errorf("twincheck: %v\n%s", err, out.String())
	}
	return rows, bad, nil
}

// geomean of v (1 when empty).
func geomean(v []float64) float64 {
	if len(v) == 0 {
		return 1
	}
	s := 0.0
	for _, x := range v {
		s += math.Log(x)
	}
	return math.Exp(s / float64(len(v)))
}

func writeTwins(w io.Writer, rows []twinRow, bad []string) {
	fmt.Fprintf(w, "\n## Bench twins: Xo/Go, median of per-round ratios (decision 0726)\n\n")
	fmt.Fprintf(w, "| row | rounds | Go ms | A ms | B ms | A/Go | B/Go | B/A |\n|---|---|---|---|---|---|---|---|\n")
	var ras, rbs []float64
	better, worse := 0, 0
	for _, r := range rows {
		d := r.rb / r.ra
		mark := ""
		switch {
		case d < 0.98:
			mark, better = " (faster)", better+1
		case d > 1.02:
			mark, worse = " (slower)", worse+1
		}
		fmt.Fprintf(w, "| %s | %d | %.1f | %.1f | %.1f | %.3f | %.3f | %.3f%s |\n", r.name, r.rounds, r.goMs, r.aMs, r.bMs, r.ra, r.rb, d, mark)
		ras, rbs = append(ras, r.ra), append(rbs, r.rb)
	}
	fmt.Fprintf(w, "\ntwins: %d rows, geomean A/Go %.3f, B/Go %.3f (B/A %.3f); %d rows faster and %d slower by more than 2%%\n", len(rows), geomean(ras), geomean(rbs), geomean(rbs)/geomean(ras), better, worse)
	for _, l := range bad {
		fmt.Fprintf(w, "twin failed: %s\n", l)
	}
}

// corpusRow is one corpus program: its own functions' instruction counts.
type corpusRow struct {
	name    string
	a, b    int
	changed []string // NAME A->B
	err     string
}

var funcHeader = regexp.MustCompile(`^// func (\S+)`)

// asmCounts counts the instructions of each function in `xo asm` output:
// the lines after `// func NAME` that start with a tab and are not an
// assembler directive (a tab and a dot).
func asmCounts(asm string) map[string]int {
	out := map[string]int{}
	cur := ""
	sc := bufio.NewScanner(strings.NewReader(asm))
	sc.Buffer(make([]byte, 1<<20), 1<<24)
	for sc.Scan() {
		l := sc.Text()
		if m := funcHeader.FindStringSubmatch(l); m != nil {
			cur = m[1]
			out[cur] += 0
			continue
		}
		if cur != "" && strings.HasPrefix(l, "\t") && !strings.HasPrefix(l, "\t.") {
			out[cur]++
		}
	}
	return out
}

var typeDecl = regexp.MustCompile(`(?m)^\s*(?:pub\s+)?(?:struct|enum|type|union|interface|trait)\s+([A-Za-z_][A-Za-z0-9_]*)`)

// own keeps the program's functions: the unqualified names, and methods of
// the types its files declare (std's are qualified by module).
func own(counts map[string]int, types map[string]bool) map[string]int {
	out := map[string]int{}
	for n, c := range counts {
		i := strings.IndexByte(n, '.')
		if i < 0 || types[n[:i]] {
			out[n] = c
		}
	}
	return out
}

func dirTypes(dir string) map[string]bool {
	types := map[string]bool{}
	files, _ := filepath.Glob(filepath.Join(dir, "*.xo"))
	for _, f := range files {
		src, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		for _, m := range typeDecl.FindAllStringSubmatch(string(src), -1) {
			types[m[1]] = true
		}
	}
	return types
}

func corpusRows(dir string, match *regexp.Regexp, a, b side) ([]corpusRow, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var rows []corpusRow
	for _, e := range entries {
		if !e.IsDir() || !match.MatchString(e.Name()) {
			continue
		}
		pd := filepath.Join(dir, e.Name())
		if files, _ := filepath.Glob(filepath.Join(pd, "*.xo")); len(files) == 0 {
			continue
		}
		types := dirTypes(pd)
		r := corpusRow{name: e.Name()}
		var counts [2]map[string]int
		for i, s := range []side{a, b} {
			c := s.cmd("asm", pd)
			c.Env = append(c.Env, "XO_BACKEND_CACHE=off", "XO_FE_CACHE=off")
			var out, errOut bytes.Buffer
			c.Stdout, c.Stderr = &out, &errOut
			if err := c.Run(); err != nil {
				r.err = fmt.Sprintf("%s: xo asm: %v: %s", s.name, err, lastLine(errOut.String()))
				break
			}
			counts[i] = own(asmCounts(out.String()), types)
		}
		if r.err == "" {
			var names []string
			for n := range counts[0] {
				names = append(names, n)
			}
			for n := range counts[1] {
				if _, ok := counts[0][n]; !ok {
					names = append(names, n)
				}
			}
			sort.Strings(names)
			for _, n := range names {
				ca, inA := counts[0][n]
				cb, inB := counts[1][n]
				r.a, r.b = r.a+ca, r.b+cb
				switch {
				case !inA:
					r.changed = append(r.changed, fmt.Sprintf("%s new %d", n, cb))
				case !inB:
					r.changed = append(r.changed, fmt.Sprintf("%s gone (%d)", n, ca))
				case ca != cb:
					r.changed = append(r.changed, fmt.Sprintf("%s %d->%d", n, ca, cb))
				}
			}
		}
		rows = append(rows, r)
	}
	return rows, nil
}

func lastLine(s string) string {
	l := strings.Split(strings.TrimSpace(s), "\n")
	return l[len(l)-1]
}

func writeCorpus(w io.Writer, corpus string, rows []corpusRow) {
	fmt.Fprintf(w, "\n## Codegen corpus (%s): instructions of each program's own functions, xo asm\n\n", corpus)
	fmt.Fprintf(w, "| program | A | B | B/A | changed functions |\n|---|---|---|---|---|\n")
	ta, tb, nch, nerr := 0, 0, 0, 0
	for _, r := range rows {
		if r.err != "" {
			fmt.Fprintf(w, "| %s | | | | error: %s |\n", r.name, r.err)
			nerr++
			continue
		}
		ta, tb = ta+r.a, tb+r.b
		if len(r.changed) == 0 {
			continue
		}
		nch++
		ratio := 1.0
		if r.a > 0 {
			ratio = float64(r.b) / float64(r.a)
		}
		ch := r.changed
		more := ""
		if len(ch) > 8 {
			ch, more = ch[:8], fmt.Sprintf(" and %d more", len(r.changed)-8)
		}
		fmt.Fprintf(w, "| %s | %d | %d | %.3f | %s%s |\n", r.name, r.a, r.b, ratio, strings.Join(ch, ", "), more)
	}
	ratio := 1.0
	if ta > 0 {
		ratio = float64(tb) / float64(ta)
	}
	fmt.Fprintf(w, "\ncorpus: %d programs, %d changed, %d errors; instructions A %d, B %d (B/A %.4f)\n", len(rows), nch, nerr, ta, tb, ratio)
}

// selfStat is the build times of -self per side.
type selfStat struct {
	wall   [2][]float64
	phases [2]map[string][]float64
}

var phaseLine = regexp.MustCompile(`^phase (\S+) (\d+(?:\.\d+)?) ms`)

// parsePhases reads `phase NAME MS ms ...` lines (--phase-times).
func parsePhases(s string) map[string]float64 {
	out := map[string]float64{}
	for _, l := range strings.Split(s, "\n") {
		if m := phaseLine.FindStringSubmatch(strings.TrimSpace(l)); m != nil {
			v, _ := strconv.ParseFloat(m[2], 64)
			out[m[1]] += v
		}
	}
	return out
}

func selfTimes(dir string, a, b side, rounds int) (selfStat, error) {
	st := selfStat{phases: [2]map[string][]float64{{}, {}}}
	tmp, err := os.MkdirTemp("", "scorecard-")
	if err != nil {
		return st, err
	}
	defer os.RemoveAll(tmp)
	for i := 0; i < rounds; i++ {
		for j, s := range []side{a, b} {
			c := s.cmd("build", "--phase-times", "-o", filepath.Join(tmp, "xo-"+s.name), dir)
			c.Env = append(c.Env, "XO_BACKEND_CACHE=off", "XO_FE_CACHE=off", "XO_BIN_CACHE=off", "XO_WHOLE_BUILD=off")
			var out bytes.Buffer
			c.Stdout, c.Stderr = &out, &out
			t := time.Now()
			if err := c.Run(); err != nil {
				return st, fmt.Errorf("%s: xo build %s: %v\n%s", s.name, dir, err, lastLine(out.String()))
			}
			st.wall[j] = append(st.wall[j], float64(time.Since(t).Milliseconds()))
			for n, v := range parsePhases(out.String()) {
				st.phases[j][n] = append(st.phases[j][n], v)
			}
		}
	}
	return st, nil
}

func median(v []float64) float64 {
	if len(v) == 0 {
		return 0
	}
	s := append([]float64{}, v...)
	sort.Float64s(s)
	return s[len(s)/2]
}

func writeSelf(w io.Writer, self string, st selfStat) {
	wa, wb := median(st.wall[0]), median(st.wall[1])
	fmt.Fprintf(w, "\n## Compile time: xo build %s, caches off, %d builds per side, medians\n\n", self, len(st.wall[0]))
	fmt.Fprintf(w, "| phase | A ms | B ms | B/A |\n|---|---|---|---|\n")
	fmt.Fprintf(w, "| wall | %.0f | %.0f | %.3f |\n", wa, wb, wb/wa)
	var names []string
	for n := range st.phases[0] {
		names = append(names, n)
	}
	for n := range st.phases[1] {
		if _, ok := st.phases[0][n]; !ok {
			names = append(names, n)
		}
	}
	sort.Slice(names, func(i, j int) bool {
		return median(st.phases[1][names[i]]) > median(st.phases[1][names[j]])
	})
	for i, n := range names {
		if i == 12 {
			break
		}
		pa, pb := median(st.phases[0][n]), median(st.phases[1][n])
		r := "-"
		if pa > 0 {
			r = fmt.Sprintf("%.3f", pb/pa)
		}
		fmt.Fprintf(w, "| %s | %.0f | %.0f | %s |\n", n, pa, pb, r)
	}
	fmt.Fprintf(w, "\ncompile: wall A %.0f ms, B %.0f ms (B/A %.3f)\n", wa, wb, wb/wa)
}
