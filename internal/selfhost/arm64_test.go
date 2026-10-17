package selfhost

import (
	"bytes"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// arm64Flags are the flags both emitters run with at the current stage
// of decision 0158: none since stage 4 (inlining); --no-inline before.
var arm64Flags = []string{}

// arm64Heavy are the runs whose emission takes the longest (most of the
// time of the comparison): the merge tier leaves them to the full tier.
func arm64Heavy(d string) bool { return mergeHeavy(d) || d == "compiler/parser" }

// TestSelfhostArm64 runs the Xo arm64 emitter (compiler/arm64, decision
// 0158), built with each backend, on every directory of the repository
// holding .xo files that the SSA builder builds completely, and compares
// the assembly with the recording (the recording's `xo asmdump` until
// decision 0215) piece by piece (arm64Pieces): every function the Xo
// emitter emits must be identical to the recorded one, with its call
// site table; every helper and literal it emits must be a recorded one. Functions it skips (a construct of a
// later stage) are counted, not compared; a run with no function skipped
// must be identical byte for byte as a whole file. The merge tier takes every second run and
// leaves arm64Heavy to the full tier; -short every 25th; the go backend
// only except in the full tier. XO_SELFHOST_DIRS=<texts> takes the runs
// whose directory contains one of the comma separated texts instead.
// XO_SELFHOST_BACKEND names one backend,
// XO_SELFHOST_KEEP=<dir> writes arm64-<backend>.txt (the counts).
func TestSelfhostArm64(t *testing.T) {
	skipDumpReplay(t)
	backends := tierBackends(t)
	if v := os.Getenv("XO_SELFHOST_BACKEND"); v != "" {
		backends = []string{v}
	}
	root := repoRoot(t)
	all := moduleDirs(t, root)
	var dirs []string
	step := byTier(t, 25, 2, 1)
	for i, d := range all {
		if v := os.Getenv("XO_SELFHOST_DIRS"); v != "" {
			// the runs whose directory contains one of the comma
			// separated texts (a development loop, as TestSelfhostSSA)
			for _, w := range strings.Split(v, ",") {
				if strings.Contains(d, w) {
					dirs = append(dirs, d)
					break
				}
			}
			continue
		}
		if i%step != 0 || (selfhostTier(t) != tierFull && arm64Heavy(d)) {
			continue
		}
		dirs = append(dirs, d)
	}
	t.Chdir(root)
	setModEnv(t, root)
	bins := selfhostBinaries(t, "arm64", backends)
	for _, b := range backends {
		bin, ok := bins[b]
		if !ok {
			continue
		}
		t.Run(b, func(t *testing.T) {
			t.Parallel()
			got := runArm64(t, bin, root, dirs, arm64Flags, "--std=std")
			wants := make([]*goldPieces, len(dirs))
			for i, d := range dirs {
				wants[i] = goAsm(root, d, arm64Flags, got[d])
			}
			st, fails := compareAll(t, "compare arm64", dirs, func(i int, st *arm64Stats) []string {
				return compareArm64(dirs[i], wants[i], got[dirs[i]], st)
			}, (*arm64Stats).add)
			t.Logf("%s", st)
			if keep := os.Getenv("XO_SELFHOST_KEEP"); keep != "" {
				os.MkdirAll(keep, 0o755)
				os.WriteFile(filepath.Join(keep, "arm64-"+b+".txt"), []byte(st.report()), 0o644)
			}
			reportFails(t, fails)
		})
	}
}

// TestSelfhostArm64Mutations compares the two emitters on mutations of
// the single directory programs with `main` (mutateCode, as
// TestSelfhostSSAMutations): 300 in the full tier, 30 in the merge
// tier, none with -short; XO_SELFHOST_ARM64_FUZZ=N and XO_SELFHOST_SEED
// override. The go backend, or the one XO_SELFHOST_BACKEND names.
func TestSelfhostArm64Mutations(t *testing.T) {
	skipDumpReplay(t)
	if selfhostTier(t) == tierShort {
		t.Skip("checks every repository program to find the sources")
	}
	n := byTier(t, 0, 30, 300)
	if v := os.Getenv("XO_SELFHOST_ARM64_FUZZ"); v != "" {
		fmt.Sscan(v, &n)
	}
	seed := int64(20261011)
	if v := os.Getenv("XO_SELFHOST_SEED"); v != "" {
		fmt.Sscan(v, &seed)
	}
	root := repoRoot(t)
	backend := "go"
	if v := os.Getenv("XO_SELFHOST_BACKEND"); v != "" {
		backend = v
	}
	bin, err := selfhostBinary("arm64", backend)
	if err != nil {
		t.Fatal(err)
	}
	var srcs []mutationSource
	for _, s := range mutationSources(t, root) {
		for _, f := range s.files {
			if bytes.Contains(f, []byte("fn main(")) {
				srcs = append(srcs, s)
				break
			}
		}
	}
	tmp := t.TempDir()
	stdDir, err := filepath.Abs(filepath.Join(root, "std"))
	if err != nil {
		t.Fatal(err)
	}
	rng := rand.New(rand.NewSource(seed))
	var dirs []string
	for i := range n {
		src := srcs[rng.Intn(len(srcs))]
		dir := fmt.Sprintf("a%04d", i)
		if err := os.MkdirAll(filepath.Join(tmp, dir), 0o755); err != nil {
			t.Fatal(err)
		}
		f := rng.Intn(len(src.files))
		for j, name := range src.names {
			data := src.files[j]
			if j == f {
				data = mutateCode(rng, data)
			}
			if err := os.WriteFile(filepath.Join(tmp, dir, name), data, 0o644); err != nil {
				t.Fatal(err)
			}
		}
		dirs = append(dirs, dir)
	}
	t.Chdir(tmp)
	setModEnv(t, root)
	wd := workDir(t)
	got := runArm64(t, bin, tmp, dirs, arm64Flags, "--std="+stdDir)
	wants := make([]*goldPieces, len(dirs))
	for i, d := range dirs {
		wants[i] = goAsm(wd, d, arm64Flags, got[d])
	}
	var st arm64Stats
	var fails []string
	for i, d := range dirs {
		fails = append(fails, compareArm64(d, wants[i], got[d], &st)...)
	}
	t.Logf("%d mutations: %s", n, st)
	reportFails(t, fails)
}

func reportFails(t *testing.T, fails []string) {
	if len(fails) == 0 {
		return
	}
	show := 10
	if v := os.Getenv("XO_SELFHOST_FAILS"); v != "" {
		fmt.Sscan(v, &show)
	}
	n := len(fails)
	if n > show {
		fails = append(fails[:show], fmt.Sprintf("... and %d more", n-show))
	}
	t.Fatalf("%d differences:\n%s", n, strings.Join(fails, "\n"))
}

// goAsm is the recorded assembly of dir checked from root (the asmdump
// kind, gogoldens_test.go): the assembly, or `== rejected`, `== error
// <why>`, cut into the pieces compareArm64 compares: meta "head" (the
// first line, when it is one of `== rejected`, `== error <why>`), else
// "all" (the whole file), and of arm64Pieces "f:<function>",
// "l:<function>" (its call site table), "h:<helper>", the set
// "literals", and "entry". got is the Xo emitter's output for dir, which
// a recording run records (xoAsmPieces).
func goAsm(root, dir string, flags []string, got string) *goldPieces {
	wd := goldensWD()
	cx := newGoldenCtx(wd, root, dir)
	o := fmt.Sprintf("flags=%s lineroot=%s", strings.Join(flags, ","), cx.norm(root))
	key := []string{o, cx.dirKey(wd, dir, true)}
	args := strings.TrimSpace("--each --std=std --wd=" + wd + " " + strings.Join(flags, " "))
	hint := goldHint("TestSelfhostArm64(Mutations)?", "arm64", args+" "+dir, wd)
	return recordedPieces("asmdump", dir+" "+o, hint, key, cx, func() *goldPieces { return xoAsmPieces(got) })
}

// xoAsmPieces is the Xo emitter's output for a run as the pieces a
// recording records: a run it cannot emit (the SSA builder skipped a
// function) as `== error unsupported ...` (compareArm64 counts it as not
// built), else the assembly without the skipped list.
func xoAsmPieces(got string) *goldPieces {
	xh := firstLine(got)
	switch {
	case strings.HasPrefix(xh, "== unsupported"):
		return asmGoldPieces("== error " + strings.TrimPrefix(xh, "== "))
	case strings.HasPrefix(xh, "== "):
		return asmGoldPieces(xh)
	}
	body, _, _ := strings.Cut(got, "== skipped\n")
	return asmGoldPieces(body)
}

// asmGoldPieces cuts an emitter's text into goAsm's pieces.
func asmGoldPieces(text string) *goldPieces {
	p := newPieces()
	if h := firstLine(text); strings.HasPrefix(h, "== ") {
		p.setMeta("head", h)
		return p
	}
	p.add("all", text)
	g := arm64Pieces(text)
	for _, f := range []string{"f", "l", "h"} {
		p.addFamily(f)
	}
	for name, t := range g.funcs {
		p.addMember("f", name, t)
	}
	for name, t := range g.lsdas {
		p.addMember("l", name, t)
	}
	for key, t := range g.helpers {
		p.addMember("h", key, t)
	}
	p.addSet("literals")
	for lit := range g.literals {
		p.addSet("literals", lit)
	}
	p.add("entry", g.entry)
	return p
}

var asmHead = regexp.MustCompile(`(?m)^=== asm `)

// runArm64 runs the Xo emitter on every dir (--each) from root and
// splits its output by run.
func runArm64(t *testing.T, bin, root string, dirs []string, flags []string, more ...string) map[string]string {
	out := map[string]string{}
	const batch = 20
	args := append(append([]string{"--each", "--wd=" + workDir(t)}, flags...), more...)
	for _, xb := range runXoBatches(t, bin, root, args, dirs, batch) {
		chunk := xb.paths
		parts := asmHead.Split(xb.out, -1)
		if len(parts) != len(chunk)+1 {
			t.Fatalf("Xo emitter printed %d runs for %d directories", len(parts)-1, len(chunk))
		}
		for j, d := range chunk {
			head := d + "\n"
			if !strings.HasPrefix(parts[j+1], head) {
				t.Fatalf("run %d is not %s", j, d)
			}
			out[d] = strings.TrimPrefix(parts[j+1], head)
		}
	}
	return out
}

// add adds the counts of o to s.
func (s *arm64Stats) add(o arm64Stats) {
	s.runs += o.runs
	s.compared += o.compared
	s.notBuilt += o.notBuilt
	s.rejected += o.rejected
	s.byteSame += o.byteSame
	s.funcs += o.funcs
	s.same += o.same
	s.skipped += o.skipped
	s.helpers += o.helpers
	s.helpersSame += o.helpersSame
	s.literals += o.literals
	addCounts(&s.skips, o.skips)
	addCounts(&s.notBuiltWhy, o.notBuiltWhy)
}

type arm64Stats struct {
	runs, compared, notBuilt, rejected, byteSame         int
	funcs, same, skipped, helpers, helpersSame, literals int
	skips                                                map[string]int
	notBuiltWhy                                          map[string]int
}

func (s arm64Stats) String() string {
	return fmt.Sprintf("%d runs: %d compared, %d rejected by both, %d not built completely by both SSA builders; "+
		"%d runs byte identical; the recording's %d functions in the runs compared: %d identical, %d skipped by the Xo emitter; "+
		"%d helpers emitted by the Xo emitter, %d identical; %d literals",
		s.runs, s.compared, s.rejected, s.notBuilt, s.byteSame, s.funcs, s.same, s.skipped, s.helpers, s.helpersSame, s.literals)
}

func (s arm64Stats) report() string {
	var b strings.Builder
	b.WriteString(s.String() + "\n")
	for _, m := range []map[string]int{s.notBuiltWhy, s.skips} {
		var keys []string
		for k := range m {
			keys = append(keys, k)
		}
		sort.Slice(keys, func(i, j int) bool { return m[keys[i]] > m[keys[j]] || m[keys[i]] == m[keys[j]] && keys[i] < keys[j] })
		for _, k := range keys {
			fmt.Fprintf(&b, "%6d %s\n", m[k], k)
		}
	}
	return b.String()
}

// compareArm64 compares one run and returns its differences.
func compareArm64(dir string, want *goldPieces, got string, st *arm64Stats) []string {
	st.runs++
	if st.skips == nil {
		st.skips, st.notBuiltWhy = map[string]int{}, map[string]int{}
	}
	gh, xh := want.m("head"), firstLine(got)
	switch {
	case want.missing:
		return []string{dir + ": " + want.missingText()}
	case gh == "== rejected":
		if xh != "== rejected" && !strings.HasPrefix(xh, "== unsupported") {
			return []string{dir + ": the recording rejects it, the Xo checker does not: " + xh}
		}
		st.rejected++
		return nil
	case xh == "== rejected":
		return []string{dir + ": the Xo checker rejects it, the recording does not"}
	case strings.HasPrefix(xh, "== error"):
		return []string{dir + ": the Xo emitter: " + xh}
	case strings.HasPrefix(gh, "== error") || strings.HasPrefix(xh, "== unsupported"):
		// the Go SSA builder reports an unsupported construct, or the Xo
		// builder skipped a function: no module after the passes to
		// compare (decision 0150 stage 8)
		st.notBuilt++
		why := xh
		if strings.HasPrefix(gh, "== error") {
			why = "go: " + gh
		}
		if len(why) > 90 {
			why = why[:90]
		}
		st.notBuiltWhy[why]++
		return nil
	}
	st.compared++
	body, skipped, _ := strings.Cut(got, "== skipped\n")
	if want.eq("all", body) {
		st.byteSame++
	} else if strings.TrimSpace(skipped) == "" {
		// since stage 4 every run whose functions were all emitted is the
		// Go emitter's text byte for byte (decision 0158)
		return []string{fmt.Sprintf("%s: the whole file differs: %s", dir, want.diff("all", body))}
	}
	skip := map[string]string{}
	for _, l := range nonEmptyLines(skipped) {
		// `<name>: <why>`: names may hold ": " (frame argument closures)
		i := strings.LastIndex(l, ": ")
		if i < 0 {
			continue
		}
		skip[l[:i]] = l[i+2:]
	}
	x := arm64Pieces(body)
	lits := make([]string, 0, len(x.literals))
	for l := range x.literals {
		lits = append(lits, l)
	}
	// the recorded side's arm64Pieces (asmGoldPieces)
	g := want.resolved("f", x.funcs, sortedKeys(skip)).resolved("l", x.lsdas, nil).resolved("h", x.helpers, nil).resolvedSet("literals", lits)
	var fails []string
	for _, name := range g.names("f:") {
		st.funcs++
		xt, ok := x.funcs[name]
		switch {
		case ok && g.eq("f:"+name, xt) && g.eq("l:"+name, x.lsdas[name]):
			st.same++
		case ok && g.eq("f:"+name, xt):
			fails = append(fails, fmt.Sprintf("%s: the call site table of %s differs: %s", dir, name, g.diff("l:"+name, x.lsdas[name])))
		case ok:
			fails = append(fails, fmt.Sprintf("%s: %s differs: %s", dir, name, g.diff("f:"+name, xt)))
		case skip[name] != "":
			st.skipped++
			st.skips[skip[name]]++
		default:
			fails = append(fails, fmt.Sprintf("%s: the Xo emitter has no %s", dir, name))
		}
	}
	for name := range x.funcs {
		if !g.has("f:" + name) {
			fails = append(fails, fmt.Sprintf("%s: the Xo emitter emits %s, the recording does not", dir, name))
		}
	}
	for key, text := range x.helpers {
		st.helpers++
		if !g.has("h:" + key) {
			fails = append(fails, fmt.Sprintf("%s: the Xo emitter emits the helper %s, the recording does not", dir, key))
		} else if !g.eq("h:"+key, text) {
			fails = append(fails, fmt.Sprintf("%s: the helper %s differs: %s", dir, key, g.diff("h:"+key, text)))
		} else {
			st.helpersSame++
		}
	}
	for lit := range x.literals {
		st.literals++
		if !g.inSet("literals", lit) {
			fails = append(fails, fmt.Sprintf("%s: the Xo emitter's literal %s is not the recording's", dir, lit))
		}
	}
	if !g.eq("entry", x.entry) {
		fails = append(fails, fmt.Sprintf("%s: xo_entry differs: %s", dir, g.diff("entry", x.entry)))
	}
	sort.Strings(fails)
	return fails
}

// asmPieces is an assembly file cut into the pieces arm64Pieces compares,
// with the labels made from counters of the whole program renamed.
type asmPieces struct {
	funcs, helpers, lsdas map[string]string
	literals              map[string]bool
	entry                 string
}

var (
	asmHelperHead = regexp.MustCompile(`^// (dup|drop|share|dupp|dropp|sharep|eq|boxdrop|boxshare|envdrop|envshare) (.*)$`)
	asmLiteral    = regexp.MustCompile(`^(\.?Lxo_(?:str|loc)\d+):$`)
	asmHelperLab  = regexp.MustCompile(`(?m)^(\.?Lxo_[a-z]+\d+):$`)
	asmXoLabel    = regexp.MustCompile(`(\.?L)xo_([a-z_]+?)(\d+)((?:_[a-z0-9]+)*)`)
	asmBegin      = regexp.MustCompile(`L(?:f)(\d+)_begin`)
	asmCallSite   = regexp.MustCompile(`_cs(\d+)([ab])`)
	asmLsdaNum    = regexp.MustCompile(`GCC_except_table_xo(\d+)|f(\d+)_lsda`)
)

// arm64Pieces cuts an assembly file into functions (`// func <name>` to
// the next piece), helpers (`// <kind> <type>`), the call site table of
// each function, the literals, and xo_entry, and renames the labels of
// program wide counters (decision 0158): `f<N>_` of the function's
// number to `f#_`, its call site numbers in order of appearance, helper
// labels to `<kind type>`, literal labels to their text, and the other
// labels of helpers (eq_yes, eq_loop, ...) to `@k` in order of
// appearance within the piece.
func arm64Pieces(text string) asmPieces {
	funcs, helpers, lsdas, lits := map[string]string{}, map[string]string{}, map[string]string{}, map[string]string{}
	var entry string
	lines := strings.Split(text, "\n")
	kind, key := "", ""
	var buf []string
	flush := func() {
		for len(buf) > 0 && (buf[len(buf)-1] == "" || kind == "d" && strings.HasPrefix(buf[len(buf)-1], "\t.p2align")) {
			buf = buf[:len(buf)-1]
		}
		t := strings.Join(buf, "\n")
		switch kind {
		case "f":
			funcs[key] = t
		case "h":
			helpers[key] = t
		case "l":
			lsdas[key] = t
		case "d":
			lits[key] = strings.TrimSpace(strings.ReplaceAll(t, "\n", " "))
		case "e":
			entry = t
		}
		kind, buf = "", nil
	}
	for i, l := range lines {
		switch {
		case strings.HasPrefix(l, "// func "):
			flush()
			kind, key, buf = "f", l[len("// func "):], []string{l}
		case asmHelperHead.MatchString(l):
			flush()
			kind, key, buf = "h", l[3:], []string{l}
		case strings.HasPrefix(l, "\t.section") && strings.Contains(l, "except") && i+2 < len(lines):
			flush()
			m := asmLsdaNum.FindStringSubmatch(lines[i+2])
			n := "?"
			if m != nil {
				n = m[1] + m[2]
			}
			kind, key, buf = "l", n, []string{l}
		case strings.HasPrefix(l, "\t.globl _xo_entry") || strings.HasPrefix(l, "\t.globl xo_entry"):
			if len(buf) > 0 && buf[len(buf)-1] == "\t.p2align 2" {
				buf = buf[:len(buf)-1]
			}
			flush()
			kind, buf = "e", []string{l}
		case strings.HasPrefix(l, "\t.section"):
			flush()
			kind, buf = "x", []string{l}
		case asmLiteral.MatchString(l):
			flush()
			kind, key, buf = "d", l[:len(l)-1], nil
		default:
			buf = append(buf, l)
		}
	}
	flush()
	labels := map[string]string{}
	for k, t := range helpers {
		if m := asmHelperLab.FindStringSubmatch(t); m != nil {
			labels[m[1]] = "<" + k + ">"
		}
	}
	for k, t := range lits {
		labels[k] = "<" + t + ">"
	}
	rename := func(t string) string {
		seen := map[string]string{}
		return asmXoLabel.ReplaceAllStringFunc(t, func(s string) string {
			m := asmXoLabel.FindStringSubmatch(s)
			base := m[1] + "xo_" + m[2] + m[3]
			if r, ok := labels[base]; ok {
				return r + m[4]
			}
			if _, ok := seen[base]; !ok {
				seen[base] = "@" + strconv.Itoa(len(seen))
			}
			return seen[base] + m[4]
		})
	}
	own := func(t, n string) string {
		t = regexp.MustCompile(`(\.?L)f`+n+`_`).ReplaceAllString(t, "${1}f#_")
		t = strings.ReplaceAll(t, "GCC_except_table_xo"+n, "GCC_except_table_xo#")
		seen := map[string]string{}
		t = asmCallSite.ReplaceAllStringFunc(t, func(s string) string {
			m := asmCallSite.FindStringSubmatch(s)
			if _, ok := seen[m[1]]; !ok {
				seen[m[1]] = strconv.Itoa(len(seen))
			}
			return "_cs" + seen[m[1]] + m[2]
		})
		return rename(t)
	}
	p := asmPieces{funcs: map[string]string{}, helpers: map[string]string{}, lsdas: map[string]string{}, literals: map[string]bool{}}
	byNum := map[string]string{}
	for name, t := range funcs {
		n := "?"
		if m := asmBegin.FindStringSubmatch(t); m != nil {
			n = m[1]
		}
		byNum[n] = name
		p.funcs[name] = own(t, n)
	}
	for n, t := range lsdas {
		name, ok := byNum[n]
		if !ok {
			name = "?" + n
		}
		p.lsdas[name] = own(t, n)
	}
	for k, t := range helpers {
		p.helpers[k] = rename(t)
	}
	for _, t := range lits {
		p.literals[t] = true
	}
	p.entry = rename(entry)
	return p
}
