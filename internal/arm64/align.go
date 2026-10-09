package arm64

import (
	"fmt"
	"regexp"
	"strings"
)

// Code placement: alignment of functions and loop tops, and a test only
// shift of every function (decision 0075). All of it is off by default:
// on Apple M4 no setting made every bench program faster, and loop
// alignment made placement stable at a worse point
// (bench/results/arm64-perf-2026-10-08.md, "Code placement"). It stays as
// an instrument: $XO_ARM64_LAYOUT (internal/build) sets it for a build.
//
// alignLoops puts `.p2align` before the top of each innermost loop (the
// earliest label a later branch of the loop goes back to). Code that
// falls into the top jumps over the padding, so the padding never runs,
// unless Layout.LoopFall lets it run through the nops once per entry.

// Layout is the placement of code: zero values are the defaults.
type Layout struct {
	// Func is log2 of the alignment of Xo functions (0: the default, 4
	// bytes).
	Func int
	// Loop is log2 of the alignment of loop tops (0: none).
	Loop int
	// LoopFall lets the code before a loop top fall through the
	// padding (the nops run once per entry into the loop) instead of
	// jumping over it.
	LoopFall bool
	// LoopOff puts the loop top LoopOff instructions past the aligned
	// address (nops, jumped over).
	LoopOff int
	// Outer aligns the tops of outer loops too, not only innermost ones.
	Outer bool
	// Pad puts Pad nops before every Xo function's label (after its
	// alignment), to move the code for placement sensitivity tests;
	// the nops never run.
	Pad int
}

// ParseLayout reads a layout written as comma separated key=value pairs
// (func=6,loop=5,fall=1,pad=3), the form of $XO_ARM64_LAYOUT.
func ParseLayout(s string) (Layout, error) {
	var l Layout
	for _, kv := range strings.Split(s, ",") {
		kv = strings.TrimSpace(kv)
		if kv == "" {
			continue
		}
		k, v, ok := strings.Cut(kv, "=")
		var n int
		if _, err := fmt.Sscanf(v, "%d", &n); !ok || err != nil || n < 0 || n > 64 {
			return l, fmt.Errorf("bad layout setting %q", kv)
		}
		switch k {
		case "func":
			l.Func = n
		case "loop":
			l.Loop = n
		case "fall":
			l.LoopFall = n != 0
		case "off":
			l.LoopOff = n
		case "outer":
			l.Outer = n != 0
		case "pad":
			l.Pad = n
		default:
			return l, fmt.Errorf("unknown layout setting %q", k)
		}
	}
	if l.Func > 12 || l.Loop > 12 {
		return l, fmt.Errorf("alignment past 2^12")
	}
	return l, nil
}

var (
	labelRE  = regexp.MustCompile(`^([.A-Za-z_][A-Za-z0-9_.$]*):$`)
	branchRE = regexp.MustCompile(`^\t(?:b|b\.[a-z]+|cbn?z [wx][0-9]+|tbn?z [wx][0-9]+, #[0-9]+) ([.A-Za-z_][A-Za-z0-9_.$]*)$`)
	noFallRE = regexp.MustCompile(`^\t(b |ret$|br |brk )`)
	csRE     = regexp.MustCompile(`_cs[0-9]+[ab]$`)
)

// alignLoops aligns the loop tops of a function body (without its out of
// line stubs) to 2^p bytes. loop maps the label of each block of an
// innermost loop to the loop's number. A loop's top is the earliest
// label that a branch from the loop, later in the text, goes back to:
// the start of the code that runs on every iteration (after a rotated
// check, or past a header whose test the latch copies).
func alignLoops(body string, p int, fall bool, off int, loop map[string]int) string {
	if p <= 2 || len(loop) == 0 {
		return body
	}
	lines := strings.Split(body, "\n")
	at := map[string]int{}
	for i, ln := range lines {
		if m := labelRE.FindStringSubmatch(ln); m != nil {
			at[m[1]] = i
		}
	}
	first := map[int]int{} // loop number: line of its top
	target := map[int]bool{}
	cur := ""
	for i, ln := range lines {
		if m := labelRE.FindStringSubmatch(ln); m != nil {
			if !csRE.MatchString(m[1]) && !strings.HasSuffix(m[1], "_begin") {
				// (call site labels are inside a block, decision 0085)
				cur = strings.TrimSuffix(m[1], "_c")
			}
			continue
		}
		m := branchRE.FindStringSubmatch(ln)
		if m == nil {
			continue
		}
		lc, ok1 := loop[cur]
		lt, ok2 := loop[strings.TrimSuffix(m[1], "_c")]
		j, ok3 := at[m[1]]
		if !ok1 || !ok2 || !ok3 || lc != lt || j >= i {
			continue
		}
		target[j] = true
		if f, ok := first[lc]; !ok || j < f {
			first[lc] = j
		}
	}
	top := map[int]bool{}
	for _, j := range first {
		// a rotated check: the loop branches back past it (L_c); the
		// branch to L itself only runs when the check fails
		if c, ok := at[labelRE.FindStringSubmatch(lines[j])[1]+"_c"]; ok && target[c] {
			j = c
		}
		top[j] = true
	}
	out := make([]string, 0, len(lines)+2*len(top))
	for i, ln := range lines {
		if top[i] {
			// the instruction before: skip labels, comments, blank lines
			k := i - 1
			for k >= 0 && (labelRE.MatchString(lines[k]) || strings.HasPrefix(lines[k], "//") || lines[k] == "") {
				k--
			}
			// in front of the labels that share the address
			j := len(out)
			for j > 0 && labelRE.MatchString(out[j-1]) {
				j--
			}
			var pad []string
			if k >= 0 && !noFallRE.MatchString(lines[k]) && !fall {
				// fallen into: jump over the padding (one taken branch
				// per entry into the loop instead of the nops)
				pad = append(pad, "\tb "+labelRE.FindStringSubmatch(ln)[1])
			}
			pad = append(pad, fmt.Sprintf("\t.p2align %d", p))
			for range off {
				pad = append(pad, "\tnop")
			}
			out = append(out[:j], append(pad, out[j:]...)...)
		}
		out = append(out, ln)
	}
	return strings.Join(out, "\n")
}

// innerLoops maps the label of every block of an innermost natural loop
// (one whose body holds no other loop's header) to the loop's number;
// with outer, of every loop (a block belongs to the smallest).
// Back edges come from a depth first search, as in loopDepth.
func (s *fstate) innerLoops(outer bool) map[string]int {
	if len(s.order) == 0 {
		return nil
	}
	state := map[*mblock]int{}
	bodies := map[*mblock]map[*mblock]bool{}
	var heads []*mblock
	var dfs func(b *mblock)
	dfs = func(b *mblock) {
		state[b] = 1
		for _, t := range b.succs {
			switch state[t] {
			case 0:
				dfs(t)
			case 1:
				body := bodies[t]
				if body == nil {
					body = map[*mblock]bool{t: true}
					bodies[t] = body
					heads = append(heads, t)
				}
				work := []*mblock{b}
				for len(work) > 0 {
					x := work[len(work)-1]
					work = work[:len(work)-1]
					if body[x] {
						continue
					}
					body[x] = true
					work = append(work, x.preds...)
				}
			}
		}
		state[b] = 2
	}
	dfs(s.order[0])
	// each block belongs to the smallest loop that holds it
	out := map[string]int{}
	size := map[string]int{}
	for n, h := range heads {
		inner := true
		for _, h2 := range heads {
			if h2 != h && bodies[h][h2] {
				inner = false
			}
		}
		if !inner && !outer {
			continue
		}
		for x := range bodies[h] {
			l := s.label(x)
			if k, ok := size[l]; !ok || len(bodies[h]) < k {
				out[l], size[l] = n, len(bodies[h])
			}
		}
	}
	return out
}
