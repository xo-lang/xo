// Compares Go's recorded results (cases.jsonl) with the Xo twin's output (one "INPUT\tOK json|ERR text" per case).
package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

type C struct{ T, In, Go, Out string }

func main() {
	f, _ := os.Open(os.Args[1])
	var cs []C
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for sc.Scan() {
		var m map[string]string
		json.Unmarshal(sc.Bytes(), &m)
		cs = append(cs, C{m["t"], m["in"], m["go"], m["out"]})
	}
	x, _ := os.Open(os.Args[2])
	var xs []string
	sc = bufio.NewScanner(x)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for sc.Scan() {
		xs = append(xs, sc.Text())
	}
	if len(xs) != len(cs) {
		fmt.Println("count mismatch", len(cs), len(xs))
	}
	cat := map[string]int{}
	var show = map[string][]string{}
	for i, c := range cs {
		if i >= len(xs) {
			break
		}
		r := xs[i][strings.Index(xs[i], "\t")+1:]
		var k string
		switch {
		case c.Go == "" && strings.HasPrefix(r, "OK "):
			if r[3:] == c.Out {
				k = "both ok, same value"
			} else {
				k = "both ok, VALUE DIFFERS"
			}
		case c.Go == "":
			k = "go ok, xo error"
		case strings.HasPrefix(r, "OK "):
			k = "go error, xo ok"
		default:
			xe := r[4:]
			switch {
			case strings.HasSuffix(xe, c.Go):
				k = "both error, xo text ends with go text"
			case strings.Contains(c.Go, "unexpected end of JSON input") && (strings.Contains(xe, "EOF")):
				k = "both error, truncation (different words)"
			case strings.HasPrefix(c.Go, "invalid character") || strings.HasPrefix(c.Go, "unexpected end"):
				k = "both error, syntax (different words)"
			case strings.HasPrefix(c.Go, "json: cannot unmarshal"):
				k = "both error, type (different words)"
			default:
				k = "both error, other"
			}
		}
		cat[k]++
		if len(show[k]) < 6 {
			show[k] = append(show[k], fmt.Sprintf("  %s %s\n    go: %s\n    xo: %s", c.T, c.In, c.Go, r))
		}
	}
	for k, n := range cat {
		fmt.Printf("%4d  %s\n", n, k)
		for _, s := range show[k] {
			fmt.Println(s)
		}
	}
}
