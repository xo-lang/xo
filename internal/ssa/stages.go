package ssa

import (
	"fmt"
	"sort"
	"strings"

	"github.com/xo-lang/xo/internal/check"
)

// StageNames are the stages of a build that `xo ssadump` prints (decision
// 0150), in order, each the module as it leaves it:
//
//   - build: the builder's output before any pass (blocks and values in
//     creation order, unreachable blocks and trivial block parameters
//     still there), printed even when the builder reported errors;
//   - unwind: after pruneUnwind (checks after calls of functions that
//     cannot unwind or fault removed);
//   - landings: after addLandings (every operation that can fault has a
//     landing; builds whose faults unwind);
//   - cleanup: after cleanup (pruned, trivial parameters removed,
//     renumbered), what Build returns;
//   - rc: after InsertRC (decision 0046).
var StageNames = []string{"build", "unwind", "landings", "cleanup", "rc"}

// Stage is the printed module after one stage.
type Stage struct {
	Name string
	Text string
}

// BuildStages builds res like BuildOpts and returns the printed module
// after each stage named in want, in build order. When the builder
// reports errors only the build stage is printed, and the errors are
// returned with it.
func BuildStages(res *check.Result, opts Options, want map[string]bool) ([]Stage, error) {
	var out []Stage
	m, err := buildWith(res, opts, func(b *builder, stage string) {
		if want[stage] {
			out = append(out, Stage{stage, b.mod.String()})
		}
	})
	if err != nil {
		return out, err
	}
	if want["rc"] || want["verify"] || want["mutants"] {
		m.InsertRC()
		if want["rc"] {
			out = append(out, Stage{"rc", m.String()})
		}
	}
	if want["verify"] {
		out = append(out, Stage{"verify", verifyText(m.Verify())})
	}
	if want["mutants"] {
		out = append(out, Stage{"mutants", m.VerifyMutants(MutantsPerFunc)})
	}
	return out, nil
}

// verifyText is the text of the stage `verify` (decision 0150 stage 8):
// `ok`, or the verifier's errors sorted, one per line.
func verifyText(err error) string {
	if err == nil {
		return "ok\n"
	}
	var lines []string
	if ve, ok := err.(VerifyError); ok {
		lines = append(lines, ve...)
	} else {
		lines = append(lines, err.Error())
	}
	sort.Strings(lines)
	return strings.Join(lines, "\n") + "\n"
}

// MutantsPerFunc is how many mutants of each function the stage
// `mutants` verifies.
const MutantsPerFunc = 6

// VerifyMutants removes, one at a time, the first n retains and releases
// of every function of m (after InsertRC) and verifies the function
// without it (TestVerifyRCMutations' mutants): one line per mutant,
// `<function> b<block> <index>: ` and its errors sorted (`; `), or `ok`.
// The self hosted verifier must print the same (decision 0150 stage 8).
func (m *Module) VerifyMutants(n int) string {
	var b strings.Builder
	m.faults = m.mayFault()
	defer func() { m.faults = nil }()
	for _, f := range m.Funcs {
		k := 0
		for _, bl := range f.Blocks {
			for i := 0; i < len(bl.Instrs) && k < n; i++ {
				v := bl.Instrs[i]
				if v.Op != OpRetain && v.Op != OpRelease {
					continue
				}
				k++
				orig := bl.Instrs
				bl.Instrs = append(append([]*Value(nil), orig[:i]...), orig[i+1:]...)
				errs := m.verifyFunc(f)
				if len(errs) == 0 {
					errs = m.verifyRC(f)
				}
				bl.Instrs = orig
				sort.Strings(errs)
				fmt.Fprintf(&b, "%s %s %d: ", f.Name, bl.Name(), i)
				if len(errs) == 0 {
					b.WriteString("ok\n")
				} else {
					b.WriteString(strings.Join(errs, "; ") + "\n")
				}
			}
		}
	}
	return b.String()
}

// BuildRaw runs the builder alone, without the passes after it: the
// module of the build stage (for timing the builder, `xo ssadump
// --bench`). A program that uses tasks is built once, without the
// unwinding checks a full build adds.
func BuildRaw(res *check.Result, opts Options) (*Module, error) {
	opts.raw = true
	b, err := buildModule(res, opts, false, nil)
	return b.mod, err
}

// stage reports the end of a stage to the watcher, if any.
func (b *builder) stage(name string) {
	if b.see != nil {
		b.see(name)
	}
}

// SplitModule splits a printed module (Module.String) into its type lines
// and its functions: each function's text by name, and the names in
// order.
func SplitModule(text string) (types []string, funcs map[string]string, order []string) {
	funcs = map[string]string{}
	var cur strings.Builder
	name := ""
	flush := func() {
		if name != "" {
			funcs[name] = cur.String()
			order = append(order, name)
		}
		cur.Reset()
		name = ""
	}
	for _, l := range strings.SplitAfter(text, "\n") {
		switch {
		case l == "":
		case name == "" && strings.HasPrefix(l, "type "):
			types = append(types, strings.TrimSuffix(l, "\n"))
		case strings.HasPrefix(l, "func "):
			flush()
			name = headerName(l)
			cur.WriteString(l)
		case name != "" && l == "\n":
			// the blank line between functions
		case name != "":
			cur.WriteString(l)
		}
	}
	flush()
	return types, funcs, order
}

// headerName is the name in a function header `func <name>(...`: the text
// up to the parenthesis that opens the parameters, which starts with a
// parameter (`v0: `, `borrow v0: `) or is `() ->`. A name may hold
// brackets of type arguments, which may hold parentheses (`f[(Int,
// Str)]`), and parentheses of its own (`xo.frameargs(id: Int)`).
func headerName(header string) string {
	s := strings.TrimPrefix(header, "func ")
	depth := 0
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '[':
			depth++
		case ']', ')':
			depth--
		case '(':
			rest := s[i+1:]
			if depth == 0 && (strings.HasPrefix(rest, "borrow ") || strings.HasPrefix(rest, ") ->") ||
				len(rest) > 1 && rest[0] == 'v' && rest[1] >= '0' && rest[1] <= '9') {
				return s[:i]
			}
			depth++
		}
	}
	return s
}
