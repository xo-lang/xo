package rt

import (
	"bufio"
	"errors"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// Files (std.md 3.1, decision 0163): streaming read and write handles,
// append, copy, permission bits, symbolic links, globs, and advisory
// locks, on the real filesystem (this file) and MemFs (fsfile_mem.go).
// The platform's lock calls are in flock_*.go.

// File is `File`: an open file (a handle).
type File struct{ h *fileH }

type fileH struct {
	mu     sync.Mutex
	path   string // as given, for errors
	write  bool   // opened for writing (else reading)
	closed bool
	osf    *os.File
	r      *bufio.Reader
	mem    *memFileH
}

func (File) XoDebug(b *strings.Builder) { b.WriteString("<file>") }
func (f File) M_debug(*Task) string     { return Debug(f) }

// FileLock is `FileLock`: a held advisory lock (a handle).
type FileLock struct{ l *flockH }

type flockH struct {
	mu   sync.Mutex
	osf  *os.File
	mem  *lock
	done bool
}

func (FileLock) XoDebug(b *strings.Builder) { b.WriteString("<file lock>") }
func (l FileLock) M_debug(*Task) string     { return Debug(l) }

// M_unlock releases the lock; a second unlock does nothing.
func (l FileLock) M_unlock(*Task) {
	h := l.l
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.done {
		return
	}
	h.done = true
	if h.mem != nil {
		h.mem.release()
		return
	}
	unlockFile(h.osf)
	h.osf.Close()
}

func fileClosed(p string) FsErr { return fsIo(p, "file already closed") }

func (f File) M_read(tk *Task, max int64) Result[Option[Bytes], FsErr] {
	h := f.h
	h.mu.Lock()
	defer h.mu.Unlock()
	switch {
	case tk.Canceled():
		return Err[Option[Bytes]](FsCanceled)
	case h.closed:
		return Err[Option[Bytes]](fileClosed(h.path))
	case h.write:
		return Err[Option[Bytes]](fsIo(h.path, "file is open for writing"))
	case max <= 0:
		return Ok[Option[Bytes], FsErr](Some(Bytes("")))
	}
	if h.mem != nil {
		return h.mem.read(max)
	}
	buf := make([]byte, min(max, 1<<20))
	n, err := h.r.Read(buf)
	if n > 0 {
		return Ok[Option[Bytes], FsErr](Some(Bytes(buf[:n])))
	}
	if err == nil || errors.Is(err, io.EOF) {
		return Ok[Option[Bytes], FsErr](Option[Bytes]{})
	}
	return Err[Option[Bytes]](mapFsErr(err, h.path))
}

func (f File) M_read_line(tk *Task) Result[Option[string], FsErr] {
	h := f.h
	h.mu.Lock()
	defer h.mu.Unlock()
	switch {
	case tk.Canceled():
		return Err[Option[string]](FsCanceled)
	case h.closed:
		return Err[Option[string]](fileClosed(h.path))
	case h.write:
		return Err[Option[string]](fsIo(h.path, "file is open for writing"))
	}
	if h.mem != nil {
		return h.mem.readLine()
	}
	line, err := h.r.ReadString('\n')
	if line == "" && err != nil {
		if errors.Is(err, io.EOF) {
			return Ok[Option[string], FsErr](Option[string]{})
		}
		return Err[Option[string]](mapFsErr(err, h.path))
	}
	return Ok[Option[string], FsErr](Some(trimLine(line)))
}

func trimLine(s string) string { return strings.TrimSuffix(strings.TrimSuffix(s, "\n"), "\r") }

func (f File) M_write(tk *Task, data Bytes) Result[Unit, FsErr] {
	h := f.h
	h.mu.Lock()
	defer h.mu.Unlock()
	switch {
	case tk.Canceled():
		return Err[Unit](FsCanceled)
	case h.closed:
		return Err[Unit](fileClosed(h.path))
	case !h.write:
		return Err[Unit](fsIo(h.path, "file is open for reading"))
	}
	if h.mem != nil {
		h.mem.write(data)
		return Ok[Unit, FsErr](Unit{})
	}
	if _, err := h.osf.Write([]byte(data)); err != nil {
		return Err[Unit](mapFsErr(err, h.path))
	}
	return Ok[Unit, FsErr](Unit{})
}

// M_close closes the file; a second close does nothing.
func (f File) M_close(*Task) Result[Unit, FsErr] {
	h := f.h
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.closed {
		return Ok[Unit, FsErr](Unit{})
	}
	h.closed = true
	if h.mem != nil {
		h.mem.close()
		return Ok[Unit, FsErr](Unit{})
	}
	if err := h.osf.Close(); err != nil {
		return Err[Unit](mapFsErr(err, h.path))
	}
	return Ok[Unit, FsErr](Unit{})
}

// ---------------------------------------------------------------------------
// The real filesystem

func (f *realFs) openFile(tk *Task, p string, write bool, flag int) Result[File, FsErr] {
	op, e := f.check(tk, p, write)
	if e != nil {
		return Err[File](*e)
	}
	if !write {
		if fi, err := os.Stat(op); err == nil && fi.IsDir() {
			return Err[File](fsErr(5, p))
		}
	}
	osf, err := os.OpenFile(op, flag, 0o666)
	if err != nil {
		return Err[File](mapFsErr(err, p))
	}
	h := &fileH{path: p, write: write, osf: osf}
	if !write {
		h.r = bufio.NewReaderSize(osf, 64<<10)
	}
	return Ok[File, FsErr](File{h})
}

func (f *realFs) M_open(tk *Task, p string) Result[File, FsErr] {
	return f.openFile(tk, p, false, os.O_RDONLY)
}

func (f *realFs) M_create(tk *Task, p string) Result[File, FsErr] {
	return f.openFile(tk, p, true, os.O_WRONLY|os.O_CREATE|os.O_TRUNC)
}

func (f *realFs) M_open_append(tk *Task, p string) Result[File, FsErr] {
	return f.openFile(tk, p, true, os.O_WRONLY|os.O_CREATE|os.O_APPEND)
}

func (f *realFs) M_append(tk *Task, p string, data Bytes) Result[Unit, FsErr] {
	return fx(tk, "fs.append", false, func() []any { return []any{p, data} }, func() Result[Unit, FsErr] {
		r := f.M_open_append(tk, p)
		if !r.Ok {
			return Err[Unit](r.E)
		}
		if w := r.V.M_write(tk, data); !w.Ok {
			r.V.M_close(tk)
			return w
		}
		return r.V.M_close(tk)
	})
}

func (f *realFs) M_copy(tk *Task, from, to string) Result[Unit, FsErr] {
	return fx(tk, "fs.copy", false, func() []any { return []any{from, to} }, func() Result[Unit, FsErr] {
		src, e := f.check(tk, from, false)
		if e != nil {
			return Err[Unit](*e)
		}
		dst, e := f.check(tk, to, true)
		if e != nil {
			return Err[Unit](*e)
		}
		in, err := os.Open(src)
		if err != nil {
			return Err[Unit](mapFsErr(err, from))
		}
		defer in.Close()
		fi, err := in.Stat()
		if err != nil {
			return Err[Unit](mapFsErr(err, from))
		}
		if fi.IsDir() {
			return Err[Unit](fsErr(5, from))
		}
		out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, fi.Mode().Perm())
		if err != nil {
			return Err[Unit](mapFsErr(err, to))
		}
		if _, err := io.Copy(out, in); err != nil {
			out.Close()
			return Err[Unit](mapFsErr(err, to))
		}
		if err := out.Close(); err != nil {
			return Err[Unit](mapFsErr(err, to))
		}
		// the permission bits of the source, also over an existing file
		os.Chmod(dst, fi.Mode().Perm())
		return Ok[Unit, FsErr](Unit{})
	})
}

func (f *realFs) M_mode(tk *Task, p string) Result[int64, FsErr] {
	return fx(tk, "fs.mode", false, func() []any { return []any{p} }, func() Result[int64, FsErr] {
		op, e := f.check(tk, p, false)
		if e != nil {
			return Err[int64](*e)
		}
		fi, err := os.Stat(op)
		if err != nil {
			return Err[int64](mapFsErr(err, p))
		}
		return Ok[int64, FsErr](int64(fi.Mode().Perm()))
	})
}

func (f *realFs) M_set_mode(tk *Task, p string, mode int64) Result[Unit, FsErr] {
	return fx(tk, "fs.set_mode", false, func() []any { return []any{p, mode} }, func() Result[Unit, FsErr] {
		op, e := f.check(tk, p, true)
		if e != nil {
			return Err[Unit](*e)
		}
		if err := os.Chmod(op, fs.FileMode(mode&0o777)); err != nil {
			return Err[Unit](mapFsErr(err, p))
		}
		return Ok[Unit, FsErr](Unit{})
	})
}

// linkEscapes reports whether a link at link (a path in a sub capability)
// to target would point outside the capability's root: an absolute
// target, or a relative one with more `..` than the link has parents.
func linkEscapes(link, target string) bool {
	target = strings.ReplaceAll(target, "\\", "/")
	if strings.HasPrefix(target, "/") || len(target) >= 2 && target[1] == ':' {
		return true
	}
	dir := path.Dir(path.Clean("/" + strings.ReplaceAll(link, "\\", "/")))
	depth := 0
	for _, seg := range strings.Split(strings.Trim(dir, "/")+"/"+target, "/") {
		switch seg {
		case "..":
			depth--
		case "", ".":
		default:
			depth++
		}
		if depth < 0 {
			return true
		}
	}
	return false
}

func (f *realFs) M_symlink(tk *Task, target, link string) Result[Unit, FsErr] {
	return fx(tk, "fs.symlink", false, func() []any { return []any{target, link} }, func() Result[Unit, FsErr] {
		op, e := f.check(tk, link, true)
		if e != nil {
			return Err[Unit](*e)
		}
		if f.sub && linkEscapes(link, target) {
			return Err[Unit](fsErr(6, link))
		}
		if err := os.Symlink(filepath.FromSlash(target), op); err != nil {
			return Err[Unit](mapFsErr(err, link))
		}
		return Ok[Unit, FsErr](Unit{})
	})
}

func (f *realFs) M_read_link(tk *Task, p string) Result[Option[string], FsErr] {
	return fx(tk, "fs.read_link", false, func() []any { return []any{p} }, func() Result[Option[string], FsErr] {
		op, e := f.check(tk, p, false)
		if e != nil {
			return Err[Option[string]](*e)
		}
		fi, err := os.Lstat(op)
		if err != nil {
			return Err[Option[string]](mapFsErr(err, p))
		}
		if fi.Mode()&fs.ModeSymlink == 0 {
			return Ok[Option[string], FsErr](Option[string]{})
		}
		t, err := os.Readlink(op)
		if err != nil {
			return Err[Option[string]](mapFsErr(err, p))
		}
		return Ok[Option[string], FsErr](Some(filepath.ToSlash(t)))
	})
}

func (f *realFs) M_glob(tk *Task, pattern string) Result[List[string], FsErr] {
	return fx(tk, "fs.glob", false, func() []any { return []any{pattern} }, func() Result[List[string], FsErr] {
		return fsGlob(tk, f, pattern)
	})
}

// lockPoll is how often a waiting lock tries again (it stays a
// cancellation point).
const lockPoll = 10 * time.Millisecond

func (f *realFs) lockFile(tk *Task, p string, wait bool) Result[Option[FileLock], FsErr] {
	return f.lockFileMode(tk, p, wait, false)
}

// lockFileMode takes the lock exclusively, or shared when shared
// (std/internal's lock_shared, decision 0214).
func (f *realFs) lockFileMode(tk *Task, p string, wait, shared bool) Result[Option[FileLock], FsErr] {
	op, e := f.check(tk, p, true)
	if e != nil {
		return Err[Option[FileLock]](*e)
	}
	osf, err := os.OpenFile(op, os.O_RDWR|os.O_CREATE, 0o666)
	if err != nil {
		return Err[Option[FileLock]](mapFsErr(err, p))
	}
	for {
		ok, err := tryLockFile(osf, shared)
		if err != nil {
			osf.Close()
			return Err[Option[FileLock]](mapFsErr(err, p))
		}
		if ok {
			return Ok[Option[FileLock], FsErr](Some(FileLock{&flockH{osf: osf}}))
		}
		if !wait {
			osf.Close()
			return Ok[Option[FileLock], FsErr](Option[FileLock]{})
		}
		tk.leave()
		select {
		case <-tk.Done():
		case <-time.After(lockPoll):
		}
		tk.enter()
		if tk.Canceled() {
			osf.Close()
			return Err[Option[FileLock]](FsCanceled)
		}
	}
}

func (f *realFs) M_lock(tk *Task, p string) Result[FileLock, FsErr] {
	r := f.lockFile(tk, p, true)
	if !r.Ok {
		return Err[FileLock](r.E)
	}
	return Ok[FileLock, FsErr](r.V.V)
}

func (f *realFs) M_try_lock(tk *Task, p string) Result[Option[FileLock], FsErr] {
	return f.lockFile(tk, p, false)
}

// ---------------------------------------------------------------------------
// Globs: on any Fs, through read_dir

// fsGlob is `f.glob(pattern)`: the paths (relative to f's root, as given
// in the pattern, without a leading "./") that match pattern, sorted. A
// segment `**` matches any number of directories (zero included); other
// segments use `*` (any run of characters but `/`), `?`, and `[...]`
// classes (path.Match). Entries whose names start with `.` match only a
// segment that starts with `.`.
func fsGlob(tk *Task, f Fs, pattern string) Result[List[string], FsErr] {
	pattern = strings.ReplaceAll(pattern, "\\", "/")
	abs := strings.HasPrefix(pattern, "/")
	segs := strings.Split(strings.Trim(pattern, "/"), "/")
	for _, s := range segs {
		if _, err := path.Match(s, ""); err != nil {
			return Err[List[string]](fsIo(pattern, "bad pattern: "+err.Error()))
		}
	}
	start := ""
	if abs {
		start = "/"
	}
	seen := map[string]bool{}
	var out []string
	var walk func(dir string, i int) *FsErr
	add := func(p string) {
		if !seen[p] {
			seen[p] = true
			out = append(out, p)
		}
	}
	join := func(dir, name string) string {
		if dir == "" {
			return name
		}
		if strings.HasSuffix(dir, "/") {
			return dir + name
		}
		return dir + "/" + name
	}
	list := func(dir string) ([]FileInfo, *FsErr) {
		d := dir
		if d == "" {
			d = "."
		}
		r := f.M_read_dir(tk, d)
		if !r.Ok {
			if r.E.Tag == 1 || r.E.Tag == 4 {
				return nil, nil // a missing directory matches nothing
			}
			e := r.E
			return nil, &e
		}
		return r.V.s, nil
	}
	walk = func(dir string, i int) *FsErr {
		if i == len(segs) {
			add(dir)
			return nil
		}
		s := segs[i]
		if s == "**" {
			if e := walk(dir, i+1); e != nil {
				return e
			}
			ents, e := list(dir)
			if e != nil {
				return e
			}
			for _, x := range ents {
				if x.F_is_dir && !strings.HasPrefix(x.F_name, ".") {
					if e := walk(join(dir, x.F_name), i); e != nil {
						return e
					}
				}
			}
			return nil
		}
		if !strings.ContainsAny(s, "*?[\\") {
			// a literal segment: no listing needed
			p := join(dir, s)
			if i == len(segs)-1 {
				r := f.M_exists(tk, p)
				if !r.Ok {
					e := r.E
					return &e
				}
				if r.V {
					add(p)
				}
				return nil
			}
			return walk(p, i+1)
		}
		ents, e := list(dir)
		if e != nil {
			return e
		}
		for _, x := range ents {
			if strings.HasPrefix(x.F_name, ".") && !strings.HasPrefix(s, ".") {
				continue
			}
			if ok, _ := path.Match(s, x.F_name); !ok {
				continue
			}
			if i < len(segs)-1 && !x.F_is_dir {
				continue
			}
			if e := walk(join(dir, x.F_name), i+1); e != nil {
				return e
			}
		}
		return nil
	}
	if e := walk(start, 0); e != nil {
		return Err[List[string]](*e)
	}
	sort.Strings(out)
	return Ok[List[string], FsErr](ListOf(out...))
}

// ---------------------------------------------------------------------------
// Env.config_dir and Env.cache_dir (std.md 3.5, decision 0163)

// userDir is the configuration (cache false) or cache directory read
// through e: the XDG variable when it holds an absolute path, else the
// OS's place under the home directory (%AppData% and %LocalAppData% on
// Windows). Paths use `/`.
func userDir(tk *Task, e Env, cache bool) Result[string, EnvErr] {
	xdg, win, mac, unix := "XDG_CONFIG_HOME", "AppData", "/Library/Application Support", "/.config"
	if cache {
		xdg, win, mac, unix = "XDG_CACHE_HOME", "LocalAppData", "/Library/Caches", "/.cache"
	}
	if v := e.M_get(tk, xdg); v.Some && (strings.HasPrefix(v.V, "/") || len(v.V) >= 3 && v.V[1] == ':') {
		return Ok[string, EnvErr](filepath.ToSlash(v.V))
	}
	switch runtimeGOOS() {
	case "windows":
		v := e.M_get(tk, win)
		if !v.Some || v.V == "" {
			return Err[string](EnvErr{Tag: 1, P: EnvErr_Missing{F_name: win}})
		}
		return Ok[string, EnvErr](strings.ReplaceAll(v.V, "\\", "/"))
	case "darwin", "ios":
		unix = mac
	}
	h := e.M_home(tk)
	if !h.Ok {
		return h
	}
	return Ok[string, EnvErr](strings.TrimSuffix(filepath.ToSlash(h.V), "/") + unix)
}

func (e realEnv) M_config_dir(tk *Task) Result[string, EnvErr]  { return userDir(tk, e, false) }
func (e realEnv) M_cache_dir(tk *Task) Result[string, EnvErr]   { return userDir(tk, e, true) }
func (e *onlyEnv) M_config_dir(tk *Task) Result[string, EnvErr] { return userDir(tk, e, false) }
func (e *onlyEnv) M_cache_dir(tk *Task) Result[string, EnvErr]  { return userDir(tk, e, true) }
func (f Testing_FakeEnv) M_config_dir(tk *Task) Result[string, EnvErr] {
	return userDir(tk, f, false)
}
func (f Testing_FakeEnv) M_cache_dir(tk *Task) Result[string, EnvErr] { return userDir(tk, f, true) }
