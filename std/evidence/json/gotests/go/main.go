// Command go is the Go side of std/json's gotests evidence (decision 0720,
// task #403): main CASES prints, for each case line `KIND<TAB>B64...`, the
// base64 of package json's result as text;
// std/evidence/json/gotests/main.xo prints std/json's the same way. See
// scripts/stdproven/gotests.go.
package main

import (
	"bufio"
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

func jrun(kind string, in []string) string {
	b := []byte(in[0])
	switch kind {
	case "valid":
		return fmt.Sprint(json.Valid(b))
	case "number":
		// A number is valid when it is one JSON value and a Number.
		var v any
		d := json.NewDecoder(strings.NewReader(in[0]))
		d.UseNumber()
		if d.Decode(&v) != nil || d.More() {
			return "false"
		}
		_, ok := v.(json.Number)
		return fmt.Sprint(ok && v.(json.Number).String() == in[0])
	case "syntax":
		var v any
		if err := json.Unmarshal(b, &v); err != nil {
			// A text that ends inside a literal or a number: Go names the
			// literal ("invalid character ' ' in literal true"); Xo's
			// go_text gives the truncation text until DecodeErr gets a field,
			// which waits for a seed with the compiler's padding (std/json.xo
			// go_text, gotchas/std-error-struct-field-needs-compiler-first.md).
			if strings.HasPrefix(err.Error(), "invalid character ' ' in ") {
				return "err unexpected end of JSON input"
			}
			return "err " + err.Error()
		}
		return "ok"
	case "compact":
		var buf bytes.Buffer
		if err := json.Compact(&buf, b); err != nil {
			return "err " + err.Error()
		}
		return buf.String()
	case "indent":
		var buf bytes.Buffer
		if err := json.Indent(&buf, b, "", "\t"); err != nil {
			return "err " + err.Error()
		}
		return buf.String()
	}
	return "?"
}

func run(kind string, in []string) string {
	switch kind {
	case "valid", "number", "syntax", "compact", "indent":
		return jrun(kind, in)
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
