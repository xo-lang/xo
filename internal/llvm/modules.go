package llvm

import (
	"bytes"
	"sort"
)

// Split divides the textual IR of a program (Generate's output) into at
// most n modules that clang compiles separately and in parallel, then
// links (decision 0168). The functions of the program (x.*, and
// xo_entry) are spread over the modules by size, largest first onto the
// lightest module; every module gets
//
//   - the whole header: type definitions, runtime declarations,
//     attribute groups, and metadata (unused ones cost nothing);
//   - a private copy of every constant it uses;
//   - an internal copy of every helper it uses (xo.dup.*, xo.drop.*,
//     xo.eq.*, ...: small, mostly alwaysinline, so each module inlines
//     them as the single module does);
//   - a `declare hidden` of every function of another module it uses.
//
// A function another module uses changes from internal to hidden linkage
// (visible across the modules, not exported from the binary). The
// modules of the same IR and n are the same bytes, so a binary built
// from them stays reproducible. n <= 1, or IR with fewer than two
// functions, gives the IR back as one module.
func Split(ir []byte, n int) [][]byte {
	if n <= 1 {
		return [][]byte{ir}
	}
	p := parseIR(ir)
	var fns []int // indexes into p.defs of the functions placed
	for i, d := range p.defs {
		if !d.helper {
			fns = append(fns, i)
		}
	}
	if len(fns) < 2 {
		return [][]byte{ir}
	}
	n = min(n, len(fns))
	// largest first onto the lightest module (ties: earlier module, and
	// the order of the IR, so the split is deterministic)
	sort.SliceStable(fns, func(a, b int) bool { return p.defs[fns[a]].size() > p.defs[fns[b]].size() })
	home := make([]int, len(p.defs))
	for i := range home {
		home[i] = -1
	}
	load := make([]int, n)
	for _, i := range fns {
		m := 0
		for k := 1; k < n; k++ {
			if load[k] < load[m] {
				m = k
			}
		}
		home[i] = m
		load[m] += p.defs[i].size()
	}
	// each module's functions, in the order of the IR
	members := make([][]int, n)
	for i := range p.defs {
		if home[i] >= 0 {
			members[home[i]] = append(members[home[i]], i)
		}
	}
	// what each module uses: its functions' references, closed over the
	// constants and helpers it copies
	uses := make([]map[string]bool, n)
	exported := map[int]bool{} // functions some other module calls
	for m := range n {
		u := map[string]bool{}
		var work []string
		add := func(refs []string) {
			for _, r := range refs {
				if !u[r] {
					u[r] = true
					work = append(work, r)
				}
			}
		}
		for _, i := range members[m] {
			add(p.defs[i].refs)
		}
		for len(work) > 0 {
			r := work[len(work)-1]
			work = work[:len(work)-1]
			if g, ok := p.globals[r]; ok {
				add(p.globalItems[g].refs)
			} else if i, ok := p.byName[r]; ok && p.defs[i].helper {
				add(p.defs[i].refs)
			}
		}
		for r := range u {
			if i, ok := p.byName[r]; ok && !p.defs[i].helper && home[i] != m {
				exported[i] = true
			}
		}
		uses[m] = u
	}
	out := make([][]byte, n)
	for m := range n {
		var b bytes.Buffer
		b.Write(p.header)
		b.WriteString("\n")
		for _, g := range p.globalItems {
			if uses[m][g.name] {
				b.Write(g.text)
			}
		}
		b.WriteString("\n")
		for i, d := range p.defs {
			switch {
			case !uses[m][d.name] || home[i] == m:
			case d.helper:
				b.Write(d.text)
			default:
				b.Write(d.decl)
			}
		}
		b.WriteString("\n")
		for _, i := range members[m] {
			d := p.defs[i]
			if exported[i] && bytes.HasPrefix(d.text, []byte("define internal ")) {
				b.WriteString("define hidden ")
				b.Write(d.text[len("define internal "):])
			} else {
				b.Write(d.text)
			}
		}
		out[m] = b.Bytes()
	}
	return out
}

// irItem is a top-level definition of the IR: a function or a constant.
type irItem struct {
	name   string // the symbol, with its @ and quotes as written
	text   []byte // the definition, through its newline
	refs   []string
	helper bool   // a function the modules copy (xo.* helpers)
	decl   []byte // the declaration of a function, for other modules
}

func (d *irItem) size() int { return len(d.text) }

// parsedIR is the IR cut into its header, constants, and functions.
type parsedIR struct {
	header      []byte
	globalItems []*irItem
	globals     map[string]int // name -> index into globalItems
	defs        []*irItem
	byName      map[string]int // name -> index into defs
}

// parseIR cuts IR as Generate writes it: top-level entities start at the
// beginning of a line, function bodies end at a line "}", constants are
// one line.
func parseIR(ir []byte) *parsedIR {
	p := &parsedIR{globals: map[string]int{}, byName: map[string]int{}}
	var header bytes.Buffer
	for len(ir) > 0 {
		line := nextLine(ir)
		switch {
		case bytes.HasPrefix(line, []byte("define ")):
			end := bytes.Index(ir, []byte("\n}\n"))
			if end < 0 {
				end = len(ir) - 3
			}
			text := ir[:end+3]
			ir = ir[end+3:]
			// a blank line after a body goes with it
			for len(ir) > 0 && ir[0] == '\n' {
				text = text[:len(text)+1]
				ir = ir[1:]
			}
			name := defSymbol(line)
			d := &irItem{name: name, text: text, helper: bytes.HasPrefix([]byte(name), []byte(`@"xo.`))}
			d.refs = symbolRefs(text[len(line):], name)
			d.refs = append(d.refs, symbolRefs(line[bytes.IndexByte(line, '('):], name)...)
			d.decl = declOf(line)
			p.byName[name] = len(p.defs)
			p.defs = append(p.defs, d)
		case len(line) > 0 && line[0] == '@':
			ir = ir[len(line):]
			name := symbolAt(line, 0)
			g := &irItem{name: name, text: line}
			g.refs = symbolRefs(line[len(name):], name)
			p.globals[name] = len(p.globalItems)
			p.globalItems = append(p.globalItems, g)
		default:
			ir = ir[len(line):]
			header.Write(line)
		}
	}
	p.header = header.Bytes()
	return p
}

// nextLine returns the first line of b with its newline.
func nextLine(b []byte) []byte {
	if i := bytes.IndexByte(b, '\n'); i >= 0 {
		return b[:i+1]
	}
	return b
}

// defSymbol is the symbol a define line defines.
func defSymbol(line []byte) string {
	i := bytes.IndexByte(line, '@')
	return symbolAt(line, i)
}

// symbolAt reads the symbol starting at the @ at b[i]: @"quoted" or
// @plain.
func symbolAt(b []byte, i int) string {
	j := i + 1
	if j < len(b) && b[j] == '"' {
		k := bytes.IndexByte(b[j+1:], '"')
		return string(b[i : j+1+k+1])
	}
	for j < len(b) && isSymChar(b[j]) {
		j++
	}
	return string(b[i:j])
}

func isSymChar(c byte) bool {
	return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '_' || c == '.' || c == '$' || c == '-'
}

// symbolRefs lists the symbols b mentions, other than self, skipping
// string literals, quoted local and type names, and comments.
func symbolRefs(b []byte, self string) []string {
	seen := map[string]bool{self: true}
	var out []string
	for i := 0; i < len(b); i++ {
		switch b[i] {
		case '"':
			k := bytes.IndexByte(b[i+1:], '"')
			if k < 0 {
				return out
			}
			i += k + 1
		case ';':
			k := bytes.IndexByte(b[i:], '\n')
			if k < 0 {
				return out
			}
			i += k
		case '@':
			s := symbolAt(b, i)
			i += len(s) - 1
			if len(s) > 1 && !seen[s] {
				seen[s] = true
				out = append(out, s)
			}
		}
	}
	return out
}

// declOf turns a define line into the declaration other modules use:
// hidden linkage, the parameter list, and the attribute groups, without
// the personality and the body's brace.
func declOf(line []byte) []byte {
	rest := line[len("define "):]
	rest = bytes.TrimPrefix(rest, []byte("internal "))
	rest = bytes.TrimPrefix(rest, []byte("hidden "))
	// the parameter list after the name: skip quoted names when matching
	// the parentheses
	at := bytes.IndexByte(rest, '@')
	name := symbolAt(rest, at)
	i := at + len(name)
	depth := 0
	for ; i < len(rest); i++ {
		c := rest[i]
		if c == '"' {
			i += bytes.IndexByte(rest[i+1:], '"') + 1
			continue
		}
		if c == '(' {
			depth++
		} else if c == ')' {
			depth--
			if depth == 0 {
				break
			}
		}
	}
	var b bytes.Buffer
	b.WriteString("declare hidden ")
	b.Write(rest[:i+1])
	for _, f := range bytes.Fields(rest[i+1:]) {
		if f[0] == '#' {
			b.WriteByte(' ')
			b.Write(f)
		}
	}
	b.WriteByte('\n')
	return b.Bytes()
}
