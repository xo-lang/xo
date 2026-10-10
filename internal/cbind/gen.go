package cbind

import (
	"errors"
	"fmt"
	"sort"
	"strings"
)

// Parameter and result kinds at the boundary.
const (
	kScalar = "scalar" // one word: integer, Bool, Float
	kStr    = "str"    // const char *, NUL terminated: copied in, copied out
	kStrN   = "strn"   // const char * and a length: Str, not copied
	kBytes  = "bytes"  // const byte pointer and a length: Bytes, not copied
	kHandle = "handle" // pointer to a struct the header does not define
	kStruct = "struct" // a flat struct of scalars, by value: copied
	kVoid   = "void"
)

type flat struct {
	xo, tag string
	fields  []flatField
}

type flatField struct {
	xo, c string
	sc    scalar
}

type cparam struct {
	xo     string // Xo parameter name
	cname  string
	kind   string
	ct     ctype
	sc     scalar
	lenCT  ctype // kStrN, kBytes: the length parameter's type
	handle string
	st     *flat
}

type cret struct {
	kind   string
	ct     ctype
	sc     scalar
	handle string
	st     *flat
}

type cfunc struct {
	xo, c  string
	sig    string
	params []*cparam
	ret    *cret
	replay string
}

func typeOf(n *obj) string {
	t := n.obj("type")
	if t == nil {
		return ""
	}
	if q := t.str("desugaredQualType"); q != "" {
		return q
	}
	return t.str("qualType")
}

// lengthName reports a parameter name that reads as a buffer length.
func lengthName(n string) bool {
	l := strings.ToLower(n)
	return l == "n" || strings.Contains(l, "len") || strings.Contains(l, "size") || strings.Contains(l, "count") || strings.HasPrefix(l, "n_") || l == "nbytes"
}

func (b *binder) function(n *obj) (*cfunc, error) {
	cname := n.str("name")
	if strings.HasPrefix(cname, "_") {
		return nil, errors.New("reserved name")
	}
	if n.flag("variadic") {
		return nil, errors.New("variadic function")
	}
	f := &cfunc{c: cname, sig: cSignature(n), replay: "value"}
	f.xo = xoName(snake(cname))
	if b.used[f.xo] {
		return nil, fmt.Errorf("its Xo name %s is taken", f.xo)
	}
	q := ""
	if t := n.obj("type"); t != nil {
		q = t.str("qualType")
	}
	i := strings.Index(q, "(")
	if i < 0 {
		return nil, fmt.Errorf("function type %s", q)
	}
	rt, err := b.parseType(q[:i])
	if err != nil {
		return nil, fmt.Errorf("result: %v", err)
	}
	ret, err := b.result(rt)
	if err != nil {
		return nil, fmt.Errorf("result: %v", err)
	}
	f.ret = ret
	var ps []*obj
	for _, e := range n.list("inner") {
		if p, ok := e.(*obj); ok && p.str("kind") == "ParmVarDecl" {
			ps = append(ps, p)
		}
	}
	names := map[string]bool{}
	for k := 0; k < len(ps); k++ {
		p := ps[k]
		pn := p.str("name")
		ct, err := b.parseType(typeOf(p))
		if err != nil {
			return nil, fmt.Errorf("parameter %s: %v", pn, err)
		}
		cp := &cparam{cname: pn, ct: ct}
		switch {
		case ct.ptr == 0:
			if sc, ok := scalarOf(ct); ok {
				cp.kind, cp.sc = kScalar, sc
			} else if st, err := b.flatStruct(ct); err == nil {
				cp.kind, cp.st = kStruct, st
			} else {
				return nil, fmt.Errorf("parameter %s: %v", pn, err)
			}
		case ct.ptr == 1 && isByteBase(ct.base) && ct.constBase:
			var next ctype
			if k+1 < len(ps) {
				next, err = b.parseType(typeOf(ps[k+1]))
			}
			if k+1 < len(ps) && err == nil && isIntBase(next) && lengthName(ps[k+1].str("name")) {
				cp.kind, cp.lenCT = kBytes, next
				if ct.base == "char" {
					cp.kind = kStrN
				}
				k++
			} else if ct.base == "char" {
				cp.kind = kStr
			} else {
				return nil, fmt.Errorf("parameter %s: %s without a length parameter after it", pn, ct)
			}
		case ct.ptr == 1 && strings.HasPrefix(ct.base, "struct "):
			h, err := b.handle(ct)
			if err != nil {
				return nil, fmt.Errorf("parameter %s: %v", pn, err)
			}
			cp.kind, cp.handle = kHandle, h
			f.replay = "handle"
		default:
			return nil, fmt.Errorf("parameter %s: %s (a pointer Xo cannot fill: an output parameter or a mutable buffer)", pn, ct)
		}
		x := xoName(snake(pn))
		if x == "" || x == "_" || names[x] {
			x = fmt.Sprintf("a%d", len(f.params)+1)
		}
		names[x] = true
		cp.xo = x
		f.params = append(f.params, cp)
	}
	if ret.kind == kHandle {
		f.replay = "handle"
	}
	b.used[f.xo] = true
	return f, nil
}

func (b *binder) result(t ctype) (*cret, error) {
	switch {
	case t.ptr == 0 && t.base == "void":
		return &cret{kind: kVoid, ct: t}, nil
	case t.ptr == 0:
		if sc, ok := scalarOf(t); ok {
			return &cret{kind: kScalar, ct: t, sc: sc}, nil
		}
		st, err := b.flatStruct(t)
		if err != nil {
			return nil, err
		}
		return &cret{kind: kStruct, ct: t, st: st}, nil
	case t.ptr == 1 && t.base == "char" && t.constBase:
		return &cret{kind: kStr, ct: t}, nil
	case t.ptr == 1 && t.base == "char":
		return nil, errors.New("char * (who frees it is not known: bind a wrapper that returns const char * or copies into a buffer)")
	case t.ptr == 1 && strings.HasPrefix(t.base, "struct "):
		h, err := b.handle(t)
		if err != nil {
			return nil, err
		}
		return &cret{kind: kHandle, ct: t, handle: h}, nil
	}
	return nil, fmt.Errorf("%s", t)
}

// handle is the Xo handle type of a pointer to a struct the header does
// not define.
func (b *binder) handle(t ctype) (string, error) {
	tag := strings.TrimPrefix(t.base, "struct ")
	if r := b.records[tag]; r != nil && r.complete {
		return "", fmt.Errorf("pointer to %s, which the header defines: Xo values never share C memory of a known layout (bind a wrapper that keeps the struct behind an opaque pointer)", t.base)
	}
	if h, ok := b.handles[tag]; ok {
		return h, nil
	}
	h := upperCamel(tag)
	for b.used[h] {
		h += "_"
	}
	b.used[h] = true
	b.handles[tag] = h
	return h, nil
}

// flatStruct is the copied Xo struct of a C struct of scalars.
func (b *binder) flatStruct(t ctype) (*flat, error) {
	if !strings.HasPrefix(t.base, "struct ") || t.ptr != 0 {
		return nil, fmt.Errorf("%s", t)
	}
	tag := strings.TrimPrefix(t.base, "struct ")
	r := b.records[tag]
	if r == nil || !r.complete {
		return nil, fmt.Errorf("%s by value, which the header does not define", t.base)
	}
	if len(r.fields) == 0 {
		return nil, fmt.Errorf("%s has no fields", t.base)
	}
	st := &flat{tag: tag}
	seen := map[string]bool{}
	for _, fd := range r.fields {
		ft, err := b.parseType(fd.t)
		if err != nil {
			return nil, fmt.Errorf("%s field %s: %v", t.base, fd.name, err)
		}
		sc, ok := scalarOf(ft)
		if !ok || fd.name == "" {
			return nil, fmt.Errorf("%s field %s is %s, not a scalar (only flat structs of numbers and Bools are copied)", t.base, fd.name, ft)
		}
		x := xoName(snake(fd.name))
		if seen[x] {
			return nil, fmt.Errorf("%s: two fields named %s in Xo", t.base, x)
		}
		seen[x] = true
		st.fields = append(st.fields, flatField{xo: x, c: fd.name, sc: sc})
	}
	if x, ok := b.structs[tag]; ok {
		st.xo = x
		return st, nil
	}
	x := upperCamel(tag)
	for b.used[x] {
		x += "_"
	}
	b.used[x] = true
	b.structs[tag] = x
	st.xo = x
	return st, nil
}

// ---------------------------------------------------------------------------
// The declaration file

func (b *binder) effects() string {
	if b.opts.Uses == nil {
		return " uses ffi"
	}
	if len(b.opts.Uses) == 0 {
		return ""
	}
	return " uses " + strings.Join(b.opts.Uses, ", ")
}

func (b *binder) declFile(fns []*cfunc) []byte {
	bnd := b.bnd
	var sb strings.Builder
	fmt.Fprintf(&sb, "// Code generated by xo bind --c from C header %s. DO NOT EDIT.\n", bnd.Header)
	fmt.Fprintf(&sb, "// xo-bind-c: v%d %s\n", Version, clangVersion(b.clang))
	fmt.Fprintf(&sb, "// c-header: %s\n", bnd.HeaderPath)
	sb.WriteString("//\n")
	fmt.Fprintf(&sb, "// Coverage: functions %d/%d.\n", bnd.Bound, bnd.Total)
	if b.opts.Uses == nil {
		sb.WriteString("// Every function carries the `ffi` effect.\n")
	} else if len(b.opts.Uses) == 0 {
		sb.WriteString("// The `use c` line declares `uses none`: calls perform no effect.\n")
	} else {
		fmt.Fprintf(&sb, "// The `use c` line declares the effects %s for every call.\n", strings.Join(b.opts.Uses, ", "))
	}
	sb.WriteString("// Values are copied at the boundary; handles are C pointers that Xo\n")
	sb.WriteString("// never frees (call the library's own free function).\n")
	sb.WriteString("// Replay (decision 0093): native builds do not record yet; each\n")
	sb.WriteString("// declaration's `Replay:` line says how a recording will treat it.\n")
	if len(bnd.Skipped) > 0 {
		sb.WriteString("//\n// Skipped (signatures that do not map):\n")
		var ks []string
		for k := range bnd.Skipped {
			ks = append(ks, k)
		}
		sort.Strings(ks)
		for _, k := range ks {
			fmt.Fprintf(&sb, "// skip: %s: %s\n", k, bnd.Skipped[k])
		}
	}
	if len(b.handles) > 0 {
		sb.WriteString("\n// Handles (opaque C pointers)\n\n")
		for _, tag := range sortedKeys(b.handles) {
			fmt.Fprintf(&sb, "/// C: struct %s *\nstruct %s {}\n", tag, b.handles[tag])
		}
	}
	if len(b.structs) > 0 {
		sb.WriteString("\n// Structs (copied)\n")
		for _, tag := range sortedKeys(b.structs) {
			st, _ := b.flatStruct(ctype{base: "struct " + tag})
			fmt.Fprintf(&sb, "\n/// C: struct %s\nstruct %s {\n", tag, st.xo)
			for _, fd := range st.fields {
				fmt.Fprintf(&sb, "  %s: %s\n", fd.xo, fd.sc.xo)
			}
			sb.WriteString("}\n")
		}
	}
	if len(fns) > 0 {
		sb.WriteString("\n// Functions\n")
	}
	for _, f := range fns {
		fmt.Fprintf(&sb, "\n/// C: %s\n", f.sig)
		if f.replay == "handle" {
			sb.WriteString("/// Replay: a C handle crosses: re executed by a replay\n")
		} else {
			sb.WriteString("/// Replay: by value\n")
		}
		var ps []string
		for _, p := range f.params {
			ps = append(ps, p.xo+": "+p.xoType())
		}
		res := ""
		switch f.ret.kind {
		case kScalar:
			res = " -> " + f.ret.sc.xo
		case kStr:
			res = " -> Str"
		case kHandle:
			res = " -> " + f.ret.handle + "?"
		case kStruct:
			res = " -> " + f.ret.st.xo
		}
		fmt.Fprintf(&sb, "fn %s(%s)%s%s\n", f.xo, strings.Join(ps, ", "), res, b.effects())
	}
	return []byte(sb.String())
}

func (p *cparam) xoType() string {
	switch p.kind {
	case kScalar:
		return p.sc.xo
	case kStr, kStrN:
		return "Str"
	case kBytes:
		return "Bytes"
	case kHandle:
		return p.handle
	case kStruct:
		return p.st.xo
	}
	return "?"
}

func sortedKeys(m map[string]string) []string {
	var ks []string
	for k := range m {
		ks = append(ks, k)
	}
	sort.Strings(ks)
	return ks
}

// ---------------------------------------------------------------------------
// The C adapter

// Sym is the runtime call name of a bound function (`rt.<Sym>`), whose C
// symbol is xo_<Sym>.
func (bnd *Binding) Sym(xo string) string { return bnd.Prefix + xo }

func wordType(sc scalar) string {
	if sc.float {
		return "double"
	}
	return "int64_t"
}

func (b *binder) adapterFile(fns []*cfunc) []byte {
	bnd := b.bnd
	var sb strings.Builder
	fmt.Fprintf(&sb, "// Code generated by xo bind --c from C header %s. DO NOT EDIT.\n", bnd.Header)
	sb.WriteString("// Adapters with the calling convention of the native runtime (decision 0116).\n\n")
	sb.WriteString("#include <stdint.h>\n#include <stdlib.h>\n#include <string.h>\n")
	if bnd.Local {
		fmt.Fprintf(&sb, "#include %q\n\n", bnd.HeaderPath)
	} else {
		fmt.Fprintf(&sb, "#include <%s>\n\n", bnd.Header)
	}
	sb.WriteString("void *xo_cffi_str(const char *p, int64_t n, int64_t *len);\n\n")
	sb.WriteString("static char *xo_cffi_cstr(const char *p, int64_t n) {\n\tchar *s = malloc((size_t)n + 1);\n\tif (!s) {\n\t\tabort();\n\t}\n\tif (n > 0) {\n\t\tmemcpy(s, p, (size_t)n);\n\t}\n\ts[n] = 0;\n\treturn s;\n}\n")
	for _, f := range fns {
		sym := "xo_" + bnd.Sym(f.xo)
		var params, args, pre, post []string
		for _, p := range f.params {
			switch p.kind {
			case kScalar:
				params = append(params, wordType(p.sc)+" "+p.xo)
				args = append(args, convIn(p.sc, p.xo))
			case kStr:
				params = append(params, "const char *"+p.xo+"_p", "int64_t "+p.xo+"_n")
				pre = append(pre, fmt.Sprintf("char *%s = xo_cffi_cstr(%s_p, %s_n);", p.xo, p.xo, p.xo))
				post = append(post, fmt.Sprintf("free(%s);", p.xo))
				args = append(args, p.xo)
			case kStrN, kBytes:
				params = append(params, "const char *"+p.xo+"_p", "int64_t "+p.xo+"_n")
				ptrT := "const " + p.ct.base + " *"
				lenS, _ := scalarOf(p.lenCT)
				args = append(args, fmt.Sprintf("(%s)%s_p", ptrT, p.xo), fmt.Sprintf("(%s)%s_n", lenS.c, p.xo))
			case kHandle:
				params = append(params, "int64_t "+p.xo)
				args = append(args, fmt.Sprintf("(%s *)(intptr_t)%s", p.ct.base, p.xo))
			case kStruct:
				var inits []string
				for _, fd := range p.st.fields {
					fp := p.xo + "_" + fd.xo
					params = append(params, wordType(fd.sc)+" "+fp)
					inits = append(inits, fmt.Sprintf("%s.%s = %s;", p.xo, fd.c, convIn(fd.sc, fp)))
				}
				pre = append(pre, fmt.Sprintf("struct %s %s;", p.st.tag, p.xo), fmt.Sprintf("memset(&%s, 0, sizeof %s);", p.xo, p.xo))
				pre = append(pre, inits...)
				args = append(args, p.xo)
			}
		}
		call := f.c + "(" + strings.Join(args, ", ") + ")"
		ret := "void"
		var body []string
		body = append(body, pre...)
		switch f.ret.kind {
		case kVoid:
			body = append(body, call+";")
			body = append(body, post...)
		case kScalar:
			ret = wordType(f.ret.sc)
			body = append(body, fmt.Sprintf("%s r = (%s)%s;", ret, ret, convOut(f.ret.sc, call)))
			body = append(body, post...)
			body = append(body, "return r;")
		case kStr:
			ret = "void *"
			params = append(params, "int64_t *xo_len")
			body = append(body, "const char *c = "+call+";", "if (!c) {", "\tc = \"\";", "}", "void *r = xo_cffi_str(c, (int64_t)strlen(c), xo_len);")
			body = append(body, post...)
			body = append(body, "return r;")
		case kHandle:
			ret = "int64_t"
			body = append(body, "int64_t r = (int64_t)(intptr_t)"+call+";")
			body = append(body, post...)
			body = append(body, "return r;")
		case kStruct:
			slot := sym + "_ret"
			fmt.Fprintf(&sb, "\nstatic _Thread_local struct %s %s;\n", f.ret.st.tag, slot)
			body = append(body, slot+" = "+call+";")
			body = append(body, post...)
		}
		if len(params) == 0 {
			params = []string{"void"}
		}
		fmt.Fprintf(&sb, "\n// %s\n%s %s(%s) {\n", f.sig, ret, sym, strings.Join(params, ", "))
		for _, l := range body {
			sb.WriteString("\t" + l + "\n")
		}
		sb.WriteString("}\n")
		if f.ret.kind == kStruct {
			for i, fd := range f.ret.st.fields {
				w := wordType(fd.sc)
				fmt.Fprintf(&sb, "\n%s %s_r%d(void) {\n\treturn (%s)%s;\n}\n", w, sym, i, w, convOut(fd.sc, sym+"_ret."+fd.c))
			}
		}
	}
	return []byte(sb.String())
}

// convIn converts a word to the C type of sc.
func convIn(sc scalar, v string) string {
	switch {
	case sc.xo == "Bool":
		return "(_Bool)(" + v + " != 0)"
	case strings.HasPrefix(sc.c, "enum "):
		return "(" + sc.c + ")(int)" + v
	}
	return "(" + sc.c + ")" + v
}

// convOut converts a C value of sc's type to its word.
func convOut(sc scalar, v string) string {
	if sc.xo == "Bool" {
		return "(" + v + " ? 1 : 0)"
	}
	return v
}
