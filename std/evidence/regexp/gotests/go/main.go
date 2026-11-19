// Command go is the Go side of std/regexp's gotests evidence (decision 0720,
// task #403): main CASES prints, for each case line `KIND<TAB>B64...`, the
// base64 of package regexp's result as text;
// std/evidence/regexp/gotests/main.xo prints std/regexp's the same way. See
// scripts/stdproven/gotests.go.
package main

import (
	"bufio"
	"encoding/base64"
	"fmt"
	"os"
	"regexp"
	"strings"
)

func compileText(pat string) string {
	if _, err := regexp.Compile(pat); err != nil {
		return "err " + strings.TrimPrefix(err.Error(), "error parsing regexp: ")
	}
	return "ok"
}

func findAll(pat, text string) string {
	re, err := regexp.Compile(pat)
	if err != nil {
		return "err " + err.Error()
	}
	var ms []string
	for _, m := range re.FindAllStringSubmatchIndex(text, -1) {
		var ps []string
		for i := 0; i+1 < len(m); i += 2 {
			if m[i] < 0 {
				ps = append(ps, "_")
			} else {
				ps = append(ps, fmt.Sprintf("%d-%d", m[i], m[i+1]))
			}
		}
		ms = append(ms, strings.Join(ps, ","))
	}
	return fmt.Sprintf("%d:%s", len(ms), strings.Join(ms, ";"))
}

func replace(pat, repl, in string, literal bool) string {
	re, err := regexp.Compile(pat)
	if err != nil {
		return "err " + err.Error()
	}
	if literal {
		return re.ReplaceAllLiteralString(in, repl)
	}
	return re.ReplaceAllString(in, repl)
}

func run(kind string, in []string) string {
	switch kind {
	case "compile":
		return compileText(in[0])
	case "find":
		return findAll(in[0], in[1])
	case "replace":
		return replace(in[0], in[1], in[2], false)
	case "replacelit":
		return replace(in[0], in[1], in[2], true)
	case "quote":
		return regexp.QuoteMeta(in[0])
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
