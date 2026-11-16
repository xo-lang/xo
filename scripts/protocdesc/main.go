// Command protocdesc writes the protoc cases of std/protodesc_test.xo: each
// .proto file of scripts/protocdesc/testdata and of the protoparse corpus
// (scripts/stdcorpus/testdata/protoparse) that protoc compiles, with the files
// it imports (the well known types from protoc's include directory), as
// (name, text, protoc's FileDescriptorProto as hex) and the groups to build
// in order. protodesc.build must give protoc's bytes for each, and
// protodesc.build_with_source_info protoc's SourceCodeInfo (PROTOC_SCI).
//
// It needs protoc (not on the offload hosts), so it is not a stdcorpus
// generator (gate.sh's stdcorpus -check runs there): run it by hand after a
// change to the .proto files, `go run ./scripts/protocdesc`, which rewrites
// the block between the BEGIN and END lines of std/protodesc_test.xo.
package main

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

const (
	testFile = "std/protodesc_test.xo"
	begin    = "// BEGIN protoc cases (go run ./scripts/protocdesc; do not edit)\n"
	end      = "// END protoc cases\n"
)

type text struct{ name, src, hex, sci string }

func main() {
	wkt := includeDir()
	var tops []string
	for _, root := range []string{"scripts/protocdesc/testdata", "scripts/stdcorpus/testdata/protoparse"} {
		filepath.Walk(root, func(p string, info os.FileInfo, err error) error {
			if err == nil && !info.IsDir() && strings.HasSuffix(p, ".proto") {
				tops = append(tops, p)
			}
			return nil
		})
	}
	sort.Strings(tops)
	var texts []text
	index := map[string]int{} // name + "\x00" + src -> texts index
	var groups [][]int
	skipped := 0
	for _, p := range tops {
		g, ok := compile(p, wkt)
		if !ok {
			skipped++
			continue
		}
		var ids []int
		for _, t := range g {
			k := t.name + "\x00" + t.src
			i, seen := index[k]
			if !seen {
				i = len(texts)
				index[k] = i
				texts = append(texts, t)
			}
			ids = append(ids, i)
		}
		groups = append(groups, ids)
	}
	var o strings.Builder
	o.WriteString(begin)
	fmt.Fprintf(&o, "// %s; %d files, %d groups (%d corpus files protoc did not compile, skipped)\n", protocVersion(), len(texts), len(groups), skipped)
	o.WriteString("/// (file name, .proto text, protoc's FileDescriptorProto as hex)\nconst PROTOC_FILES: List[(Str, Str, Str)] = [\n")
	for _, t := range texts {
		fmt.Fprintf(&o, "  (%s, %s, \"%s\"),\n", xq(t.name), xq(t.src), t.hex)
	}
	o.WriteString("]\n\n/// each a file and the files it imports, imports first (indexes into PROTOC_FILES)\nconst PROTOC_GROUPS: List[List[Int]] = [\n")
	for _, g := range groups {
		s := make([]string, len(g))
		for i, v := range g {
			s[i] = strconv.Itoa(v)
		}
		fmt.Fprintf(&o, "  [%s],\n", strings.Join(s, ", "))
	}
	o.WriteString("]\n\n/// protoc's SourceCodeInfo (--include_source_info) of each file outside google/protobuf: (index into\n/// PROTOC_FILES, hex)\nconst PROTOC_SCI: List[(Int, Str)] = [\n")
	for i, t := range texts {
		if strings.HasPrefix(t.name, "google/protobuf/") || t.sci == "" {
			continue
		}
		fmt.Fprintf(&o, "  (%d, \"%s\"),\n", i, t.sci)
	}
	o.WriteString("]\n")
	o.WriteString(end)
	old, err := os.ReadFile(testFile)
	if err != nil {
		panic(err)
	}
	s := string(old)
	i, j := strings.Index(s, begin), strings.Index(s, end)
	if i < 0 || j < i {
		s = strings.TrimRight(s, "\n") + "\n\n" + o.String()
	} else {
		s = s[:i] + o.String() + s[j+len(end):]
	}
	if err := os.WriteFile(testFile, []byte(s), 0o644); err != nil {
		panic(err)
	}
	fmt.Fprintf(os.Stderr, "protocdesc: %d files, %d groups, %d skipped\n", len(texts), len(groups), skipped)
}

// compile runs protoc on p from the nearest ancestor directory that resolves
// its imports, and returns its file and every import, imports first.
func compile(p, wkt string) ([]text, bool) {
	dir := filepath.Dir(p)
	for d := dir; d != "." && d != "/"; d = filepath.Dir(d) {
		rel, _ := filepath.Rel(d, p)
		out, err := os.CreateTemp("", "protocdesc-*.pb")
		if err != nil {
			panic(err)
		}
		out.Close()
		cmd := exec.Command("protoc", "-I", d, "-I", wkt, "--include_imports", "--include_source_info", "--descriptor_set_out="+out.Name(), filepath.ToSlash(rel))
		var stderr bytes.Buffer
		cmd.Stderr = &stderr
		err = cmd.Run()
		set, _ := os.ReadFile(out.Name())
		os.Remove(out.Name())
		if err != nil {
			continue
		}
		var g []text
		for _, fd := range files(set) {
			name := fdName(fd)
			src, err := os.ReadFile(filepath.Join(d, name))
			if err != nil {
				src, err = os.ReadFile(filepath.Join(wkt, name))
			}
			if err != nil {
				panic(name + ": " + err.Error())
			}
			desc, sci := splitSourceInfo(fd)
			g = append(g, text{name, string(src), hex.EncodeToString(desc), hex.EncodeToString(sci)})
		}
		return g, true
	}
	return nil, false
}

// files splits a FileDescriptorSet into its files (field 1, in order).
func files(set []byte) [][]byte {
	var out [][]byte
	for len(set) > 0 {
		tag, n := uvarint(set)
		set = set[n:]
		if tag != 1<<3|2 {
			panic("unexpected field in FileDescriptorSet")
		}
		l, n := uvarint(set)
		set = set[n:]
		out = append(out, set[:l])
		set = set[l:]
	}
	return out
}

// splitSourceInfo splits a FileDescriptorProto into the descriptor without its
// source_code_info (field 9) and that field's message.
func splitSourceInfo(fd []byte) (desc, sci []byte) {
	for i := 0; i < len(fd); {
		start := i
		tag, n := uvarint(fd[i:])
		i += n
		switch tag & 7 {
		case 0:
			_, n = uvarint(fd[i:])
			i += n
		case 2:
			l, n := uvarint(fd[i:])
			if tag>>3 == 9 {
				sci = fd[i+n : i+n+int(l)]
				i += n + int(l)
				continue
			}
			i += n + int(l)
		default:
			panic("unexpected wire type")
		}
		desc = append(desc, fd[start:i]...)
	}
	return desc, sci
}

// fdName is a FileDescriptorProto's name (field 1).
func fdName(fd []byte) string {
	for len(fd) > 0 {
		tag, n := uvarint(fd)
		fd = fd[n:]
		switch tag & 7 {
		case 0:
			_, n = uvarint(fd)
			fd = fd[n:]
		case 2:
			l, n := uvarint(fd)
			if tag>>3 == 1 {
				return string(fd[n : n+int(l)])
			}
			fd = fd[n+int(l):]
		default:
			panic("unexpected wire type")
		}
	}
	return ""
}

func uvarint(b []byte) (uint64, int) {
	var v uint64
	for i, c := range b {
		v |= uint64(c&0x7f) << (7 * i)
		if c < 0x80 {
			return v, i + 1
		}
	}
	panic("bad varint")
}

// includeDir is protoc's directory of the well known types' .proto files.
func includeDir() string {
	p, err := exec.LookPath("protoc")
	if err != nil {
		panic("protocdesc needs protoc")
	}
	p, _ = filepath.EvalSymlinks(p)
	for d := filepath.Dir(p); d != "/"; d = filepath.Dir(d) {
		if _, err := os.Stat(filepath.Join(d, "include", "google", "protobuf", "descriptor.proto")); err == nil {
			return filepath.Join(d, "include")
		}
	}
	panic("no include/google/protobuf next to protoc")
}

func protocVersion() string {
	b, _ := exec.Command("protoc", "--version").Output()
	return strings.TrimSpace(string(b))
}

// xq quotes s as an Xo string literal.
func xq(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c == '\\':
			b.WriteString(`\\`)
		case c == '"':
			b.WriteString(`\"`)
		case c == '$':
			b.WriteString(`\$`)
		case c == '\n':
			b.WriteString(`\n`)
		case c == '\r':
			b.WriteString(`\r`)
		case c == '\t':
			b.WriteString(`\t`)
		case c < 0x20 || c == 0x7f:
			fmt.Fprintf(&b, `\u{%x}`, c)
		default:
			b.WriteByte(c)
		}
	}
	b.WriteByte('"')
	return b.String()
}
