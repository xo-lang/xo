// Command go is the Go side of std/path's gotests evidence (decision 0720,
// task #403): main CASES prints, for each case line `KIND<TAB>B64...`, the
// base64 of package path's result as text;
// std/evidence/path/gotests/main.xo prints std/path's the same way. See
// scripts/stdproven/gotests.go.
package main

import (
	"bufio"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"path"
	"strings"
)

func match(pattern, name string) string {
	ok, err := path.Match(pattern, name)
	if err != nil {
		if errors.Is(err, path.ErrBadPattern) {
			return "err bad pattern " + pattern
		}
		return "err " + err.Error()
	}
	return fmt.Sprint(ok)
}

func run(kind string, in []string) string {
	switch kind {
	case "clean":
		return path.Clean(in[0])
	case "split":
		d, f := path.Split(in[0])
		return d + "|" + f
	case "ext":
		return path.Ext(in[0])
	case "base":
		return path.Base(in[0])
	case "dir":
		return path.Dir(in[0])
	case "abs":
		return fmt.Sprint(path.IsAbs(in[0]))
	case "match":
		return match(in[0], in[1])
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
