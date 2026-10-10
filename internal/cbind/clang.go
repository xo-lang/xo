package cbind

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// Clang finds the C compiler that reads headers: $XO_CLANG, then clang on
// PATH (the LLVM backend's rule, internal/build.Clang).
func Clang() (string, error) {
	if c := os.Getenv("XO_CLANG"); c != "" {
		return exec.LookPath(c)
	}
	p, err := exec.LookPath("clang")
	if err != nil {
		return "", errors.New("`use c` needs clang to read C headers: install it or set XO_CLANG")
	}
	return p, nil
}

// clangVersion is the first line of `clang --version`.
func clangVersion(clang string) string {
	out, err := exec.Command(clang, "--version").Output()
	if err != nil {
		return "unknown"
	}
	line, _, _ := strings.Cut(string(out), "\n")
	return strings.TrimSpace(line)
}

// dumpAST runs clang's JSON AST dump on a translation unit that includes
// the header and returns the top level declarations with the file each
// is declared in.
func dumpAST(clang, include string, incDirs []string) ([]decl, error) {
	tmp, err := os.MkdirTemp("", "xo-cbind-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(tmp)
	in := filepath.Join(tmp, "xo_cbind_in.c")
	if err := os.WriteFile(in, []byte(include+"\n"), 0o644); err != nil {
		return nil, err
	}
	args := []string{"-fsyntax-only", "-Xclang", "-ast-dump=json", "-x", "c"}
	for _, d := range incDirs {
		args = append(args, "-I", d)
	}
	run := func(extra ...string) ([]byte, []byte, error) {
		cmd := exec.Command(clang, append(append(append([]string(nil), args...), extra...), in)...)
		var stdout, stderr bytes.Buffer
		cmd.Stdout, cmd.Stderr = &stdout, &stderr
		err := cmd.Run()
		return stdout.Bytes(), stderr.Bytes(), err
	}
	out, errOut, err := run()
	if err != nil && runtime.GOOS == "darwin" {
		// clang's default SDK may be missing: try the one xcrun names
		if p, e2 := exec.Command("xcrun", "--show-sdk-path").Output(); e2 == nil {
			if o2, eo2, e3 := run("-isysroot", strings.TrimSpace(string(p))); e3 == nil {
				out, errOut, err = o2, eo2, nil
			}
		}
	}
	if err != nil {
		msg := strings.TrimSpace(string(errOut))
		if i := strings.Index(msg, "\n"); i >= 0 && strings.Count(msg, "\n") > 6 {
			msg = strings.Join(strings.SplitN(msg, "\n", 7)[:6], "\n")
		}
		return nil, fmt.Errorf("clang cannot read the header: %s", msg)
	}
	return topDecls(out)
}

// obj is a JSON object with its keys in source order: clang leaves out
// the `file` of a location that repeats the previous one, so the order
// decides which file a declaration is in.
type obj struct {
	keys []string
	vals []any
}

func (o *obj) get(k string) any {
	for i, kk := range o.keys {
		if kk == k {
			return o.vals[i]
		}
	}
	return nil
}

func (o *obj) str(k string) string {
	s, _ := o.get(k).(string)
	return s
}

func (o *obj) obj(k string) *obj {
	x, _ := o.get(k).(*obj)
	return x
}

func (o *obj) list(k string) []any {
	x, _ := o.get(k).([]any)
	return x
}

func (o *obj) flag(k string) bool {
	b, _ := o.get(k).(bool)
	return b
}

func decodeValue(dec *json.Decoder) (any, error) {
	t, err := dec.Token()
	if err != nil {
		return nil, err
	}
	switch d := t.(type) {
	case json.Delim:
		switch d {
		case '{':
			o := &obj{}
			for dec.More() {
				kt, err := dec.Token()
				if err != nil {
					return nil, err
				}
				k, _ := kt.(string)
				v, err := decodeValue(dec)
				if err != nil {
					return nil, err
				}
				o.keys = append(o.keys, k)
				o.vals = append(o.vals, v)
			}
			if _, err := dec.Token(); err != nil {
				return nil, err
			}
			return o, nil
		case '[':
			var xs []any
			for dec.More() {
				v, err := decodeValue(dec)
				if err != nil {
					return nil, err
				}
				xs = append(xs, v)
			}
			if _, err := dec.Token(); err != nil {
				return nil, err
			}
			return xs, nil
		}
		return nil, fmt.Errorf("unexpected %v", d)
	}
	return t, nil
}

// decl is a top level declaration of the translation unit.
type decl struct {
	n    *obj
	file string // the file of its location ("" for builtins)
}

// topDecls decodes the dump and finds the file of each top level
// declaration by replaying clang's file elision in key order.
func topDecls(data []byte) ([]decl, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	v, err := decodeValue(dec)
	if err != nil && err != io.EOF {
		return nil, fmt.Errorf("reading clang's AST dump: %v", err)
	}
	tu, ok := v.(*obj)
	if !ok {
		return nil, errors.New("clang's AST dump is not an object")
	}
	last := ""
	var walk func(v any)
	walkLoc := func(l *obj) {
		if l == nil {
			return
		}
		if f := l.str("file"); f != "" {
			last = f
		}
	}
	walk = func(v any) {
		switch x := v.(type) {
		case *obj:
			for i, k := range x.keys {
				switch k {
				case "includedFrom":
					continue
				case "loc", "begin", "end", "spellingLoc", "expansionLoc":
					if l, ok := x.vals[i].(*obj); ok {
						walkLoc(l)
					}
				}
				walk(x.vals[i])
			}
		case []any:
			for _, e := range x {
				walk(e)
			}
		}
	}
	var out []decl
	for _, e := range tu.list("inner") {
		n, ok := e.(*obj)
		if !ok {
			continue
		}
		file := ""
		for i, k := range n.keys {
			if k == "loc" {
				if l, ok := n.vals[i].(*obj); ok {
					walk(l)
					walkLoc(l)
					if l.obj("expansionLoc") != nil || l.str("file") != "" || l.get("line") != nil || l.get("offset") != nil {
						file = last
					}
				}
				continue
			}
			walk(n.vals[i])
		}
		out = append(out, decl{n: n, file: file})
	}
	return out, nil
}
