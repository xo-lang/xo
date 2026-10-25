package main

import (
	"fmt"
	"io"
	"sort"
	"strings"
)

// Symbol is one text symbol of the binary, Size filled by analyze.
type Symbol struct {
	Name string `json:"name"`
	Addr uint64 `json:"addr"`
	Size uint64 `json:"size"`
}

// TextInfo is the text section the symbols are measured against.
type TextInfo struct {
	Addr uint64 `json:"addr"`
	Size uint64 `json:"size"`
}

// Group is one module's share of the text.
type Group struct {
	Name    string  `json:"name"`
	Bytes   uint64  `json:"bytes"`
	Symbols int     `json:"symbols"`
	Share   float64 `json:"share"` // of the text section, 0 to 1
}

// Instantiation is one element type a Map family was instantiated for.
type Instantiation struct {
	Type    string `json:"type"`
	Bytes   uint64 `json:"bytes"`
	Symbols int    `json:"symbols"`
}

// Family is the Map runtime's map, mlog or maph, with its instantiations.
type Family struct {
	Name    string          `json:"name"`
	Bytes   uint64          `json:"bytes"`
	Symbols int             `json:"symbols"`
	Top     []Instantiation `json:"top"`
}

// Report is everything the tool measured.
type Report struct {
	Binary   string       `json:"binary"`
	Format   string       `json:"format"`
	Text     TextInfo     `json:"text"`
	Sum      uint64       `json:"sum"`
	Symbols  int          `json:"symbols"`
	Aliases  int          `json:"aliases"`
	Groups   []Group      `json:"groups"`
	Families []Family     `json:"map_families"`
	Top      []Symbol     `json:"top"`
	Reach    *ReachReport `json:"reach,omitempty"`
}

// Hidden is the group of the compiler's hidden helpers (_xo_h*), which carry
// no module: generic instantiations of drops, shares and comparisons.
const Hidden = "hidden (_xo_h*)"

// NonXo is the group of the symbols that are not Xo functions: the entry
// points, the runtime's _xo_x_* trampolines and anything the linker added.
const NonXo = "non-Xo"

// xoPrefix is the mangling of a Xo function: _xo_f_<module>_2e<name>, the
// module and name further escaped (_5b is [, _2c is , _20 is space).
const xoPrefix = "_xo_f_"

// moduleOf returns the module of a Xo function's symbol.
func moduleOf(name string) (string, bool) {
	if !strings.HasPrefix(name, xoPrefix) {
		return "", false
	}
	rest := name[len(xoPrefix):]
	parts := strings.Split(rest, "_2e")
	// xo.map, xo.mlog and xo.maph are the Map runtime's submodules: their
	// symbols carry two dots before the function name.
	if parts[0] == "xo" && len(parts) >= 3 {
		return parts[0] + "." + parts[1], true
	}
	return parts[0], true
}

// Glue is the group of the compiler's generated drop, share, dup and eq
// functions (drop_20Str is "drop Str" in the mangling): they belong to no
// module and there are dozens of them, so they are counted together.
const Glue = "glue (drop/share/eq)"

// gluePrefixes start a generated function's mangled name, followed by its
// type argument (_20 is the separating space).
var gluePrefixes = []string{"drop_20", "dropp_20", "dup_20", "dupp_20",
	"share_20", "sharep_20", "envdrop_20", "envshare_20", "dropm_20", "boxdrop_20", "eq_20"}

// isGlue reports whether a symbol's text after the xo prefix is one of the
// generated drop, share, dup or eq functions.
func isGlue(rest string) bool {
	for _, p := range gluePrefixes {
		if strings.HasPrefix(rest, p) {
			return true
		}
	}
	return false
}

// groupOf is the group a symbol belongs to: its module, Glue, Hidden or
// NonXo.
func groupOf(name string) string {
	if strings.HasPrefix(name, xoPrefix) {
		if isGlue(name[len(xoPrefix):]) {
			return Glue
		}
		if m, ok := moduleOf(name); ok {
			return m
		}
	}
	if strings.HasPrefix(name, "_xo_h") {
		return Hidden
	}
	return NonXo
}

// mapFamilies are the three implementations of Map (std/native/llrt/map*).
var mapFamilies = map[string]bool{"xo.map": true, "xo.mlog": true, "xo.maph": true}

// demangle undoes the _XX escapes of a symbol name: _2e is '.', _5b is '[',
// _20 is space, and so on. An underscore not followed by two hex digits is
// literal (module names such as cabi__tls keep theirs).
func demangle(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == '_' && i+2 < len(s) && isHex(s[i+1]) && isHex(s[i+2]) {
			b.WriteByte(hexVal(s[i+1])<<4 | hexVal(s[i+2]))
			i += 2
			continue
		}
		b.WriteByte(s[i])
	}
	return b.String()
}

func isHex(c byte) bool {
	return c >= '0' && c <= '9' || c >= 'a' && c <= 'f'
}

func hexVal(c byte) byte {
	if c <= '9' {
		return c - '0'
	}
	return c - 'a' + 10
}

// mapType is the element type of the first Map[...] in a demangled name:
// mlog's symbols carry xo.MapLog[Map[K, V]], maph's Map[K, V], and so on.
func mapType(demangled string) string {
	i := strings.Index(demangled, "Map[")
	if i < 0 {
		return ""
	}
	depth := 0
	for j := i + 3; j < len(demangled); j++ {
		switch demangled[j] {
		case '[':
			depth++
		case ']':
			depth--
			if depth == 0 {
				return demangled[i : j+1]
			}
		}
	}
	return demangled[i:]
}

// displayName is a symbol as a reader knows it: _xo_f_aesgcm_2eGcm_2egcm__bulk
// is aesgcm.Gcm.gcm__bulk. Only the part after the module separator is
// unescaped: a module name such as aesgcm starts with two hex letters, and
// unescaping it as one string would read "_ae" as a byte.
func displayName(name string) string {
	if !strings.HasPrefix(name, xoPrefix) {
		return name
	}
	rest := name[len(xoPrefix):]
	if isGlue(rest) && !strings.Contains(rest, "_2e") {
		return demangle(rest)
	}
	parts := strings.Split(rest, "_2e")
	if parts[0] == "xo" && len(parts) >= 3 {
		return "xo." + parts[1] + "." + demangle(strings.Join(parts[2:], "_2e"))
	}
	if len(parts) == 1 {
		return parts[0]
	}
	return parts[0] + "." + demangle(strings.Join(parts[1:], "_2e"))
}

// analyze fills sizes from the gaps between sorted symbols and groups them.
// It fails when the gaps do not cover the text section exactly, which is the
// check that says the symbol table and the section agree.
func analyze(bin *execFile) (*Report, error) {
	text := bin.Text
	syms := make([]Symbol, len(bin.Syms))
	copy(syms, bin.Syms)
	sort.Slice(syms, func(i, j int) bool {
		if syms[i].Addr != syms[j].Addr {
			return syms[i].Addr < syms[j].Addr
		}
		return syms[i].Name < syms[j].Name
	})
	// One size per address; a second symbol at the same address is an alias
	// (the compiler emits them for generic instantiations that coincide).
	uniq := syms[:0]
	aliases := 0
	for _, s := range syms {
		if s.Addr < text.Addr || s.Addr >= text.Addr+text.Size {
			continue
		}
		if n := len(uniq); n > 0 && uniq[n-1].Addr == s.Addr {
			aliases++
			continue
		}
		uniq = append(uniq, s)
	}
	var sum uint64
	for i := range uniq {
		end := text.Addr + text.Size
		if i+1 < len(uniq) {
			end = uniq[i+1].Addr
		}
		uniq[i].Size = end - uniq[i].Addr
		sum += uniq[i].Size
	}
	if sum != text.Size {
		return nil, fmt.Errorf("%s: symbols cover %d of the %d byte text section (difference %d)",
			bin.Name, sum, text.Size, int64(text.Size)-int64(sum))
	}

	rep := &Report{
		Binary:  bin.Name,
		Format:  bin.Format,
		Text:    text,
		Sum:     sum,
		Symbols: len(uniq),
		Aliases: aliases,
	}
	groups := map[string]*Group{}
	fams := map[string]*Family{}
	insts := map[string]map[string]*Instantiation{}
	for _, s := range uniq {
		g := groupOf(s.Name)
		gr := groups[g]
		if gr == nil {
			gr = &Group{Name: g}
			groups[g] = gr
		}
		gr.Bytes += s.Size
		gr.Symbols++
		if mapFamilies[g] {
			f := fams[g]
			if f == nil {
				f = &Family{Name: g}
				fams[g] = f
				insts[g] = map[string]*Instantiation{}
			}
			f.Bytes += s.Size
			f.Symbols++
			t := mapType(demangle(s.Name))
			if t == "" {
				t = "(nongeneric)"
			}
			in := insts[g][t]
			if in == nil {
				in = &Instantiation{Type: t}
				insts[g][t] = in
			}
			in.Bytes += s.Size
			in.Symbols++
		}
	}
	for _, g := range groups {
		g.Share = float64(g.Bytes) / float64(text.Size)
		rep.Groups = append(rep.Groups, *g)
	}
	sort.Slice(rep.Groups, func(i, j int) bool {
		if rep.Groups[i].Bytes != rep.Groups[j].Bytes {
			return rep.Groups[i].Bytes > rep.Groups[j].Bytes
		}
		return rep.Groups[i].Name < rep.Groups[j].Name
	})
	for _, f := range fams {
		for _, in := range insts[f.Name] {
			f.Top = append(f.Top, *in)
		}
		sort.Slice(f.Top, func(i, j int) bool {
			if f.Top[i].Bytes != f.Top[j].Bytes {
				return f.Top[i].Bytes > f.Top[j].Bytes
			}
			return f.Top[i].Type < f.Top[j].Type
		})
		if len(f.Top) > 10 {
			f.Top = f.Top[:10]
		}
		rep.Families = append(rep.Families, *f)
	}
	sort.Slice(rep.Families, func(i, j int) bool { return rep.Families[i].Bytes > rep.Families[j].Bytes })

	top := make([]Symbol, len(uniq))
	copy(top, uniq)
	sort.Slice(top, func(i, j int) bool {
		if top[i].Size != top[j].Size {
			return top[i].Size > top[j].Size
		}
		return top[i].Name < top[j].Name
	})
	rep.Top = top
	return rep, nil
}

// printReport writes the human report.
func printReport(w io.Writer, r *Report) {
	fmt.Fprintf(w, "%s: %s, __text %d B at 0x%x, %d symbols", r.Binary, r.Format, r.Text.Size, r.Text.Addr, r.Symbols)
	if r.Aliases > 0 {
		fmt.Fprintf(w, " (%d aliases)", r.Aliases)
	}
	fmt.Fprintln(w)
	fmt.Fprintf(w, "symbol gaps cover %d of %d bytes\n\n", r.Sum, r.Text.Size)

	fmt.Fprintln(w, "groups:")
	for _, g := range r.Groups {
		fmt.Fprintf(w, "  %8d  %5.1f%%  %-24s %d syms\n", g.Bytes, 100*g.Share, g.Name, g.Symbols)
	}
	if len(r.Families) > 0 {
		fmt.Fprintln(w, "\nMap families:")
		var total uint64
		for _, f := range r.Families {
			total += f.Bytes
			fmt.Fprintf(w, "  %8d  %-10s %d syms\n", f.Bytes, f.Name, f.Symbols)
		}
		fmt.Fprintf(w, "  %8d  %-10s (all three)\n", total, "total")
		for _, f := range r.Families {
			fmt.Fprintf(w, "\n  %s:", f.Name)
			for _, in := range f.Top {
				fmt.Fprintf(w, "\n    %8d  %4d syms  %s", in.Bytes, in.Symbols, in.Type)
			}
			fmt.Fprintln(w)
		}
	}
	fmt.Fprintf(w, "\ntop %d symbols:\n", len(r.Top))
	for _, s := range r.Top {
		fmt.Fprintf(w, "  %8d  %-48s [%s]\n", s.Size, displayName(s.Name), groupOf(s.Name))
	}
	if r.Reach != nil {
		printReach(w, r.Reach)
	}
}
