// Command genpsl writes the public suffix tables of std/publicsuffix into
// std/publicsuffix.xo (between its BEGIN GENERATED and END GENERATED
// lines) from golang.org/x/net's publicsuffix package: its data files
// (text, nodes, children), its table.go constants and the list's version,
// copied as they are, so std/publicsuffix answers as x/net's does for the
// same list (decision 0617 B).
// Usage: go run ./scripts/genpsl NETDIR FILE
// (NETDIR: the golang.org/x/net@VERSION directory in the module cache).
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

func fail(f string, a ...any) {
	fmt.Fprintf(os.Stderr, "genpsl: "+f+"\n", a...)
	os.Exit(1)
}

func bytesLit(b []byte) string {
	var s strings.Builder
	s.WriteString(`b"`)
	for _, c := range b {
		fmt.Fprintf(&s, `\x%02x`, c)
	}
	s.WriteString(`"`)
	return s.String()
}

func main() {
	if len(os.Args) != 3 {
		fail("usage: go run ./scripts/genpsl NETDIR FILE")
	}
	dir := filepath.Join(os.Args[1], "publicsuffix")
	read := func(name string) []byte {
		b, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			fail("%v", err)
		}
		return b
	}
	table := string(read("table.go"))
	consts := map[string]string{}
	for _, m := range regexp.MustCompile(`(?m)^\s*(\w+)\s*=\s*(\d+)\s*$`).FindAllStringSubmatch(table, -1) {
		consts[m[1]] = m[2]
	}
	ver := regexp.MustCompile(`const version = "(.*)"`).FindStringSubmatch(table)
	ntld := regexp.MustCompile(`const numTLD = (\d+)`).FindStringSubmatch(table)
	if ver == nil || ntld == nil {
		fail("table.go: no version or numTLD")
	}
	need := []string{"nodesBitsChildren", "nodesBitsICANN", "nodesBitsTextOffset", "nodesBitsTextLength", "childrenBitsWildcard", "childrenBitsNodeType", "childrenBitsHi", "childrenBitsLo", "nodesBits"}
	for _, k := range need {
		if consts[k] == "" {
			fail("table.go: no %s", k)
		}
	}
	if consts["nodesBits"] != "40" {
		fail("nodesBits %s: the reader takes 5 byte nodes", consts["nodesBits"])
	}
	text, nodes, children := read("data/text"), read("data/nodes"), read("data/children")
	var o strings.Builder
	fmt.Fprintf(&o, "// BEGIN GENERATED\n// From golang.org/x/net/publicsuffix (scripts/genpsl): %d nodes, %d children, %d bytes of labels.\n", len(nodes)/5, len(children)/4, len(text))
	fmt.Fprintf(&o, "/// The list the tables were made from (x/net's version).\npub const VERSION: Str = %q\n\n", ver[1])
	fmt.Fprintf(&o, "const NUM_TLD: Int = %s\n", ntld[1])
	for _, k := range need[:8] {
		name := strings.ToUpper(regexp.MustCompile(`([a-z])([A-Z])`).ReplaceAllString(k, "${1}_${2}"))
		fmt.Fprintf(&o, "const %s: Int = %s\n", name, consts[k])
	}
	fmt.Fprintf(&o, "\n/// Every label's text, the nodes' offsets into it.\nconst TEXT: Bytes = %s\n\n", bytesLit(text))
	fmt.Fprintf(&o, "/// 5 bytes a node, big endian: ICANN, children index, text offset, text length.\nconst NODES: Bytes = %s\n\n", bytesLit(nodes))
	fmt.Fprintf(&o, "/// 4 bytes a children entry, big endian: wildcard, node type, hi, lo.\nconst CHILDREN: Bytes = %s\n// END GENERATED\n", bytesLit(children))
	src, err := os.ReadFile(os.Args[2])
	if err != nil {
		fail("%v", err)
	}
	s := string(src)
	i, j := strings.Index(s, "// BEGIN GENERATED\n"), strings.Index(s, "// END GENERATED\n")
	if i < 0 || j < i {
		fail("%s: no BEGIN GENERATED ... END GENERATED", os.Args[2])
	}
	s = s[:i] + o.String() + s[j+len("// END GENERATED\n"):]
	if err := os.WriteFile(os.Args[2], []byte(s), 0o644); err != nil {
		fail("%v", err)
	}
}
