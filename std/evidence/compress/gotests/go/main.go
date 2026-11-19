// Command go is the Go side of std/compress's gotests evidence (decision 0720,
// task #403): main CASES prints, for each case line `KIND<TAB>B64...`, the
// base64 of package flate's result as text;
// std/evidence/compress/gotests/main.xo prints std/compress's the same way. See
// scripts/stdproven/gotests.go.
package main

import (
	"bufio"
	"bytes"
	"compress/flate"
	"compress/gzip"
	"compress/lzw"
	"compress/zlib"
	"encoding/base64"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
)

func bytesOf(s string) []byte {
	var b []byte
	if s != "" {
		for _, p := range strings.Split(s, ",") {
			n, _ := strconv.Atoi(p)
			b = append(b, byte(n))
		}
	}
	return b
}

func bytesText(b []byte) string {
	s := make([]string, len(b))
	for i, c := range b {
		s[i] = strconv.Itoa(int(c))
	}
	return strings.Join(s, ",")
}

func out(b []byte, err error) string {
	if err != nil {
		return "err " + err.Error()
	}
	return "ok " + bytesText(b)
}

func crun(kind string, in []string) string {
	switch kind {
	case "zlib":
		var r io.ReadCloser
		var err error
		if d := bytesOf(in[1]); len(d) > 0 {
			r, err = zlib.NewReaderDict(strings.NewReader(in[0]), d)
		} else {
			r, err = zlib.NewReader(strings.NewReader(in[0]))
		}
		if err != nil {
			return out(nil, err)
		}
		return out(io.ReadAll(r))
	case "gunzip":
		r, err := gzip.NewReader(bytes.NewReader(bytesOf(in[0])))
		if err != nil {
			return out(nil, err)
		}
		return out(io.ReadAll(r))
	case "inflate":
		// Go's reader gives the bytes before the error; the error is the
		// result.
		b, err := io.ReadAll(flate.NewReader(strings.NewReader(in[0])))
		if err != nil {
			return out(nil, err)
		}
		return out(b, nil)
	case "deflate":
		n, _ := strconv.Atoi(in[1])
		var buf bytes.Buffer
		w, _ := flate.NewWriter(&buf, n)
		w.Write(bytesOf(in[0]))
		w.Close()
		return out(buf.Bytes(), nil)
	case "lzw":
		// desc is "name;LSB|MSB;width"
		p := strings.Split(in[0], ";")
		order := lzw.LSB
		if p[1] == "MSB" {
			order = lzw.MSB
		}
		w, _ := strconv.Atoi(p[2])
		return out(io.ReadAll(lzw.NewReader(strings.NewReader(in[1]), order, w)))
	}
	return "?"
}

func run(kind string, in []string) string {
	switch kind {
	case "zlib", "gunzip", "inflate", "deflate", "lzw":
		return crun(kind, in)
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
