package mod

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/xo-lang/xo/internal/diag"
)

// HashDir is the content hash of a module tree (core 4.3 [P1]): "h1:"
// and the base64 SHA-256 of the lines "<sha256 hex of file>  <slash
// path>\n" of every regular file, sorted by path. The same tree hashes
// the same in the cache and in vendor/.
func HashDir(dir string) (string, error) {
	var files []string
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() && p != dir {
			// a nested module is not part of this one (a vendored
			// github.com/acme/tools holds vendored .../tools/cli)
			if _, err := os.Stat(filepath.Join(p, "xo.mod")); err == nil {
				return filepath.SkipDir
			}
		}
		if d.Type().IsRegular() {
			rel, err := filepath.Rel(dir, p)
			if err != nil {
				return err
			}
			files = append(files, filepath.ToSlash(rel))
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	sort.Strings(files)
	h := sha256.New()
	for _, f := range files {
		data, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(f)))
		if err != nil {
			return "", err
		}
		fmt.Fprintf(h, "%x  %s\n", sha256.Sum256(data), f)
	}
	return "h1:" + base64.StdEncoding.EncodeToString(h.Sum(nil)), nil
}

// HashModFile is the hash of a module version's xo.mod alone, in the
// HashDir form with the single path "xo.mod".
func HashModFile(dir string) (string, error) {
	data, err := os.ReadFile(filepath.Join(dir, "xo.mod"))
	if err != nil {
		return "", err
	}
	h := sha256.New()
	fmt.Fprintf(h, "%x  %s\n", sha256.Sum256(data), "xo.mod")
	return "h1:" + base64.StdEncoding.EncodeToString(h.Sum(nil)), nil
}

// Lock is a parsed xo.lock: "<path> <version> <hash>" for the tree of
// every selected module version and "<path> <version>/xo.mod <hash>" for
// the xo.mod of every version the selection read.
type Lock struct {
	File    string
	Entries map[string]string // "<path> <version>" or "<path> <version>/xo.mod" -> hash
	Lines   map[string]int
}

const lockHeader = "# xo.lock: content hashes of the module versions this module builds with.\n# Written by the xo toolchain (xo get, xo mod tidy); do not edit (core 4.3).\n"

// ReadLock reads dir/xo.lock; a missing file is an empty lock.
func ReadLock(dir string) (*Lock, []*Problem) {
	l := &Lock{File: filepath.Join(dir, "xo.lock"), Entries: map[string]string{}, Lines: map[string]int{}}
	data, err := os.ReadFile(l.File)
	if err != nil {
		return l, nil
	}
	var probs []*Problem
	sc := bufio.NewScanner(bytes.NewReader(data))
	line := 0
	for sc.Scan() {
		line++
		text := strings.TrimSpace(sc.Text())
		if text == "" || strings.HasPrefix(text, "#") {
			continue
		}
		f := strings.Fields(text)
		if len(f) != 3 || !strings.HasPrefix(f[2], "h1:") {
			probs = append(probs, &Problem{Code: diag.ModFileInvalid, File: l.File, Line: line, Msg: "malformed xo.lock line: want `<module path> <version>[/xo.mod] h1:<hash>`", Notes: []string{"xo.lock is written by the toolchain; `xo mod tidy` rewrites it"}})
			continue
		}
		key := f[0] + " " + f[1]
		l.Entries[key] = f[2]
		l.Lines[key] = line
	}
	return l, probs
}

// WriteLock writes dir/xo.lock with the given entries, sorted.
func WriteLock(dir string, entries map[string]string) error {
	var keys []string
	for k := range entries {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		pi, vi, _ := strings.Cut(keys[i], " ")
		pj, vj, _ := strings.Cut(keys[j], " ")
		if pi != pj {
			return pi < pj
		}
		bi, bj := strings.TrimSuffix(vi, "/xo.mod"), strings.TrimSuffix(vj, "/xo.mod")
		if bi != bj {
			return Compare(bi, bj) < 0
		}
		return vi < vj
	})
	var b strings.Builder
	b.WriteString(lockHeader)
	for _, k := range keys {
		b.WriteString(k + " " + entries[k] + "\n")
	}
	name := filepath.Join(dir, "xo.lock")
	if len(keys) == 0 {
		if _, err := os.Stat(name); err != nil {
			return nil // nothing required and no lock: do not create one
		}
	}
	return os.WriteFile(name, []byte(b.String()), 0o644)
}
