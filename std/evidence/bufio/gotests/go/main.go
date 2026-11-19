// Command go is the Go side of std/bufio's gotests evidence (decision 0720,
// task #403): main CASES prints, for each case line `KIND<TAB>B64...`, the
// base64 of package bufio's result as text;
// std/evidence/bufio/gotests/main.xo prints std/bufio's the same way. See
// scripts/stdproven/gotests.go.
package main

import (
	"bufio"
	"bytes"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"strings"
	"testing/iotest"
)

func src(in []byte, one bool) io.Reader {
	if one {
		return iotest.OneByteReader(bytes.NewReader(in))
	}
	return bytes.NewReader(in)
}

// brun: lines (Scanner with ScanLines, each line's invalid UTF-8 bytes
// U+FFFD as std/bufio's read_line) and until (Reader.ReadBytes('\n') to
// the end), over the input whole or a byte at a time (KIND1).
func brun(kind string, in []byte) string {
	one := strings.HasSuffix(kind, "1")
	var out []string
	if strings.HasPrefix(kind, "lines") {
		s := bufio.NewScanner(src(in, one))
		for s.Scan() {
			out = append(out, hex.EncodeToString([]byte(string([]rune(s.Text())))))
		}
		return strings.Join(out, "|")
	}
	r := bufio.NewReaderSize(src(in, one), 16)
	for {
		b, err := r.ReadBytes('\n')
		if len(b) > 0 {
			out = append(out, hex.EncodeToString(b))
		}
		if err != nil {
			break
		}
	}
	return strings.Join(out, "|")
}

func run(kind string, in []string) string {
	switch kind {
	case "lines", "lines1", "until", "until1":
		return brun(kind, []byte(in[0]))
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
