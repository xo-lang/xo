package arm64

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

// Compare chains and jump tables for `match` (decision 0051).
//
// The SSA lowers a `match` on an enum tag (or an Int) to a chain of
// blocks that each compare the same register with one constant and
// branch; reverse postorder puts each arm's code between two tests, so
// every test that fails is a taken branch. jumpTables, a MIR pass after
// threading, does two things with such chains:
//
//   - A chain that tests at least jumpTableMin distinct values, whose
//     table would be at most jumpTableSlack times that many entries,
//     becomes an iSwitch on its first block: one bounds check and an
//     indirect branch through a table of 32 bit offsets placed after the
//     function's fault stubs (gaps go to the chain's final block). The
//     chain blocks nothing else reaches are dropped.
//   - Every other chain is laid out as a run of tests that fall through
//     to each other, so a branch is taken only to the arm that matches.
//
// On an Apple M4 Pro the indirect branch predicts a repeating dispatch
// sequence (an interpreter loop) better than a chain, but resolves later
// and loses on unpredictable tags at every size measured (4 to 16 arms);
// tables are therefore kept for large matches, where dispatch loops live
// (bench/results/arm64-perf-2026-10-08.md).
var (
	jumpTableMin   = 8
	jumpTableSlack = 3
)

func init() {
	// XO_ARM64_JT_MIN overrides jumpTableMin (0 disables tables): a knob
	// for measuring the threshold, not a supported setting.
	if v, err := strconv.Atoi(os.Getenv("XO_ARM64_JT_MIN")); err == nil {
		jumpTableMin = v
	}
}

// chainTest reports the register and constant of a block's terminator
// when it is `cmp r, #k` with b.eq or b.ne, and which target is taken
// when r == k (the other continues the chain).
func chainTest(in *inst) (r vreg, k int64, match, next *mblock, ok bool) {
	if in == nil || in.kind != iCondBr || len(in.uses) != 1 || (in.cond != "eq" && in.cond != "ne") {
		return 0, 0, nil, nil, false
	}
	if !strings.HasPrefix(in.text, "cmp $u0, #") {
		return 0, 0, nil, nil, false
	}
	n, err := strconv.ParseInt(in.text[len("cmp $u0, #"):], 10, 64)
	if err != nil {
		return 0, 0, nil, nil, false
	}
	if in.cond == "eq" {
		return in.uses[0], n, in.targets[0], in.targets[1], true
	}
	return in.uses[0], n, in.targets[1], in.targets[0], true
}

// chain follows the compare chain that starts at head: the blocks after
// head that hold nothing but a compare of the same register, the first
// target of each value, and the block reached when no value matches.
func chain(head *mblock) (blocks []*mblock, cases map[int64]*mblock, keys []int64, def *mblock) {
	r, _, _, _, ok := chainTest(head.term())
	if !ok {
		return nil, nil, nil, nil
	}
	cases = map[int64]*mblock{}
	seen := map[*mblock]bool{head: true}
	b := head
	for {
		_, k, match, next, _ := chainTest(b.term())
		if _, dup := cases[k]; !dup {
			cases[k] = match
			keys = append(keys, k)
		}
		r2, _, _, _, ok2 := chainTest(next.term())
		if !ok2 || r2 != r || len(next.insts) != 1 || seen[next] || next.cold {
			return blocks, cases, keys, next
		}
		seen[next] = true
		blocks = append(blocks, next)
		b = next
	}
}

// jumpTables replaces dense compare chains with iSwitch terminators and
// lays out the other chains as a run of tests that fall through to each
// other (a branch is taken only to the arm that matches).
func (s *fstate) jumpTables() {
	changed := false
	for _, head := range s.hot {
		_, cases, keys, def := chain(head)
		if jumpTableMin <= 0 || len(keys) < jumpTableMin {
			continue
		}
		lo, hi := keys[0], keys[0]
		for _, k := range keys {
			lo, hi = min(lo, k), max(hi, k)
		}
		size := hi - lo + 1
		if lo < 0 || lo >= 4096 || size > 4096 || size > int64(jumpTableSlack*len(keys)) {
			continue
		}
		targets := make([]*mblock, size+1)
		for i := range targets {
			targets[i] = def
		}
		for k, t := range cases {
			targets[k-lo] = t
		}
		head.insts[len(head.insts)-1] = &inst{kind: iSwitch, text: strconv.FormatInt(lo, 10), uses: []vreg{head.term().uses[0]}, targets: targets}
		changed = true
	}
	if changed {
		s.thread() // drops the chain blocks nothing reaches now
	}
	placed := map[*mblock]bool{}
	for i := 0; i < len(s.hot); i++ {
		head := s.hot[i]
		if placed[head] {
			continue
		}
		blocks, _, _, _ := chain(head)
		if len(blocks) == 0 {
			continue
		}
		move := map[*mblock]bool{}
		for _, b := range blocks {
			move[b] = true
			placed[b] = true
		}
		out := make([]*mblock, 0, len(s.hot))
		for _, b := range s.hot {
			if move[b] {
				continue
			}
			out = append(out, b)
			if b == head {
				out = append(out, blocks...)
			}
		}
		s.hot = out
	}
}

// emitSwitch emits an iSwitch: values outside the table go to the last
// target; the table holds offsets from its own label.
func (s *fstate) emitSwitch(b *strings.Builder, in *inst, fr *frameInfo) {
	lo, _ := strconv.ParseInt(in.text, 10, 64)
	n := len(in.targets) - 1
	idx := s.useReg(b, in.uses[0], 2, fr)
	if lo != 0 {
		fmt.Fprintf(b, "\tsub x17, %s, #%d\n", idx, lo)
		idx = "x17"
	}
	tab := s.g.local(fmt.Sprintf("f%d_jt%d", s.g.nfunc, s.cur.id))
	fmt.Fprintf(b, "\tcmp %s, #%d\n", idx, n-1)
	fmt.Fprintf(b, "\tb.hi %s\n", s.label(s.fwd(in.targets[n], fr)))
	fmt.Fprintf(b, "\tadr x16, %s\n", tab)
	fmt.Fprintf(b, "\tldrsw x17, [x16, %s, lsl #2]\n", idx)
	b.WriteString("\tadd x16, x16, x17\n\tbr x16\n")
	fmt.Fprintf(&s.stubs, "\t.p2align 2\n%s:\n", tab)
	for _, t := range in.targets[:n] {
		fmt.Fprintf(&s.stubs, "\t.long %s-%s\n", s.label(s.fwd(t, fr)), tab)
	}
}
