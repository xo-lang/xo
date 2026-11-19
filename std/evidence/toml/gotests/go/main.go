// Command go is the Go side of std/toml's gotests evidence (decision 0720,
// task #403): main CASES prints, for each case line `KIND<TAB>B64...`, the
// base64 of package v2's result as text;
// std/evidence/toml/gotests/main.xo prints std/toml's the same way. See
// scripts/stdproven/gotests.go.
package main

import (
	"bufio"
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"github.com/pelletier/go-toml/v2"
	"os"
	"strings"
)

// doc is the document as JSON (Unmarshal into a map, json.Marshal of it,
// keys sorted, < > & as they are), "error" for one that is not TOML
func doc(text string) string {
	var m map[string]interface{}
	if err := toml.Unmarshal([]byte(text), &m); err != nil {
		return "error"
	}
	if m == nil {
		m = map[string]interface{}{}
	}
	var b bytes.Buffer
	e := json.NewEncoder(&b)
	e.SetEscapeHTML(false)
	if err := e.Encode(m); err != nil {
		return "json: " + err.Error()
	}
	return strings.TrimSuffix(b.String(), "\n")
}

func run(kind string, in []string) string {
	switch kind {
	case "doc":
		return doc(in[0])
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
