package cbind

import (
	"fmt"
	"strings"
)

// ctype is a C type in canonical form: a base type with qualifiers
// removed and typedefs resolved, and a pointer depth.
type ctype struct {
	base      string // "int", "unsigned long", "struct tiny_point", "enum mode", "void"
	ptr       int
	constBase bool // the pointee (or the value, with ptr 0) is const
}

func (t ctype) String() string {
	s := t.base
	if t.constBase {
		s = "const " + s
	}
	if t.ptr > 0 {
		s += " " + strings.Repeat("*", t.ptr)
	}
	return s
}

var qualifiers = map[string]bool{
	"const": true, "volatile": true, "restrict": true, "__restrict": true, "__restrict__": true,
	"_Nullable": true, "_Nonnull": true, "_Null_unspecified": true, "__unaligned": true,
	"_Atomic": false,
}

// builtinBase normalizes the spelling of a builtin type, or returns "".
func builtinBase(words []string) string {
	s := strings.Join(words, " ")
	switch s {
	case "void", "_Bool", "bool", "float", "double", "long double":
		if s == "bool" {
			return "_Bool"
		}
		return s
	case "char", "signed char", "unsigned char":
		return s
	}
	signed, unsigned := false, false
	var rest []string
	for _, w := range words {
		switch w {
		case "signed":
			signed = true
		case "unsigned":
			unsigned = true
		default:
			rest = append(rest, w)
		}
	}
	r := strings.Join(rest, " ")
	switch r {
	case "", "int":
		if !signed && !unsigned && r == "" {
			return ""
		}
		r = "int"
	case "short", "short int":
		r = "short"
	case "long", "long int":
		r = "long"
	case "long long", "long long int":
		r = "long long"
	case "char":
		if unsigned {
			return "unsigned char"
		}
		return "signed char"
	case "__int128":
		r = "__int128"
	default:
		return ""
	}
	if unsigned {
		return "unsigned " + r
	}
	return r
}

// parseType turns clang's spelling of a type into a ctype, resolving
// typedef names through typedefs.
func (b *binder) parseType(q string) (ctype, error) {
	return b.parseTypeDepth(q, 0)
}

func (b *binder) parseTypeDepth(q string, depth int) (ctype, error) {
	if depth > 20 {
		return ctype{}, fmt.Errorf("typedef cycle at %s", q)
	}
	q = strings.TrimSpace(q)
	if strings.ContainsAny(q, "([") {
		return ctype{}, fmt.Errorf("function pointer or array type %s", q)
	}
	parts := strings.Split(q, "*")
	t := ctype{ptr: len(parts) - 1}
	var words []string
	for _, w := range strings.Fields(parts[0]) {
		if w == "const" {
			t.constBase = true
			continue
		}
		if qv, ok := qualifiers[w]; ok {
			if !qv {
				return ctype{}, fmt.Errorf("atomic type %s", strings.TrimSpace(parts[0]))
			}
			continue
		}
		words = append(words, w)
	}
	if len(words) == 0 {
		return ctype{}, fmt.Errorf("type %q", q)
	}
	switch words[0] {
	case "struct", "union", "enum":
		if len(words) != 2 {
			return ctype{}, fmt.Errorf("anonymous %s type", words[0])
		}
		t.base = words[0] + " " + words[1]
		return t, nil
	}
	if bb := builtinBase(words); bb != "" {
		t.base = bb
		return t, nil
	}
	if len(words) == 1 {
		if under, ok := b.typedefs[words[0]]; ok {
			u, err := b.parseTypeDepth(under, depth+1)
			if err != nil {
				return ctype{}, err
			}
			u.ptr += t.ptr
			if t.ptr == 0 || u.ptr == t.ptr {
				// `const T` where T is a typedef of a value or of the
				// pointee (with typedef pointers, const stays outermost)
				u.constBase = u.constBase || t.constBase
			}
			return u, nil
		}
	}
	return ctype{}, fmt.Errorf("unknown type %s", strings.Join(words, " "))
}

// scalar describes a C value type that crosses as one machine word.
type scalar struct {
	xo    string // the Xo type
	c     string // the C type written in the adapter
	float bool   // passed as a double
}

// scalarOf maps a C value type (pointer depth 0) to its Xo scalar.
func scalarOf(t ctype) (scalar, bool) {
	if t.ptr != 0 {
		return scalar{}, false
	}
	switch t.base {
	case "_Bool":
		return scalar{"Bool", "_Bool", false}, true
	case "char":
		return scalar{"Int8", "char", false}, true
	case "signed char":
		return scalar{"Int8", "signed char", false}, true
	case "unsigned char":
		return scalar{"UInt8", "unsigned char", false}, true
	case "short":
		return scalar{"Int16", "short", false}, true
	case "unsigned short":
		return scalar{"UInt16", "unsigned short", false}, true
	case "int":
		return scalar{"Int32", "int", false}, true
	case "unsigned int":
		return scalar{"UInt32", "unsigned int", false}, true
	case "long":
		return scalar{"Int", "long", false}, true
	case "long long":
		return scalar{"Int", "long long", false}, true
	case "unsigned long":
		return scalar{"UInt64", "unsigned long", false}, true
	case "unsigned long long":
		return scalar{"UInt64", "unsigned long long", false}, true
	case "float":
		return scalar{"Float32", "float", true}, true
	case "double":
		return scalar{"Float", "double", true}, true
	}
	if strings.HasPrefix(t.base, "enum ") {
		return scalar{"Int32", t.base, false}, true
	}
	return scalar{}, false
}

// isByteBase reports a base whose pointer is a byte buffer.
func isByteBase(base string) bool {
	switch base {
	case "void", "char", "signed char", "unsigned char":
		return true
	}
	return false
}

// isIntBase reports an integer base type (a buffer length).
func isIntBase(t ctype) bool {
	s, ok := scalarOf(t)
	return ok && !s.float && s.xo != "Bool" && !strings.HasPrefix(t.base, "enum ")
}
