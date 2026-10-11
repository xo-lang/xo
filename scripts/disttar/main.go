// Command disttar writes a release archive (decision 0157): a gzipped tar
// of a directory, the same bytes for the same files wherever and whenever
// it runs (paths in byte order, no times, owners, or names in the tar or
// gzip headers, modes 0755 for executables and 0644 for other files).
//
//	go run ./scripts/disttar -o xo-v0.9.0-darwin-arm64.tar.gz dist/xo-v0.9.0-darwin-arm64
//
// The archive's top directory is the base name of the directory given.
// scripts/release.sh runs it for each target.
package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"time"
)

func main() {
	out := flag.String("o", "", "archive to write (.tar.gz)")
	flag.Parse()
	if *out == "" || flag.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "usage: disttar -o archive.tar.gz dir")
		os.Exit(2)
	}
	data, err := archive(flag.Arg(0))
	if err == nil {
		err = os.WriteFile(*out, data, 0o644)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "disttar:", err)
		os.Exit(1)
	}
}

// archive returns the gzipped tar of dir.
func archive(dir string) ([]byte, error) {
	top := filepath.Base(filepath.Clean(dir))
	type entry struct {
		name string // slash separated, under top
		dir  bool
		exec bool
		path string
	}
	var ents []entry
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(dir, p)
		if err != nil {
			return err
		}
		name := path.Join(top, filepath.ToSlash(rel))
		switch {
		case d.IsDir():
			ents = append(ents, entry{name: name + "/", dir: true})
		case d.Type().IsRegular():
			fi, err := d.Info()
			if err != nil {
				return err
			}
			ents = append(ents, entry{name: name, exec: fi.Mode()&0o111 != 0, path: p})
		default:
			return fmt.Errorf("%s: not a regular file or directory", p)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(ents, func(i, j int) bool { return ents[i].name < ents[j].name })
	var b bytes.Buffer
	zw, err := gzip.NewWriterLevel(&b, gzip.BestCompression)
	if err != nil {
		return nil, err
	}
	// no name, comment, or modification time in the gzip header
	zw.Header = gzip.Header{OS: 255}
	tw := tar.NewWriter(zw)
	for _, e := range ents {
		h := &tar.Header{Name: e.name, ModTime: time.Unix(0, 0), Format: tar.FormatPAX}
		var data []byte
		switch {
		case e.dir:
			h.Typeflag, h.Mode = tar.TypeDir, 0o755
		default:
			data, err = os.ReadFile(e.path)
			if err != nil {
				return nil, err
			}
			h.Typeflag, h.Mode, h.Size = tar.TypeReg, 0o644, int64(len(data))
			if e.exec {
				h.Mode = 0o755
			}
		}
		if err := tw.WriteHeader(h); err != nil {
			return nil, err
		}
		if _, err := tw.Write(data); err != nil {
			return nil, err
		}
	}
	if err := tw.Close(); err != nil {
		return nil, err
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}
