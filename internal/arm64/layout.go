package arm64

import "github.com/xo-lang/xo/internal/ssa"

// Value layout: every SSA value is a sequence of 64 bit words, each in a
// general purpose (false) or floating point (true) register. In memory
// (list elements, and the values the reference counting helpers read) a
// word takes 8 bytes at offset 8*k, except a list element of type Bool,
// which is one byte.
//
//	Int, Bool, List   one integer word (a Bool is 0 or 1)
//	Float             one floating point word
//	Str               two integer words: the object and the length
//	function value    two integer words: the code and the environment
//	                  (closures.go)
//	Box[T]            one integer word: the cell (boxes.go)
//	struct, tuple     the words of the fields, in order
//	enum              the tag, then W integer payload words, W the most
//	                  words of any variant; a variant's fields are laid
//	                  out from the first payload word, Float fields as
//	                  their bits
//	(), capabilities  no words

// layout returns the word classes of t.
func (g *gen) layout(t *ssa.Type) []bool {
	return g.layoutPath(t, map[*ssa.Type]bool{})
}

func (g *gen) layoutPath(t *ssa.Type, path map[*ssa.Type]bool) []bool {
	if l, ok := g.layouts[t]; ok {
		return l
	}
	if path[t] {
		fail("recursive type `%s` (not boxed)", t.Name)
	}
	path[t] = true
	defer delete(path, t)
	var l []bool
	switch t.Kind {
	case ssa.KUnit, ssa.KCap:
		l = []bool{}
	case ssa.KInt, ssa.KBool, ssa.KList, ssa.KBox:
		// a List or Box is a pointer: its element may contain the type
		// that contains it
		l = []bool{false}
	case ssa.KFloat:
		l = []bool{true}
	case ssa.KStr, ssa.KFunc:
		l = []bool{false, false}
	case ssa.KStruct:
		for _, f := range t.Fields {
			l = append(l, g.layoutPath(f.Type, path)...)
		}
	case ssa.KTuple:
		for _, e := range t.Elems {
			l = append(l, g.layoutPath(e, path)...)
		}
	case ssa.KEnum:
		w := 0
		for _, v := range t.Variants {
			n := 0
			for _, f := range v.Fields {
				n += len(g.layoutPath(f.Type, path))
			}
			if n > w {
				w = n
			}
		}
		l = make([]bool, 1+w)
	default:
		fail("type `%s`", t.Name)
	}
	if l == nil {
		l = []bool{}
	}
	g.layouts[t] = l
	return l
}

func (g *gen) nwords(t *ssa.Type) int { return len(g.layout(t)) }

// fieldWords returns the first word and the word count of field i of a
// struct or tuple.
func (g *gen) fieldWords(t *ssa.Type, i int) (int, int) {
	var ts []*ssa.Type
	if t.Kind == ssa.KStruct {
		for _, f := range t.Fields {
			ts = append(ts, f.Type)
		}
	} else {
		ts = t.Elems
	}
	off := 0
	for k := 0; k < i; k++ {
		off += g.nwords(ts[k])
	}
	return off, g.nwords(ts[i])
}

// payloadWords returns the first word (counting the tag as word 0) and
// the word count of field fi of variant tag of enum t.
func (g *gen) payloadWords(t *ssa.Type, tag, fi int) (int, int) {
	off := 1
	fs := t.Variants[tag].Fields
	for k := 0; k < fi; k++ {
		off += g.nwords(fs[k].Type)
	}
	return off, g.nwords(fs[fi].Type)
}

// esize is the size in bytes of a list element of type t.
func (g *gen) esize(t *ssa.Type) int64 {
	if t.Kind == ssa.KBool {
		return 1
	}
	return 8 * int64(g.nwords(t))
}

// heapLeaves returns the word offsets and types of the heap leaves of a
// value of type t (a Str, a List, or an enum with heap payload) that
// reference counting walks, through structs and tuples.
func (g *gen) heapLeaves(t *ssa.Type, base int, out *[]leaf) {
	if !t.Heap() {
		return
	}
	switch t.Kind {
	case ssa.KStr, ssa.KList, ssa.KEnum, ssa.KFunc, ssa.KBox:
		*out = append(*out, leaf{base, t})
	case ssa.KStruct, ssa.KTuple:
		n := len(t.Elems)
		if t.Kind == ssa.KStruct {
			n = len(t.Fields)
		}
		for i := 0; i < n; i++ {
			off, _ := g.fieldWords(t, i)
			ft := t.Elems
			var et *ssa.Type
			if t.Kind == ssa.KStruct {
				et = t.Fields[i].Type
			} else {
				et = ft[i]
			}
			g.heapLeaves(et, base+off, out)
		}
	}
}

type leaf struct {
	off int
	t   *ssa.Type
}
