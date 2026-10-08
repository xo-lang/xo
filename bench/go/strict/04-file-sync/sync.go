package main

import (
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
)

// Options configures a sync.
type Options struct {
	Src    string
	Dst    string
	DryRun bool
	Delete bool
	// CaseInsensitive makes path comparison ignore case.
	CaseInsensitive bool
	// SkipNames lists file names that are never synced or deleted.
	SkipNames []string
	// Out receives one line per action. Nil means io.Discard.
	Out io.Writer

	// copyFn copies file contents. Nil means io.Copy. Tests inject failures here.
	copyFn func(dst io.Writer, src io.Reader) (int64, error)
}

// DefaultOptions returns options with platform specific defaults: case
// insensitive comparison on macOS and Windows, and the platform junk file
// skipped.
func DefaultOptions(src, dst string) Options {
	o := Options{Src: src, Dst: dst}
	switch runtime.GOOS {
	case "darwin", "ios":
		o.CaseInsensitive = true
		o.SkipNames = []string{".DS_Store"}
	case "windows":
		o.CaseInsensitive = true
		o.SkipNames = []string{"Thumbs.db"}
	default:
		o.CaseInsensitive = false
	}
	return o
}

// Summary counts the outcome of a sync.
type Summary struct {
	Copied  int
	Deleted int
	Skipped int
	Failed  int
}

// printer writes formatted lines and remembers the first write error.
type printer struct {
	w   io.Writer
	err error
}

func (p *printer) printf(format string, args ...any) {
	if p.err != nil || p.w == nil {
		return
	}
	_, p.err = fmt.Fprintf(p.w, format, args...)
}

// dstDir wraps the destination root, which is nil only in a dry run whose
// destination does not exist yet.
type dstDir struct {
	root *os.Root
}

func (d dstDir) lstat(rel string) (fs.FileInfo, error) {
	if d.root == nil {
		return nil, fs.ErrNotExist
	}
	return d.root.Lstat(filepath.FromSlash(rel))
}

func (d dstDir) mustRoot() (*os.Root, error) {
	if d.root == nil {
		return nil, errors.New("destination is not open")
	}
	return d.root, nil
}

type syncer struct {
	opts    Options
	src     *os.Root
	dst     dstDir
	out     *printer
	sum     Summary
	srcKeys map[string]struct{}
	copyFn  func(dst io.Writer, src io.Reader) (int64, error)
}

// Sync makes opts.Dst mirror opts.Src. All reads go through an os.Root
// opened on Src and all writes through an os.Root opened on Dst, so the
// operation cannot touch anything outside those trees. A non nil error means
// the sync could not start (or its output could not be written); per file
// problems are counted in Summary.Failed instead.
func Sync(opts Options) (sum Summary, err error) {
	if err := checkOverlap(opts.Src, opts.Dst, opts.CaseInsensitive); err != nil {
		return sum, err
	}
	src, err := os.OpenRoot(opts.Src)
	if err != nil {
		return sum, fmt.Errorf("open source: %w", err)
	}
	defer func() {
		if cerr := src.Close(); cerr != nil && err == nil {
			err = cerr
		}
	}()
	info, err := src.Stat(".")
	if err != nil {
		return sum, fmt.Errorf("stat source: %w", err)
	}
	if !info.IsDir() {
		return sum, fmt.Errorf("source %q is not a directory", opts.Src)
	}

	out := &printer{w: opts.Out}
	verb := ""
	if opts.DryRun {
		verb = "would "
	}

	var dst dstDir
	root, err := os.OpenRoot(opts.Dst)
	switch {
	case err == nil:
		dst.root = root
	case errors.Is(err, fs.ErrNotExist):
		out.printf("%smkdir %s\n", verb, opts.Dst)
		if !opts.DryRun {
			if err := os.MkdirAll(opts.Dst, 0o755); err != nil {
				return sum, fmt.Errorf("create destination: %w", err)
			}
			root, err = os.OpenRoot(opts.Dst)
			if err != nil {
				return sum, fmt.Errorf("open destination: %w", err)
			}
			dst.root = root
		}
	default:
		return sum, fmt.Errorf("open destination: %w", err)
	}
	if dst.root != nil {
		r := dst.root
		defer func() {
			if cerr := r.Close(); cerr != nil && err == nil {
				err = cerr
			}
		}()
	}

	s := &syncer{
		opts:    opts,
		src:     src,
		dst:     dst,
		out:     out,
		srcKeys: make(map[string]struct{}),
		copyFn:  opts.copyFn,
	}
	if s.copyFn == nil {
		s.copyFn = io.Copy
	}
	s.copyPhase(verb)
	if opts.Delete && dst.root != nil {
		s.deletePhase(verb, dst.root)
	}
	if out.err != nil {
		return s.sum, fmt.Errorf("write output: %w", out.err)
	}
	return s.sum, nil
}

// key normalizes a slash separated relative path for comparison.
func (s *syncer) key(rel string) string {
	if s.opts.CaseInsensitive {
		return strings.ToLower(rel)
	}
	return rel
}

func (s *syncer) inSrc(rel string) bool {
	_, ok := s.srcKeys[s.key(rel)]
	return ok
}

func (s *syncer) isSkipName(name string) bool {
	for _, n := range s.opts.SkipNames {
		if n == name || (s.opts.CaseInsensitive && strings.EqualFold(n, name)) {
			return true
		}
	}
	return false
}

func (s *syncer) fail(rel string, err error) {
	s.sum.Failed++
	s.out.printf("FAIL %s: %v\n", rel, err)
}

func (s *syncer) copyPhase(verb string) {
	walkErr := fs.WalkDir(s.src.FS(), ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			s.fail(p, err)
			return nil
		}
		if p == "." {
			return nil
		}
		if s.isSkipName(d.Name()) {
			s.sum.Skipped++
			s.out.printf("skip %s (ignored name)\n", p)
			if d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		s.srcKeys[s.key(p)] = struct{}{}
		if d.IsDir() {
			return s.syncDir(p, d, verb)
		}
		if !d.Type().IsRegular() {
			s.sum.Skipped++
			s.out.printf("skip %s (not a regular file)\n", p)
			return nil
		}
		s.syncFile(p, d, verb)
		return nil
	})
	if walkErr != nil {
		s.fail(".", walkErr)
	}
}

func (s *syncer) syncDir(p string, d fs.DirEntry, verb string) error {
	dinfo, err := s.dst.lstat(p)
	if err == nil {
		if dinfo.IsDir() {
			return nil
		}
		s.fail(p, errors.New("destination exists and is not a directory"))
		return fs.SkipDir
	}
	if !errors.Is(err, fs.ErrNotExist) {
		s.fail(p, err)
		return fs.SkipDir
	}
	s.out.printf("%smkdir %s\n", verb, p)
	if s.opts.DryRun {
		return nil
	}
	perm := fs.FileMode(0o755)
	if info, err := d.Info(); err == nil {
		perm = info.Mode().Perm() | 0o700
	}
	root, err := s.dst.mustRoot()
	if err != nil {
		s.fail(p, err)
		return fs.SkipDir
	}
	if err := root.MkdirAll(filepath.FromSlash(p), perm); err != nil {
		s.fail(p, err)
		return fs.SkipDir
	}
	return nil
}

func (s *syncer) syncFile(p string, d fs.DirEntry, verb string) {
	sinfo, err := d.Info()
	if err != nil {
		s.fail(p, err)
		return
	}
	dinfo, err := s.dst.lstat(p)
	switch {
	case err == nil:
		if dinfo.IsDir() {
			s.fail(p, errors.New("destination is a directory"))
			return
		}
		if dinfo.Mode().IsRegular() && dinfo.Size() == sinfo.Size() && !sinfo.ModTime().After(dinfo.ModTime()) {
			s.sum.Skipped++
			return
		}
	case errors.Is(err, fs.ErrNotExist):
		// Missing: copy it.
	default:
		s.fail(p, err)
		return
	}
	s.out.printf("%scopy %s\n", verb, p)
	if s.opts.DryRun {
		s.sum.Copied++
		return
	}
	if err := s.copyFile(p, sinfo); err != nil {
		s.fail(p, err)
		return
	}
	s.sum.Copied++
}

// copyFile writes src/p to a temporary file next to dst/p, then renames it
// into place. The temporary file is removed on any failure.
func (s *syncer) copyFile(p string, sinfo fs.FileInfo) (err error) {
	root, err := s.dst.mustRoot()
	if err != nil {
		return err
	}
	in, err := s.src.Open(filepath.FromSlash(p))
	if err != nil {
		return err
	}
	defer func() {
		if cerr := in.Close(); cerr != nil && err == nil {
			err = cerr
		}
	}()

	tmp := filepath.FromSlash(path.Join(path.Dir(p), ".filesync-"+rand.Text()[:16]+".tmp"))
	out, err := root.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	committed := false
	closed := false
	defer func() {
		if committed {
			return
		}
		if !closed {
			err = errors.Join(err, out.Close())
		}
		if rerr := root.Remove(tmp); rerr != nil && !errors.Is(rerr, fs.ErrNotExist) {
			err = errors.Join(err, fmt.Errorf("remove temp file: %w", rerr))
		}
	}()

	if _, err := s.copyFn(out, in); err != nil {
		return err
	}
	if err := out.Sync(); err != nil {
		return err
	}
	closed = true
	if err := out.Close(); err != nil {
		return err
	}
	if err := root.Chmod(tmp, sinfo.Mode().Perm()); err != nil {
		return err
	}
	if err := root.Chtimes(tmp, sinfo.ModTime(), sinfo.ModTime()); err != nil {
		return err
	}
	if err := root.Rename(tmp, filepath.FromSlash(p)); err != nil {
		return err
	}
	committed = true
	return nil
}

type dstEntry struct {
	rel   string
	isDir bool
}

func (s *syncer) deletePhase(verb string, root *os.Root) {
	var entries []dstEntry
	blocked := make(map[string]bool)
	walkErr := fs.WalkDir(root.FS(), ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			s.fail(p, err)
			blocked[p] = true
			return nil
		}
		if p == "." {
			return nil
		}
		entries = append(entries, dstEntry{rel: p, isDir: d.IsDir()})
		return nil
	})
	if walkErr != nil {
		s.fail(".", walkErr)
		return
	}

	// Reverse of a pre-order walk visits children before their parents.
	for _, e := range slices.Backward(entries) {
		parent := path.Dir(e.rel)
		if s.isSkipName(path.Base(e.rel)) {
			// Ignored names are left alone unless their directory is going away.
			if parent == "." || s.inSrc(parent) {
				continue
			}
		} else if s.inSrc(e.rel) {
			continue
		}
		if e.isDir && blocked[e.rel] {
			blocked[parent] = true
			continue
		}
		s.out.printf("%sdelete %s\n", verb, e.rel)
		if !s.opts.DryRun {
			if err := root.Remove(filepath.FromSlash(e.rel)); err != nil {
				s.fail(e.rel, err)
				blocked[parent] = true
				continue
			}
		}
		s.sum.Deleted++
	}
}

// checkOverlap rejects a source and destination where one contains the other,
// which would make the walk read its own output or delete its own input.
func checkOverlap(src, dst string, caseInsensitive bool) error {
	rs, err := resolve(src)
	if err != nil {
		return fmt.Errorf("resolve source: %w", err)
	}
	rd, err := resolve(dst)
	if err != nil {
		return fmt.Errorf("resolve destination: %w", err)
	}
	if within(rd, rs, caseInsensitive) || within(rs, rd, caseInsensitive) {
		return fmt.Errorf("source %q and destination %q overlap", src, dst)
	}
	return nil
}

// resolve returns the absolute, symlink free form of p. Trailing components
// that do not exist yet are kept as given.
func resolve(p string) (string, error) {
	abs, err := filepath.Abs(p)
	if err != nil {
		return "", err
	}
	r, err := filepath.EvalSymlinks(abs)
	if err == nil {
		return r, nil
	}
	if !errors.Is(err, fs.ErrNotExist) {
		return "", err
	}
	parent := filepath.Dir(abs)
	if parent == abs {
		return abs, nil
	}
	rp, err := resolve(parent)
	if err != nil {
		return "", err
	}
	return filepath.Join(rp, filepath.Base(abs)), nil
}

// within reports whether child is parent or lies under it.
func within(child, parent string, caseInsensitive bool) bool {
	if caseInsensitive {
		child, parent = strings.ToLower(child), strings.ToLower(parent)
	}
	rel, err := filepath.Rel(parent, child)
	if err != nil {
		return false
	}
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)))
}
