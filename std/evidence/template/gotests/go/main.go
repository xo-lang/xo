// Command go is the Go side of std/template's gotests evidence (decision 0720,
// task #403): main CASES prints, for each case line `KIND<TAB>B64...`, the
// base64 of package template's result as text;
// std/evidence/template/gotests/main.xo prints std/template's the same way. See
// scripts/stdproven/gotests.go.
package main

import (
	"bufio"
	"encoding/base64"
	"fmt"
	"os"
	"strings"
	"text/template"
)

// trun parses a template (exec, parse: name and text; cmp: "{{EXPR}}")
// and, but for parse, executes it on nil data, as json null is.
func trun(kind string, in []string) string {
	name, src := "cmp", ""
	if kind == "cmp" {
		src = "{{" + in[0] + "}}"
	} else {
		name, src = in[0], in[1]
	}
	t, err := template.New(name).Parse(src)
	if err != nil {
		return "err " + err.Error()
	}
	if kind == "parse" {
		return "ok"
	}
	var b strings.Builder
	if err := t.Execute(&b, nil); err != nil {
		return "err " + err.Error()
	}
	return "ok " + b.String()
}

func run(kind string, in []string) string {
	switch kind {
	case "exec", "parse", "cmp":
		return trun(kind, in)
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
