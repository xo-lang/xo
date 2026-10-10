package lsp

import (
	"net/url"
	"path/filepath"
	"unicode/utf16"
	"unicode/utf8"
)

// text is a source file with its line starts, for converting between the
// checker's 1 based line and byte column and LSP's 0 based line and UTF-16
// character.
type text struct {
	src   []byte
	lines []int // byte offset of the start of each line
}

func newText(src []byte) *text {
	t := &text{src: src, lines: []int{0}}
	for i, c := range src {
		if c == '\n' {
			t.lines = append(t.lines, i+1)
		}
	}
	return t
}

func (t *text) lineBytes(line int) []byte {
	if line < 0 || line >= len(t.lines) {
		return nil
	}
	end := len(t.src)
	if line+1 < len(t.lines) {
		end = t.lines[line+1] - 1
	}
	b := t.src[t.lines[line]:end]
	if n := len(b); n > 0 && b[n-1] == '\r' {
		b = b[:n-1]
	}
	return b
}

// position converts a 1 based line and byte column.
func (t *text) position(line, col int) position {
	l := line - 1
	b := t.lineBytes(l)
	n := col - 1
	if n > len(b) {
		n = len(b)
	}
	if n < 0 {
		n = 0
	}
	return position{Line: max(l, 0), Character: utf16Len(b[:n])}
}

// offset converts an LSP position to a byte offset into src.
func (t *text) offset(p position) int {
	if p.Line >= len(t.lines) {
		return len(t.src)
	}
	b := t.lineBytes(p.Line)
	units, i := 0, 0
	for i < len(b) && units < p.Character {
		r, size := utf8.DecodeRune(b[i:])
		units += utf16.RuneLen(r)
		if units > p.Character {
			break
		}
		i += size
	}
	return t.lines[p.Line] + i
}

// col converts an LSP position to the checker's 1 based line and byte column.
func (t *text) col(p position) (line, col int) {
	off := t.offset(p)
	return p.Line + 1, off - t.lines[min(p.Line, len(t.lines)-1)] + 1
}

// offsetPosition converts a byte offset.
func (t *text) offsetPosition(off int) position {
	lo, hi := 0, len(t.lines)-1
	for lo < hi {
		mid := (lo + hi + 1) / 2
		if t.lines[mid] <= off {
			lo = mid
		} else {
			hi = mid - 1
		}
	}
	return t.position(lo+1, off-t.lines[lo]+1)
}

// end is the position after the last byte.
func (t *text) end() position { return t.offsetPosition(len(t.src)) }

func utf16Len(b []byte) int {
	n := 0
	for len(b) > 0 {
		r, size := utf8.DecodeRune(b)
		n += utf16.RuneLen(r)
		b = b[size:]
	}
	return n
}

func uriToPath(uri string) string {
	u, err := url.Parse(uri)
	if err != nil || u.Scheme != "file" {
		return ""
	}
	return filepath.Clean(filepath.FromSlash(u.Path))
}

func pathToURI(path string) string {
	return (&url.URL{Scheme: "file", Path: filepath.ToSlash(path)}).String()
}
