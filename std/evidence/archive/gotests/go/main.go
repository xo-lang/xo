// Command go is the Go side of std/archive's gotests evidence (decision 0720,
// task #403): main CASES prints, for each case line `KIND<TAB>B64...`, the
// base64 of package tar's result as text;
// std/evidence/archive/gotests/main.xo prints std/archive's the same way. See
// scripts/stdproven/gotests.go.
package main

import (
	"archive/tar"
	"archive/zip"
	"bufio"
	"encoding/base64"
	"fmt"
	"io"
	"io/fs"
	"os"
	"strings"
)

// skip names the inputs the Go tests decode first (base64, bzip2).
func skip(name string) bool {
	return strings.HasSuffix(name, ".base64") || strings.HasSuffix(name, ".bz2")
}

// entry is an Entry as unzip and untar give it: a path is a Str (each
// byte that is not UTF-8 read as U+FFFD) and a directory's has no
// trailing slash.
func entry(name string, n int, dir bool) string {
	name = string([]rune(name))
	if dir {
		name = strings.TrimSuffix(name, "/")
	}
	return fmt.Sprintf("%s:%d:%v", name, n, dir)
}

func arun(kind string, in []string) string {
	if skip(in[0]) {
		return "skip"
	}
	var out []string
	switch kind {
	case "tar":
		r := tar.NewReader(strings.NewReader(in[1]))
		for {
			h, err := r.Next()
			if err == io.EOF {
				break
			}
			if err != nil {
				return "err"
			}
			// untar keeps files and directories, skips a global PAX
			// header, and refuses links, devices and the rest
			if h.Typeflag == tar.TypeXGlobalHeader {
				continue
			}
			if h.Typeflag != tar.TypeReg && h.Typeflag != tar.TypeDir {
				return "err"
			}
			b, err := io.ReadAll(r)
			if err != nil {
				return "err"
			}
			out = append(out, entry(h.Name, len(b), h.Typeflag == tar.TypeDir))
		}
	case "zip":
		r, err := zip.NewReader(strings.NewReader(in[1]), int64(len(in[1])))
		if err != nil {
			return "err"
		}
		for _, f := range r.File {
			// unzip refuses a symlink and every other kind but a file
			// or a directory
			if f.Mode()&fs.ModeType&^fs.ModeDir != 0 {
				return "err"
			}
			rc, err := f.Open()
			if err != nil {
				return "err"
			}
			b, err := io.ReadAll(rc)
			if err != nil {
				return "err"
			}
			out = append(out, entry(f.Name, len(b), f.FileInfo().IsDir()))
		}
	}
	return "ok " + strings.Join(out, "|")
}

func run(kind string, in []string) string {
	switch kind {
	case "tar", "zip":
		return arun(kind, in)
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
