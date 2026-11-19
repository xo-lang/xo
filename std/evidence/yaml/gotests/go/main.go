// Command go is the Go side of std/yaml's gotests evidence (decision 0720,
// task #403): main CASES prints, for each case line `KIND<TAB>B64...`, the
// base64 of package yaml's result as text;
// std/evidence/yaml/gotests/main.xo prints std/yaml's the same way. See
// scripts/stdproven/gotests.go.
package main

import (
	"bufio"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"gopkg.in/yaml.v3"
	"os"
	"strconv"
	"strings"
)

var kindNames = map[yaml.Kind]string{yaml.DocumentNode: "doc", yaml.SequenceNode: "seq", yaml.MappingNode: "map", yaml.ScalarNode: "scalar", yaml.AliasNode: "alias"}

func dump(n *yaml.Node, depth int, out *[]string) {
	*out = append(*out, fmt.Sprintf("%s%s %d %s %s %s %s %s %s %d:%d", strings.Repeat(" ", depth), kindNames[n.Kind], n.Style, strconv.Quote(n.Tag), strconv.Quote(n.Value), strconv.Quote(n.Anchor), strconv.Quote(n.HeadComment), strconv.Quote(n.LineComment), strconv.Quote(n.FootComment), n.Line, n.Column))
	for _, c := range n.Content {
		dump(c, depth+1, out)
	}
}

// parse is Unmarshal into a Node; a text with no document leaves the zero
// Node, which std/yaml's parse documents as a Document with no content.
func parse(text string) (*yaml.Node, error) {
	var n yaml.Node
	if err := yaml.Unmarshal([]byte(text), &n); err != nil {
		return nil, err
	}
	if n.Kind == 0 {
		n.Kind = yaml.DocumentNode
	}
	return &n, nil
}

func node(text string) string {
	n, err := parse(text)
	if err != nil {
		return "err " + err.Error()
	}
	var out []string
	dump(n, 0, &out)
	return strings.Join(out, "\n")
}

func reenc(text string) string {
	n, err := parse(text)
	if err != nil {
		return "err " + err.Error()
	}
	b, err := yaml.Marshal(n)
	if err != nil {
		return "err " + err.Error()
	}
	return string(b)
}

// toJSON is yaml.Unmarshal into an interface{} and json.Marshal of it.
func toJSON(text string) string {
	var v interface{}
	if err := yaml.Unmarshal([]byte(text), &v); err != nil {
		return "err " + err.Error()
	}
	b, err := json.Marshal(v)
	if err != nil {
		return "err " + err.Error()
	}
	return string(b)
}

func run(kind string, in []string) string {
	switch kind {
	case "json":
		return toJSON(in[0])
	case "node":
		return node(in[0])
	case "reenc":
		return reenc(in[0])
	}
	return "unknown kind " + kind
}

func main() {
	f, err := os.Open(os.Args[1])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	sc := bufio.NewScanner(f)
	sc.Buffer(nil, 1<<24)
	w := bufio.NewWriter(os.Stdout)
	defer w.Flush()
	for sc.Scan() {
		f := strings.Split(sc.Text(), "\t")
		in := make([]string, 0, len(f)-1)
		for _, s := range f[1:] {
			b, err := base64.StdEncoding.DecodeString(s)
			if err != nil {
				fmt.Fprintln(os.Stderr, err)
				os.Exit(1)
			}
			in = append(in, string(b))
		}
		fmt.Fprintln(w, base64.StdEncoding.EncodeToString([]byte(run(f[0], in))))
	}
}
