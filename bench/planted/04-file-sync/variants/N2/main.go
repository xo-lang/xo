// Command filesync makes a destination directory mirror a source directory
// (one way).
//
// Usage:
//
//	filesync [--dry-run] [--delete] <src> <dst>
package main

import (
	"crypto/rand"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strings"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

// run executes the command and returns the process exit status.
func run(args []string, stdout, stderr io.Writer) int {
	fset := flag.NewFlagSet("filesync", flag.ContinueOnError)
	fset.SetOutput(stderr)
	dryRun := fset.Bool("dry-run", false, "perform no writes; print what would happen")
	del := fset.Bool("delete", false, "remove files and directories in dst that do not exist in src")
	fset.Usage = func() {
		fmt.Fprintln(stderr, "usage: filesync [--dry-run] [--delete] <src> <dst>")
		fset.PrintDefaults()
	}

	// Allow flags to appear before or after the positional arguments.
	var pos []string
	rest := args
	for {
		if err := fset.Parse(rest); err != nil {
			if errors.Is(err, flag.ErrHelp) {
				return 0
			}
			return 2
		}
		rest = fset.Args()
		if len(rest) == 0 {
			break
		}
		pos = append(pos, rest[0])
		rest = rest[1:]
	}
	if len(pos) != 2 {
		fset.Usage()
		return 1
	}

	opt := DefaultOptions()
	opt.DryRun = *dryRun
	opt.Delete = *del
	opt.Out = stdout
	opt.Err = stderr

	sum, err := Sync(pos[0], pos[1], opt)
	if err != nil {
		fmt.Fprintf(stderr, "filesync: %v\n", err)
		return 1
	}
	prefix := ""
	if opt.DryRun {
		prefix = "dry run: "
	}
	fmt.Fprintf(stdout, "%scopied: %d, deleted: %d, skipped: %d, failed: %d\n",
		prefix, sum.Copied, sum.Deleted, sum.Skipped, sum.Failed)
	if sum.Failed > 0 {
		return 1
	}
	return 0
}

// Options controls a sync.
type Options struct {
	DryRun          bool
	Delete          bool
	CaseInsensitive bool     // compare paths case insensitively
	SkipNames       []string // base names that are never synced or deleted
	Out             io.Writer
	Err             io.Writer

	// copyFn copies file data. Tests replace it to inject failures.
	copyFn func(dst io.Writer, src io.Reader) (int64, error)
}

// DefaultOptions returns options appropriate for the running platform.
func DefaultOptions() Options {
	opt := Options{Out: io.Discard, Err: io.Discard}
	switch runtime.GOOS {
	case "darwin", "ios":
		opt.CaseInsensitive = true
		opt.SkipNames = []string{".DS_Store"}
	case "windows":
		opt.CaseInsensitive = true
		opt.SkipNames = []string{"Thumbs.db"}
	}
	return opt
}

// Summary counts the outcome of a sync.
type Summary struct {
	Copied  int
	Deleted int
	Skipped int
	Failed  int
}

const tempPrefix = ".filesync-"

// Sync makes dst mirror src. A non-nil error means the sync could not start;
// per file failures are counted in Summary.Failed.
func Sync(src, dst string, opt Options) (Summary, error) {
	if opt.Out == nil {
		opt.Out = io.Discard
	}
	if opt.Err == nil {
		opt.Err = io.Discard
	}
	if opt.copyFn == nil {
		opt.copyFn = io.Copy
	}

	if err := checkOverlap(src, dst, opt.CaseInsensitive); err != nil {
		return Summary{}, err
	}

	srcRoot, err := os.OpenRoot(src)
	if err != nil {
		return Summary{}, fmt.Errorf("open source: %w", err)
	}
	defer srcRoot.Close()

	dstRoot, err := openDst(dst, opt.DryRun)
	if err != nil {
		return Summary{}, err
	}
	if dstRoot != nil {
		defer dstRoot.Close()
	}

	s := &syncer{src: srcRoot, dst: dstRoot, opt: opt, seen: map[string]bool{}}
	s.copyTree()
	if opt.Delete {
		s.deleteExtra()
	}
	return s.sum, nil
}

// openDst opens the destination root, creating it unless this is a dry run.
// In a dry run a missing destination yields a nil root (treated as empty).
func openDst(dst string, dryRun bool) (*os.Root, error) {
	if !dryRun {
		if err := os.MkdirAll(dst, 0o755); err != nil {
			return nil, fmt.Errorf("create destination: %w", err)
		}
	}
	r, err := os.OpenRoot(dst)
	if err != nil {
		if dryRun && errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("open destination: %w", err)
	}
	return r, nil
}

// checkOverlap rejects src and dst that are the same directory or nested in
// each other, which would make the walk see its own output.
func checkOverlap(src, dst string, caseInsensitive bool) error {
	a, err := resolve(src)
	if err != nil {
		return err
	}
	b, err := resolve(dst)
	if err != nil {
		return err
	}
	if caseInsensitive {
		a, b = strings.ToLower(a), strings.ToLower(b)
	}
	if within(a, b) || within(b, a) {
		return fmt.Errorf("source %q and destination %q overlap", src, dst)
	}
	return nil
}

// within reports whether child is parent or below it.
func within(parent, child string) bool {
	rel, err := filepath.Rel(parent, child)
	if err != nil {
		return false
	}
	if rel == "." {
		return true
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// resolve returns an absolute path with symlinks evaluated for the longest
// existing prefix.
func resolve(p string) (string, error) {
	abs, err := filepath.Abs(p)
	if err != nil {
		return "", err
	}
	var tail []string
	cur := abs
	for {
		if r, err := filepath.EvalSymlinks(cur); err == nil {
			parts := append([]string{r}, tail...)
			return filepath.Join(parts...), nil
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			return abs, nil
		}
		tail = append([]string{filepath.Base(cur)}, tail...)
		cur = parent
	}
}

type syncer struct {
	src, dst *os.Root // dst is nil in a dry run against a missing destination
	opt      Options
	sum      Summary
	seen     map[string]bool // keys of every source path that was walked
	// incomplete is set when part of the source tree could not be walked;
	// deletion is then unsafe because missing entries are unknown.
	incomplete bool
}

func (s *syncer) key(p string) string {
	if s.opt.CaseInsensitive {
		return strings.ToLower(p)
	}
	return p
}

func (s *syncer) skipName(name string) bool {
	for _, n := range s.opt.SkipNames {
		if name == n || (s.opt.CaseInsensitive && strings.EqualFold(name, n)) {
			return true
		}
	}
	return false
}

func (s *syncer) logf(format string, args ...any) {
	fmt.Fprintf(s.opt.Out, format+"\n", args...)
}

func (s *syncer) fail(p string, err error) {
	s.sum.Failed++
	fmt.Fprintf(s.opt.Err, "error: %s: %v\n", p, err)
}

func (s *syncer) copyTree() {
	_ = fs.WalkDir(s.src.FS(), ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			s.incomplete = true
			s.fail(p, err)
			return nil
		}
		if p == "." {
			return nil
		}
		if s.skipName(d.Name()) {
			s.sum.Skipped++
			s.logf("skip %s (ignored name)", p)
			if d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		s.seen[s.key(p)] = true
		switch {
		case d.IsDir():
			if !s.ensureDir(p, d) {
				s.incomplete = true
				return fs.SkipDir
			}
		case d.Type().IsRegular():
			s.syncFile(p, d)
		default:
			s.sum.Skipped++
			s.logf("skip %s (not a regular file)", p)
		}
		return nil
	})
}

// ensureDir makes sure directory p exists in dst. It returns false if the
// subtree should not be descended into.
func (s *syncer) ensureDir(p string, d fs.DirEntry) bool {
	op := filepath.FromSlash(p)
	if s.dst != nil {
		fi, err := s.dst.Lstat(op)
		if err == nil {
			if fi.IsDir() {
				return true
			}
			s.fail(p, errors.New("destination exists and is not a directory"))
			return false
		}
		if !errors.Is(err, fs.ErrNotExist) {
			s.fail(p, err)
			return false
		}
	}
	if s.opt.DryRun {
		s.logf("mkdir %s", p)
		return true
	}
	perm := fs.FileMode(0o755)
	if info, err := d.Info(); err == nil {
		perm = info.Mode().Perm() | 0o700
	}
	if err := s.dst.MkdirAll(op, perm); err != nil {
		s.fail(p, err)
		return false
	}
	s.logf("mkdir %s", p)
	return true
}

func (s *syncer) syncFile(p string, d fs.DirEntry) {
	op := filepath.FromSlash(p)
	si, err := d.Info()
	if err != nil {
		s.fail(p, err)
		return
	}
	if s.dst != nil {
		di, err := s.dst.Lstat(op)
		switch {
		case err == nil:
			if di.IsDir() {
				s.fail(p, errors.New("destination is a directory"))
				return
			}
			if di.Mode().IsRegular() && di.Size() == si.Size() && !si.ModTime().After(di.ModTime()) {
				s.sum.Skipped++
				s.logf("skip %s (unchanged)", p)
				return
			}
		case !errors.Is(err, fs.ErrNotExist):
			s.fail(p, err)
			return
		}
	}
	if s.opt.DryRun {
		s.sum.Copied++
		s.logf("copy %s", p)
		return
	}
	if err := s.copyFile(op, si); err != nil {
		s.fail(p, err)
		return
	}
	s.sum.Copied++
	s.logf("copy %s", p)
}

// copyFile writes src file op to a temp file next to its destination and
// renames it into place. The temp file is removed on any failure.
func (s *syncer) copyFile(op string, si fs.FileInfo) (err error) {
	in, err := s.src.Open(op)
	if err != nil {
		return err
	}
	defer in.Close()

	dir := filepath.Dir(op)
	var tmp string
	var out *os.File
	for range 10 {
		tmp = filepath.Join(dir, tempPrefix+rand.Text()+".tmp")
		out, err = s.dst.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
		if err == nil || !errors.Is(err, fs.ErrExist) {
			break
		}
	}
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	closed := false
	defer func() {
		if err != nil {
			if !closed {
				out.Close()
			}
			if rmErr := s.dst.Remove(tmp); rmErr != nil && !errors.Is(rmErr, fs.ErrNotExist) {
				fmt.Fprintf(s.opt.Err, "error: remove temp file %s: %v\n", tmp, rmErr)
			}
		}
	}()

	if _, err = s.opt.copyFn(out, in); err != nil {
		return fmt.Errorf("copy: %w", err)
	}
	if err = out.Sync(); err != nil {
		return fmt.Errorf("sync: %w", err)
	}
	closed = true
	if err = out.Close(); err != nil {
		return fmt.Errorf("close: %w", err)
	}
	if err = s.dst.Chmod(tmp, si.Mode().Perm()); err != nil {
		return fmt.Errorf("chmod: %w", err)
	}
	if err = s.dst.Chtimes(tmp, si.ModTime(), si.ModTime()); err != nil {
		return fmt.Errorf("chtimes: %w", err)
	}
	if err = s.dst.Rename(tmp, op); err != nil {
		return fmt.Errorf("rename: %w", err)
	}
	return nil
}

// deleteExtra removes dst entries that have no counterpart in src, deepest
// first so that directories are empty when removed.
func (s *syncer) deleteExtra() {
	if s.dst == nil {
		return
	}
	if s.incomplete {
		fmt.Fprintln(s.opt.Err, "warning: source walk incomplete, not deleting anything")
		return
	}
	type entry struct {
		p   string
		dir bool
	}
	var entries []entry
	_ = fs.WalkDir(s.dst.FS(), ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			s.fail(p, err)
			return nil
		}
		if p == "." {
			return nil
		}
		entries = append(entries, entry{p, d.IsDir()})
		return nil
	})
	for i := len(entries) - 1; i >= 0; i-- {
		e := entries[i]
		if s.seen[s.key(e.p)] {
			continue
		}
		// Ignored names are left alone unless their directory is going away,
		// in which case they would otherwise keep it from being removed.
		if s.skipName(path.Base(e.p)) && !s.parentExtra(e.p) {
			continue
		}
		if s.opt.DryRun {
			s.sum.Deleted++
			s.logf("delete %s", e.p)
			continue
		}
		if err := s.dst.Remove(filepath.FromSlash(e.p)); err != nil {
			s.fail(e.p, err)
			continue
		}
		s.sum.Deleted++
		s.logf("delete %s", e.p)
	}
}

func (s *syncer) parentExtra(p string) bool {
	dir := path.Dir(p)
	return dir != "." && !s.seen[s.key(dir)]
}
