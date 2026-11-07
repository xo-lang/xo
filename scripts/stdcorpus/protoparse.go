package main

import (
	"crypto/sha256"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

var (
	protoBlockComment = regexp.MustCompile(`(?s)/\*.*?\*/`)
	protoLineComment  = regexp.MustCompile(`//[^\n]*`)
	protoKeyword      = regexp.MustCompile(`^(message|enum)\s+\w+`)
	protoRPC          = regexp.MustCompile(`(?m)^\s*rpc\s+\w+`)
)

// protoparseCorpus is the real world check of std/protoparse (decision 0530): the proto3 files of the Go module cache
// (grpc-go, protobuf-go, genproto and their neighbors) are parsed, and the counts of top level messages, top level
// enums and rpcs (found here with regular expressions on the text without comments, which is independent of the
// parser) must match; the generated Xo source must be the same on a second run.
func protoparseCorpus() string {
	out, err := exec.Command("go", "env", "GOMODCACHE").Output()
	if err != nil {
		fmt.Fprintln(os.Stderr, "stdcorpus: go env GOMODCACHE:", err)
		os.Exit(1)
	}
	root := strings.TrimSpace(string(out))
	type pf struct{ name, text string }
	var files []pf
	seen := map[[32]byte]bool{}
	filepath.Walk(root, func(p string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || !strings.HasSuffix(p, ".proto") || info.Size() < 300 || info.Size() > 14000 {
			return nil
		}
		b, _ := os.ReadFile(p)
		t := string(b)
		if !strings.Contains(t, `syntax = "proto3"`) || strings.Contains(t, "group ") {
			return nil
		}
		h := sha256.Sum256(b)
		if seen[h] {
			return nil
		}
		seen[h] = true
		rel, _ := filepath.Rel(root, p)
		files = append(files, pf{rel, t})
		return nil
	})
	sort.Slice(files, func(i, j int) bool { return files[i].name < files[j].name })
	// every kth file, so the sample spreads over the modules
	step := len(files)/70 + 1
	var o strings.Builder
	o.WriteString(header("the proto3 files of the Go module cache (Apache 2.0 licensed: grpc-go, protobuf-go, genproto and others), with the counts of top level messages, top level enums and rpcs found by regular expressions"))
	o.WriteString("use std/protoparse\n\n")
	o.WriteString("/// (file, text, top level messages, top level enums, rpcs)\nconst FILES: List[(Str, Str, Int, Int, Int)] = [\n")
	n := 0
	for i := 0; i < len(files); i += step {
		f := files[i]
		bare := protoLineComment.ReplaceAllString(protoBlockComment.ReplaceAllString(f.text, ""), "")
		fmt.Fprintf(&o, "  (%s, %s, %d, %d, %d),\n", xq(f.name), xq(f.text),
			protoTop(bare, "message"), protoTop(bare, "enum"), len(protoRPC.FindAllString(bare, -1)))
		n++
	}
	o.WriteString("]\n")
	o.WriteString(protoparseCheckXo)
	fmt.Fprintf(os.Stderr, "stdcorpus: protoparse: %d files\n", n)
	return o.String()
}

const protoparseCheckXo = `
test "std/protoparse reads the proto3 files of real projects" {
  var bad: List[Str] = []
  for (name, text, msgs, enums, rpcs) in FILES {
    match protoparse.parse(text) {
      Ok(f) => {
        var r = 0
        for s in f.services {
          r += s.rpcs.len()
        }
        if f.messages.len() != msgs || f.enums.len() != enums || r != rpcs {
          bad.push("${name}: got ${f.messages.len()} messages ${f.enums.len()} enums ${r} rpcs, want ${msgs} ${enums} ${rpcs}")
        }
        match protoparse.gen(f) {
          Ok(a) => {
            let b = protoparse.gen(f).or_fault("gen again")
            if a != b {
              bad.push("${name}: gen is not deterministic")
            }
          }
          // a type from an imported file is not known to one file's gen
          Err(e) => if !text.contains("\nimport ") { bad.push("${name}: gen: ${e}") }
        }
      }
      Err(e) => bad.push("${name}: ${e}")
    }
  }
  if !bad.is_empty() {
    fault("${bad.len()} of ${FILES.len()} files differ, first: ${bad[0]}")
  }
}
`

// protoTop counts the keyword declarations (message or enum) at brace depth 0 of text without comments, skipping
// string literals; it does not depend on the layout of the file.
func protoTop(t, kw string) int {
	n, depth := 0, 0
	isWord := func(c byte) bool {
		return c == '_' || c == '.' || c >= '0' && c <= '9' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z'
	}
	for i := 0; i < len(t); i++ {
		switch c := t[i]; {
		case c == '"' || c == '\'':
			for i++; i < len(t) && t[i] != c; i++ {
				if t[i] == '\\' {
					i++
				}
			}
		case c == '{':
			depth++
		case c == '}':
			depth--
		case depth == 0 && (i == 0 || !isWord(t[i-1])):
			if m := protoKeyword.FindStringSubmatch(t[i:]); m != nil && m[1] == kw {
				n++
				i += len(kw) - 1
			}
		}
	}
	return n
}
