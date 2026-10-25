package main

import (
	"encoding/binary"
	"fmt"
	"io"
	"sort"
)

// ReachReport is the -reach result: the symbols reachable from the entry
// points over direct branches and materialized function addresses. Indirect
// calls through a register loaded from memory are not followed, so the
// unreachable list is a lower bound on dead code, never a proof.
type ReachReport struct {
	Supported        bool     `json:"supported"`
	Note             string   `json:"note,omitempty"`
	Symbols          int      `json:"symbols"`
	Reachable        int      `json:"reachable"`
	Unreachable      int      `json:"unreachable"`
	UnreachableBytes uint64   `json:"unreachable_bytes"`
	Text             uint64   `json:"text"`
	Top              []Symbol `json:"top,omitempty"`
}

// roots are the entry points every program's reachability starts from:
// the process entry, the support entry, the runtime's program entry and the
// program's main. Mach-O's symbols carry a leading underscore and ELF's do
// not (_start, main, xo_entry, xo_f_main), so both spellings are listed.
var roots = []string{
	"_main", "main",
	"_start", "start",
	"_xo_entry", "xo_entry",
	"_xo_x_rt_program", "xo_x_rt_program",
	"_xo_f_main", "xo_f_main",
}

func reachability(bin *execFile) *ReachReport {
	rep := &ReachReport{Supported: true, Text: bin.Text.Size}
	if bin.Arch != "arm64" {
		rep.Supported = false
		rep.Note = "the reachability scan decodes arm64 only; the size report stands"
		return rep
	}
	syms := make([]Symbol, len(bin.Syms))
	copy(syms, bin.Syms)
	sort.Slice(syms, func(i, j int) bool {
		if syms[i].Addr != syms[j].Addr {
			return syms[i].Addr < syms[j].Addr
		}
		return syms[i].Name < syms[j].Name
	})
	uniq := syms[:0]
	for _, s := range syms {
		if n := len(uniq); n > 0 && uniq[n-1].Addr == s.Addr {
			continue
		}
		uniq = append(uniq, s)
	}
	syms = uniq
	textEnd := bin.Text.Addr + bin.Text.Size
	for i := range syms {
		end := textEnd
		if i+1 < len(syms) {
			end = syms[i+1].Addr
		}
		syms[i].Size = end - syms[i].Addr
	}
	at := make(map[uint64]int, len(syms))
	for i, s := range syms {
		at[s.Addr] = i
	}
	edges := make([][]int, len(syms))
	linked := make([]map[int]bool, len(syms))
	addEdge := func(from, to int) {
		if from == to || from < 0 || to < 0 {
			return
		}
		if linked[from] == nil {
			linked[from] = map[int]bool{}
		}
		if !linked[from][to] {
			linked[from][to] = true
			edges[from] = append(edges[from], to)
		}
	}
	code := bin.Code
	cur := -1
	addrOf := map[uint32]uint64{}
	for off := 0; off+4 <= len(code); off += 4 {
		pc := bin.Text.Addr + uint64(off)
		for cur+1 < len(syms) && syms[cur+1].Addr <= pc {
			cur++
			addrOf = map[uint32]uint64{}
		}
		insn := binary.LittleEndian.Uint32(code[off:])
		target := func(imm int64) {
			t := pc + uint64(imm)
			if to, ok := at[t]; ok {
				addEdge(cur, to)
			}
		}
		switch {
		case insn&0xfc000000 == 0x94000000, insn&0xfc000000 == 0x14000000: // BL, B
			imm := int64(insn & 0x03ffffff)
			if imm&0x02000000 != 0 {
				imm -= 0x04000000
			}
			target(imm << 2)
		case insn&0x9f000000 == 0x90000000: // ADRP
			rd := insn & 0x1f
			imm := int64((insn>>5)&0x7ffff)<<2 | int64((insn>>29)&3)
			if imm&(1<<20) != 0 {
				imm -= 1 << 21
			}
			addrOf[rd] = (pc &^ 0xfff) + uint64(imm<<12)
		case insn&0x7f000000 == 0x11000000 && insn>>31&1 == 1: // ADD (immediate), 64 bit
			rd := insn & 0x1f
			rn := (insn >> 5) & 0x1f
			imm := uint64((insn >> 10) & 0xfff)
			if insn>>22&1 == 1 {
				imm <<= 12
			}
			if a, ok := addrOf[rn]; ok {
				addrOf[rd] = a + imm
				if to, ok := at[addrOf[rd]]; ok {
					addEdge(cur, to)
				}
			}
		}
	}

	reached := make([]bool, len(syms))
	var queue []int
	for _, name := range roots {
		for i, s := range syms {
			if s.Name == name && !reached[i] {
				reached[i] = true
				queue = append(queue, i)
			}
		}
	}
	if len(queue) == 0 {
		rep.Supported = false
		rep.Note = "no entry symbol found (" + joinNames(roots) + "); the size report stands"
		return rep
	}
	for len(queue) > 0 {
		i := queue[0]
		queue = queue[1:]
		for _, j := range edges[i] {
			if !reached[j] {
				reached[j] = true
				queue = append(queue, j)
			}
		}
	}
	rep.Symbols = len(syms)
	var dead []Symbol
	for i, s := range syms {
		if reached[i] {
			rep.Reachable++
			continue
		}
		rep.Unreachable++
		rep.UnreachableBytes += s.Size
		dead = append(dead, s)
	}
	sort.Slice(dead, func(i, j int) bool {
		if dead[i].Size != dead[j].Size {
			return dead[i].Size > dead[j].Size
		}
		return dead[i].Name < dead[j].Name
	})
	if len(dead) > 10 {
		dead = dead[:10]
	}
	rep.Top = dead
	return rep
}

func joinNames(xs []string) string {
	s := ""
	for i, x := range xs {
		if i > 0 {
			s += ", "
		}
		s += x
	}
	return s
}

// printReach writes the -reach section, labelled as the lower bound it is.
func printReach(w io.Writer, r *ReachReport) {
	if !r.Supported {
		fmt.Fprintf(w, "\nreach: not measured: %s\n", r.Note)
		return
	}
	share := 0.0
	if r.Text > 0 {
		share = 100 * float64(r.UnreachableBytes) / float64(r.Text)
	}
	fmt.Fprintf(w, "\nreach (lower bound: direct branches and materialized addresses, no indirect calls):\n")
	fmt.Fprintf(w, "  %d of %d symbols reachable, %d unreachable = %d B (%.1f%% of __text)\n",
		r.Reachable, r.Symbols, r.Unreachable, r.UnreachableBytes, share)
	fmt.Fprintf(w, "  largest unreachable:\n")
	for _, s := range r.Top {
		fmt.Fprintf(w, "    %8d  %-48s [%s]\n", s.Size, displayName(s.Name), groupOf(s.Name))
	}
}
