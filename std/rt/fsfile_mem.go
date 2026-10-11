package rt

import (
	"path"
	"strings"
)

// Files on a test's MemFs (std.md 3.1, 8.3, decision 0163): handles read a
// snapshot taken at open and write through to the file at once; symbolic
// links are followed by read, read_text, open, copy, stat, and exists;
// locks are held in the test's memory and wait like a semaphore.

// memFileH is an open MemFs file.
type memFileH struct {
	v    *memView
	full string
	data string // reads: the contents at open
	off  int
}

func (m *memFileH) read(max int64) Result[Option[Bytes], FsErr] {
	if m.off >= len(m.data) {
		return Ok[Option[Bytes], FsErr](Option[Bytes]{})
	}
	n := min(int64(len(m.data)-m.off), max, 1<<20)
	b := m.data[m.off : m.off+int(n)]
	m.off += int(n)
	return Ok[Option[Bytes], FsErr](Some(Bytes(b)))
}

func (m *memFileH) readLine() Result[Option[string], FsErr] {
	if m.off >= len(m.data) {
		return Ok[Option[string], FsErr](Option[string]{})
	}
	rest := m.data[m.off:]
	line := rest
	if i := strings.IndexByte(rest, '\n'); i >= 0 {
		line = rest[:i+1]
	}
	m.off += len(line)
	return Ok[Option[string], FsErr](Some(trimLine(line)))
}

func (m *memFileH) write(data Bytes) {
	fs := m.v.fs
	fs.mu.Lock()
	defer fs.mu.Unlock()
	n := fs.get(m.full)
	if n == nil || n.dir {
		// removed since it was opened: written again, as an unlinked
		// file would not be; MemFs keeps the data visible
		n = &memNode{}
		fs.put(m.full, n)
	}
	n.data += data
	n.mod = fs.now()
}

func (m *memFileH) close() {}

// follow resolves symbolic links at full (relative to the fs root);
// false when a chain is longer than 40 links. Caller holds fs.mu.
func (fs *memFs) follow(full string) (string, bool) {
	for i := 0; i < 40; i++ {
		n := fs.get(full)
		if n == nil || n.link == "" {
			return full, true
		}
		t := n.link
		if strings.HasPrefix(t, "/") {
			full = strings.Trim(path.Clean(t), "/")
		} else {
			full = strings.Trim(path.Join(path.Dir("/"+full), t), "/")
		}
	}
	return full, false
}

func (f Testing_MemFs) openFile(tk *Task, p string, write, truncate bool) Result[File, FsErr] {
	full, e := f.v.begin(tk, p, write)
	if e != nil {
		return Err[File](*e)
	}
	defer f.v.fs.mu.Unlock()
	fs := f.v.fs
	op := opRead
	if write {
		op = opWrite
	}
	if e := fs.failed(op, full); e != nil {
		return Err[File](*e)
	}
	full, ok := fs.follow(full)
	if !ok {
		return Err[File](fsIo(p, "too many levels of symbolic links"))
	}
	n := fs.get(full)
	if !write {
		switch {
		case n == nil:
			return Err[File](fsErr(1, p))
		case n.dir:
			return Err[File](fsErr(5, p))
		}
		return Ok[File, FsErr](File{&fileH{path: p, mem: &memFileH{v: f.v, full: full, data: string(n.data)}}})
	}
	if full == "" || n != nil && n.dir {
		return Err[File](fsErr(5, p))
	}
	if bad, ok := fs.mkdirs(full, false); !ok {
		return Err[File](fsErr(4, f.v.rel(bad)))
	}
	if n == nil {
		n = &memNode{mod: fs.now()}
		fs.put(full, n)
	} else if truncate {
		n.data, n.mod = "", fs.now()
	}
	return Ok[File, FsErr](File{&fileH{path: p, write: true, mem: &memFileH{v: f.v, full: full}}})
}

func (f Testing_MemFs) M_open(tk *Task, p string) Result[File, FsErr] {
	if f.v.real != nil {
		return f.v.real.M_open(tk, p)
	}
	return f.openFile(tk, p, false, false)
}

func (f Testing_MemFs) M_create(tk *Task, p string) Result[File, FsErr] {
	if f.v.real != nil {
		return f.v.real.M_create(tk, p)
	}
	return f.openFile(tk, p, true, true)
}

func (f Testing_MemFs) M_open_append(tk *Task, p string) Result[File, FsErr] {
	if f.v.real != nil {
		return f.v.real.M_open_append(tk, p)
	}
	return f.openFile(tk, p, true, false)
}

func (f Testing_MemFs) M_append(tk *Task, p string, data Bytes) Result[Unit, FsErr] {
	if f.v.real != nil {
		return f.v.real.M_append(tk, p, data)
	}
	r := f.M_open_append(tk, p)
	if !r.Ok {
		return Err[Unit](r.E)
	}
	r.V.M_write(tk, data)
	return r.V.M_close(tk)
}

func (f Testing_MemFs) M_copy(tk *Task, from, to string) Result[Unit, FsErr] {
	if f.v.real != nil {
		return f.v.real.M_copy(tk, from, to)
	}
	src, e := f.v.begin(tk, from, false)
	if e != nil {
		return Err[Unit](*e)
	}
	fs := f.v.fs
	if e := fs.failed(opRead, src); e != nil {
		fs.mu.Unlock()
		return Err[Unit](*e)
	}
	src, ok := fs.follow(src)
	n := fs.get(src)
	var data Bytes
	var mode int64
	var hasMode bool
	switch {
	case !ok:
		fs.mu.Unlock()
		return Err[Unit](fsIo(from, "too many levels of symbolic links"))
	case n == nil:
		fs.mu.Unlock()
		return Err[Unit](fsErr(1, from))
	case n.dir:
		fs.mu.Unlock()
		return Err[Unit](fsErr(5, from))
	}
	data, mode, hasMode = n.data, n.mode, n.hasMode
	fs.mu.Unlock()
	if w := f.M_write(tk, to, data); !w.Ok {
		return w
	}
	dst, _ := f.v.resolve(to)
	fs.mu.Lock()
	if d := fs.get(dst); d != nil {
		d.mode, d.hasMode = mode, hasMode
	}
	fs.mu.Unlock()
	return Ok[Unit, FsErr](Unit{})
}

// memMode is a node's permission bits: as set, else 0o755 for a
// directory and 0o644 for a file.
func memMode(n *memNode) int64 {
	switch {
	case n.hasMode:
		return n.mode
	case n.dir:
		return 0o755
	}
	return 0o644
}

func (f Testing_MemFs) M_mode(tk *Task, p string) Result[int64, FsErr] {
	if f.v.real != nil {
		return f.v.real.M_mode(tk, p)
	}
	full, e := f.v.begin(tk, p, false)
	if e != nil {
		return Err[int64](*e)
	}
	defer f.v.fs.mu.Unlock()
	full, _ = f.v.fs.follow(full)
	n := f.v.fs.get(full)
	if n == nil {
		return Err[int64](fsErr(1, p))
	}
	return Ok[int64, FsErr](memMode(n))
}

func (f Testing_MemFs) M_set_mode(tk *Task, p string, mode int64) Result[Unit, FsErr] {
	if f.v.real != nil {
		return f.v.real.M_set_mode(tk, p, mode)
	}
	full, e := f.v.begin(tk, p, true)
	if e != nil {
		return Err[Unit](*e)
	}
	defer f.v.fs.mu.Unlock()
	if e := f.v.fs.failed(opWrite, full); e != nil {
		return Err[Unit](*e)
	}
	full, _ = f.v.fs.follow(full)
	n := f.v.fs.get(full)
	if n == nil {
		return Err[Unit](fsErr(1, p))
	}
	n.mode, n.hasMode = mode&0o777, true
	return Ok[Unit, FsErr](Unit{})
}

func (f Testing_MemFs) M_symlink(tk *Task, target, link string) Result[Unit, FsErr] {
	if f.v.real != nil {
		return f.v.real.M_symlink(tk, target, link)
	}
	full, e := f.v.begin(tk, link, true)
	if e != nil {
		return Err[Unit](*e)
	}
	defer f.v.fs.mu.Unlock()
	fs := f.v.fs
	if e := fs.failed(opWrite, full); e != nil {
		return Err[Unit](*e)
	}
	if f.v.sub && linkEscapes(link, target) {
		return Err[Unit](fsErr(6, link))
	}
	if full == "" || fs.get(full) != nil {
		return Err[Unit](fsErr(2, link))
	}
	if bad, ok := fs.mkdirs(full, false); !ok {
		return Err[Unit](fsErr(4, f.v.rel(bad)))
	}
	t := strings.ReplaceAll(target, "\\", "/")
	if strings.HasPrefix(t, "/") && f.v.root != "" {
		// an absolute target inside the view names a path of the view
		t = "/" + strings.Trim(path.Join(f.v.root, t), "/")
	}
	fs.put(full, &memNode{link: t, mod: fs.now()})
	return Ok[Unit, FsErr](Unit{})
}

func (f Testing_MemFs) M_read_link(tk *Task, p string) Result[Option[string], FsErr] {
	if f.v.real != nil {
		return f.v.real.M_read_link(tk, p)
	}
	full, e := f.v.begin(tk, p, false)
	if e != nil {
		return Err[Option[string]](*e)
	}
	defer f.v.fs.mu.Unlock()
	n := f.v.fs.get(full)
	switch {
	case n == nil:
		return Err[Option[string]](fsErr(1, p))
	case n.link == "":
		return Ok[Option[string], FsErr](Option[string]{})
	}
	t := n.link
	if strings.HasPrefix(t, "/") && f.v.root != "" {
		t = "/" + strings.TrimPrefix(strings.TrimPrefix(strings.Trim(t, "/"), f.v.root), "/")
	}
	return Ok[Option[string], FsErr](Some(t))
}

func (f Testing_MemFs) M_glob(tk *Task, pattern string) Result[List[string], FsErr] {
	if f.v.real != nil {
		return f.v.real.M_glob(tk, pattern)
	}
	return fsGlob(tk, f, pattern)
}

func (f Testing_MemFs) lockFile(tk *Task, p string, wait bool) Result[Option[FileLock], FsErr] {
	full, e := f.v.begin(tk, p, true)
	if e != nil {
		return Err[Option[FileLock]](*e)
	}
	fs := f.v.fs
	if e := fs.failed(opWrite, full); e != nil {
		fs.mu.Unlock()
		return Err[Option[FileLock]](*e)
	}
	if n := fs.get(full); n == nil {
		if bad, ok := fs.mkdirs(full, false); !ok {
			fs.mu.Unlock()
			return Err[Option[FileLock]](fsErr(4, f.v.rel(bad)))
		}
		fs.put(full, &memNode{mod: fs.now()})
	} else if n.dir {
		fs.mu.Unlock()
		return Err[Option[FileLock]](fsErr(5, p))
	}
	if fs.locks == nil {
		fs.locks = map[string]*lock{}
	}
	l := fs.locks[fs.key(full)]
	if l == nil {
		l = &lock{permits: 1}
		fs.locks[fs.key(full)] = l
	}
	fs.mu.Unlock()
	if !wait {
		l.mu.Lock()
		free := l.free(false, false) && len(l.q) == 0
		if free {
			l.permits--
		}
		l.mu.Unlock()
		if !free {
			return Ok[Option[FileLock], FsErr](Option[FileLock]{})
		}
		return Ok[Option[FileLock], FsErr](Some(FileLock{&flockH{mem: l}}))
	}
	if !l.acquire(tk) {
		return Err[Option[FileLock]](FsCanceled)
	}
	return Ok[Option[FileLock], FsErr](Some(FileLock{&flockH{mem: l}}))
}

func (f Testing_MemFs) M_lock(tk *Task, p string) Result[FileLock, FsErr] {
	if f.v.real != nil {
		return f.v.real.M_lock(tk, p)
	}
	r := f.lockFile(tk, p, true)
	if !r.Ok {
		return Err[FileLock](r.E)
	}
	return Ok[FileLock, FsErr](r.V.V)
}

func (f Testing_MemFs) M_try_lock(tk *Task, p string) Result[Option[FileLock], FsErr] {
	if f.v.real != nil {
		return f.v.real.M_try_lock(tk, p)
	}
	return f.lockFile(tk, p, false)
}
