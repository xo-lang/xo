// xolayout reads callgrind output files of xo's small builds and writes xo's
// layout file (decision 0453): every Xo function they ran, by instructions
// run, with no target line, since it holds for every target. Sampling misses functions that run once, which are most of the code a
// small build touches, so this file is exact where `xo prof --layout` is not.
//
//	go run ./scripts/xolayout OUT CALLGRIND...
//
// scripts/xolayout.sh makes the callgrind files.
package main

import (
	"bufio"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

var (
	fnLine    = regexp.MustCompile(`^(c?fn)=\((\d+)\)(?: (.*))?$`)
	recursion = regexp.MustCompile(`'\d+$`)
)

// counts adds one callgrind file's self instructions per function to c.
func counts(path string, c map[string]int) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	ids := map[string]string{}
	cur := ""
	skip := false
	s := bufio.NewScanner(f)
	s.Buffer(make([]byte, 1<<20), 1<<26)
	for s.Scan() {
		l := s.Text()
		if m := fnLine.FindStringSubmatch(l); m != nil {
			if m[3] != "" {
				ids[m[2]] = recursion.ReplaceAllString(strings.TrimSpace(m[3]), "")
			}
			if m[1] == "fn" {
				cur = ids[m[2]]
				c[cur] += 0
				skip = false
			}
			continue
		}
		if strings.HasPrefix(l, "calls=") {
			// the next line is the call's inclusive cost
			skip = true
			continue
		}
		if skip {
			skip = false
			continue
		}
		if cur == "" || l == "" || !strings.ContainsAny(l[:1], "+-*0123456789") {
			continue
		}
		w := strings.Fields(l)
		if len(w) < 2 {
			continue
		}
		if n, err := strconv.Atoi(w[len(w)-1]); err == nil {
			c[cur] += n
		}
	}
	return s.Err()
}

// demangle undoes the emitter's mangle_fn (compiler/arm64 gen.xo): `__` is an
// underscore and `_HH` any other byte outside letters and digits.
func demangle(n string) (string, bool) {
	if !strings.HasPrefix(n, "xo_f_") {
		return "", false
	}
	n = n[len("xo_f_"):]
	var b strings.Builder
	for i := 0; i < len(n); i++ {
		if n[i] != '_' {
			b.WriteByte(n[i])
			continue
		}
		if i+1 < len(n) && n[i+1] == '_' {
			b.WriteByte('_')
			i++
			continue
		}
		if i+2 >= len(n) {
			return "", false
		}
		v, err := strconv.ParseUint(n[i+1:i+3], 16, 8)
		if err != nil {
			return "", false
		}
		b.WriteByte(byte(v))
		i += 2
	}
	return b.String(), true
}

func main() {
	if len(os.Args) < 3 {
		fmt.Fprintln(os.Stderr, "usage: xolayout OUT CALLGRIND...")
		os.Exit(2)
	}
	c := map[string]int{}
	for _, p := range os.Args[2:] {
		if err := counts(p, c); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	}
	type fn struct {
		name string
		n    int
	}
	var fns []fn
	total := 0
	for k, v := range c {
		if name, ok := demangle(k); ok {
			fns = append(fns, fn{name, v + 1})
			total += v
		}
	}
	sort.Slice(fns, func(i, j int) bool {
		if fns[i].n != fns[j].n {
			return fns[i].n > fns[j].n
		}
		return fns[i].name < fns[j].name
	})
	var b strings.Builder
	fmt.Fprintf(&b, "xo layout v1\nsamples %d\n", total)
	for _, f := range fns {
		fmt.Fprintf(&b, "fn %s %d\n", f.name, f.n)
	}
	if err := os.WriteFile(os.Args[1], []byte(b.String()), 0o644); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Printf("%d functions\n", len(fns))
}
