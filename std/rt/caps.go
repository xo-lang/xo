package rt

import (
	"bufio"
	"context"
	crand "crypto/rand"
	"encoding/binary"
	"errors"
	"io"
	"io/fs"
	"net"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
)

// RealOs returns the real capabilities given to `main` (core 6.3).
func RealOs(tasks Task_Group) Os {
	return Os{
		F_net:   realNet{},
		F_fs:    &realFs{},
		F_clock: realClock{},
		F_rand:  realRand{},
		F_env:   realEnv{},
		F_proc:  realProc{},
		F_stdio: realStdio{},
		F_tasks: tasks,
	}
}

// ---------------------------------------------------------------------------
// Fs (std.md 3.1)

// realFs is the real filesystem, rooted at the filesystem root (root "")
// or, for a sub capability, at root.
type realFs struct {
	root      string // "" for os.fs
	sub       bool
	readOnly  bool
	writeOnly bool
}

func fsErr(tag int, p string) FsErr {
	switch tag {
	case 1:
		return FsErr{Tag: 1, P: FsErr_NotFound{F_path: p}}
	case 2:
		return FsErr{Tag: 2, P: FsErr_Exists{F_path: p}}
	case 3:
		return FsErr{Tag: 3, P: FsErr_NotEmpty{F_path: p}}
	case 4:
		return FsErr{Tag: 4, P: FsErr_NotDir{F_path: p}}
	case 5:
		return FsErr{Tag: 5, P: FsErr_IsDir{F_path: p}}
	case 6:
		return FsErr{Tag: 6, P: FsErr_Denied{F_path: p}}
	}
	return FsErr{Tag: 8}
}

func fsIo(p, msg string) FsErr { return FsErr{Tag: 7, P: FsErr_Io{F_path: p, F_msg: msg}} }

// FsCanceled is FsErr.Canceled.
var FsCanceled = FsErr{Tag: 8}

// mapFsErr converts an OS error to FsErr.
func mapFsErr(err error, p string) FsErr {
	// specific errnos first: Go maps ENOTEMPTY to fs.ErrExist
	switch {
	case errors.Is(err, syscall.ENOTEMPTY):
		return fsErr(3, p)
	case errors.Is(err, syscall.ENOTDIR):
		return fsErr(4, p)
	case errors.Is(err, syscall.EISDIR):
		return fsErr(5, p)
	case errors.Is(err, fs.ErrNotExist):
		return fsErr(1, p)
	case errors.Is(err, fs.ErrExist):
		return fsErr(2, p)
	case errors.Is(err, fs.ErrPermission):
		return fsErr(6, p)
	}
	msg := err.Error()
	var pe *fs.PathError
	if errors.As(err, &pe) {
		msg = pe.Err.Error()
	}
	return fsIo(p, msg)
}

// resolve maps an Xo path to an OS path (std.md 3.1): os.fs takes
// absolute paths and resolves relative ones against the working
// directory; a sub capability resolves paths against its root and denies
// paths that escape it.
func (f *realFs) resolve(p string) (string, bool) {
	if !f.sub {
		if p == "" {
			p = "."
		}
		return filepath.FromSlash(p), true
	}
	clean := path.Clean("/" + strings.ReplaceAll(p, "\\", "/"))
	if strings.Contains(p, "..") {
		// a path that names a parent beyond the root escapes it
		depth := 0
		for _, seg := range strings.Split(p, "/") {
			switch seg {
			case "..":
				depth--
			case "", ".":
			default:
				depth++
			}
			if depth < 0 {
				return "", false
			}
		}
	}
	return filepath.Join(f.root, filepath.FromSlash(clean)), true
}

// relPath is the path of an OS path relative to the capability root.
func (f *realFs) relPath(osPath, given string) string {
	if !f.sub {
		return filepath.ToSlash(osPath)
	}
	rel, err := filepath.Rel(f.root, osPath)
	if err != nil {
		return filepath.ToSlash(given)
	}
	rel = filepath.ToSlash(rel)
	if rel == "." {
		return ""
	}
	return rel
}

func (f *realFs) check(tk *Task, p string, write bool) (string, *FsErr) {
	if tk != nil && tk.Canceled() {
		e := FsCanceled
		return "", &e
	}
	if write && f.readOnly || !write && f.writeOnly {
		e := fsErr(6, p)
		return "", &e
	}
	op, ok := f.resolve(p)
	if !ok {
		e := fsErr(6, p)
		return "", &e
	}
	return op, nil
}

func (f *realFs) M_sub(_ *Task, dir string) Fs {
	op, ok := f.resolve(dir)
	if !ok {
		return &realFs{root: filepath.Join(f.root, "denied"), sub: true, readOnly: true, writeOnly: true}
	}
	if abs, err := filepath.Abs(op); err == nil {
		op = abs
	}
	return &realFs{root: op, sub: true, readOnly: f.readOnly, writeOnly: f.writeOnly}
}

func (f *realFs) M_cwd(*Task) Fs {
	wd, _ := os.Getwd()
	return &realFs{root: wd, sub: true, readOnly: f.readOnly, writeOnly: f.writeOnly}
}

// M_cwd_path is the working directory as os.Getwd gives it (decision
// 0218): $PWD only when it names the same directory, else the OS's
// getcwd; / separators on Windows.
func (f *realFs) M_cwd_path(*Task) Result[string, FsErr] {
	wd, err := os.Getwd()
	if err != nil {
		return Err[string](fsIo(".", err.Error()))
	}
	return Ok[string, FsErr](textOf(filepath.ToSlash(wd)))
}

func (f *realFs) M_read_only(*Task) Fs {
	c := *f
	c.readOnly = true
	return &c
}

func (f *realFs) M_write_only(*Task) Fs {
	c := *f
	c.writeOnly = true
	return &c
}

func (f *realFs) M_case_sensitive(*Task) bool {
	return !(runtimeGOOS() == "darwin" || runtimeGOOS() == "windows")
}

func (f *realFs) info(osPath, given string, fi os.FileInfo) FileInfo {
	return FileInfo{F_name: fi.Name(), F_path: f.relPath(osPath, given), F_is_dir: fi.IsDir(), F_size: fi.Size(), F_modified: Time(fi.ModTime().UnixNano()), F_is_symlink: fi.Mode()&os.ModeSymlink != 0}
}

func (f *realFs) M_stat(tk *Task, p string) Result[Option[FileInfo], FsErr] {
	return fx(tk, "fs.stat", false, func() []any { return []any{p} }, func() Result[Option[FileInfo], FsErr] { return f.stat(tk, p) })
}

func (f *realFs) stat(tk *Task, p string) Result[Option[FileInfo], FsErr] {
	op, e := f.check(tk, p, false)
	if e != nil {
		return Err[Option[FileInfo]](*e)
	}
	fi, err := os.Stat(op)
	if errors.Is(err, fs.ErrNotExist) {
		return Ok[Option[FileInfo], FsErr](Option[FileInfo]{})
	}
	if err != nil {
		return Err[Option[FileInfo]](mapFsErr(err, p))
	}
	return Ok[Option[FileInfo], FsErr](Some(f.info(op, p, fi)))
}

func (f *realFs) M_exists(tk *Task, p string) Result[bool, FsErr] {
	r := f.M_stat(tk, p)
	if !r.Ok {
		return Err[bool](r.E)
	}
	return Ok[bool, FsErr](r.V.Some)
}

func (f *realFs) M_read(tk *Task, p string) Result[Bytes, FsErr] {
	return fx(tk, "fs.read", false, func() []any { return []any{p} }, func() Result[Bytes, FsErr] { return f.read(tk, p) })
}

func (f *realFs) read(tk *Task, p string) Result[Bytes, FsErr] {
	op, e := f.check(tk, p, false)
	if e != nil {
		return Err[Bytes](*e)
	}
	data, err := os.ReadFile(op)
	if err != nil {
		return Err[Bytes](mapFsErr(err, p))
	}
	return Ok[Bytes, FsErr](Bytes(data))
}

func (f *realFs) M_read_text(tk *Task, p string) Result[string, FsErr] {
	r := f.M_read(tk, p)
	if !r.Ok {
		return Err[string](r.E)
	}
	return Ok[string, FsErr](textOf(string(r.V)))
}

func (f *realFs) M_read_dir(tk *Task, dir string) Result[List[FileInfo], FsErr] {
	return fx(tk, "fs.read_dir", false, func() []any { return []any{dir} }, func() Result[List[FileInfo], FsErr] { return f.readDir(tk, dir) })
}

func (f *realFs) readDir(tk *Task, dir string) Result[List[FileInfo], FsErr] {
	op, e := f.check(tk, dir, false)
	if e != nil {
		return Err[List[FileInfo]](*e)
	}
	ents, err := os.ReadDir(op)
	if err != nil {
		return Err[List[FileInfo]](mapFsErr(err, dir))
	}
	var out []FileInfo
	for _, de := range ents {
		fi, err := de.Info()
		// os.File.Readdir: an entry removed since the listing is skipped,
		// any other error is the result
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return Err[List[FileInfo]](mapFsErr(err, path.Join(dir, textOf(de.Name()))))
		}
		// the name the program sees: outside bytes (decision 0259)
		out = append(out, f.info(filepath.Join(op, de.Name()), path.Join(dir, textOf(de.Name())), fi))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].F_name < out[j].F_name })
	return Ok[List[FileInfo], FsErr](ListOf(out...))
}

func (f *realFs) M_walk(tk *Task, dir string) Result[List[FileInfo], FsErr] {
	return fx(tk, "fs.walk", false, func() []any { return []any{dir} }, func() Result[List[FileInfo], FsErr] { return f.walk(tk, dir) })
}

func (f *realFs) walk(tk *Task, dir string) Result[List[FileInfo], FsErr] {
	op, e := f.check(tk, dir, false)
	if e != nil {
		return Err[List[FileInfo]](*e)
	}
	if _, err := os.Stat(op); err != nil {
		return Err[List[FileInfo]](mapFsErr(err, dir))
	}
	var out []FileInfo
	var walk func(osDir, xoDir string) *FsErr
	walk = func(osDir, xoDir string) *FsErr {
		ents, err := os.ReadDir(osDir)
		if err != nil {
			e := mapFsErr(err, xoDir)
			return &e
		}
		sort.Slice(ents, func(i, j int) bool { return ents[i].Name() < ents[j].Name() })
		for _, de := range ents {
			fi, err := de.Info()
			child := filepath.Join(osDir, de.Name())
			xc := path.Join(xoDir, de.Name())
			if errors.Is(err, fs.ErrNotExist) {
				continue
			}
			if err != nil {
				e := mapFsErr(err, xc)
				return &e
			}
			out = append(out, f.info(child, xc, fi))
			if fi.IsDir() {
				if e := walk(child, xc); e != nil {
					return e
				}
			}
		}
		return nil
	}
	if e := walk(op, dir); e != nil {
		return Err[List[FileInfo]](*e)
	}
	return Ok[List[FileInfo], FsErr](ListOf(out...))
}

func (f *realFs) M_write(tk *Task, p string, data Bytes) Result[Unit, FsErr] {
	return fx(tk, "fs.write", false, func() []any { return []any{p, int64(len(data))} }, func() Result[Unit, FsErr] { return f.write(tk, p, data) })
}

func (f *realFs) write(tk *Task, p string, data Bytes) Result[Unit, FsErr] {
	op, e := f.check(tk, p, true)
	if e != nil {
		return Err[Unit](*e)
	}
	if err := os.WriteFile(op, []byte(data), 0o644); err != nil {
		return Err[Unit](mapFsErr(err, p))
	}
	return Ok[Unit, FsErr](Unit{})
}

func (f *realFs) M_create_temp(tk *Task, dir, prefix string) Result[string, FsErr] {
	return fx(tk, "fs.create_temp", false, func() []any { return []any{dir, prefix} }, func() Result[string, FsErr] { return f.createTemp(tk, dir, prefix) })
}

func (f *realFs) createTemp(tk *Task, dir, prefix string) Result[string, FsErr] {
	op, e := f.check(tk, dir, true)
	if e != nil {
		return Err[string](*e)
	}
	tf, err := os.CreateTemp(op, prefix+"*")
	if err != nil {
		return Err[string](mapFsErr(err, dir))
	}
	name := tf.Name()
	tf.Close()
	return Ok[string, FsErr](f.relPath(name, path.Join(dir, filepath.Base(name))))
}

func (f *realFs) M_rename(tk *Task, from, to string) Result[Unit, FsErr] {
	return fx(tk, "fs.rename", false, func() []any { return []any{from, to} }, func() Result[Unit, FsErr] { return f.rename(tk, from, to) })
}

func (f *realFs) rename(tk *Task, from, to string) Result[Unit, FsErr] {
	of, e := f.check(tk, from, true)
	if e != nil {
		return Err[Unit](*e)
	}
	ot, e := f.check(tk, to, true)
	if e != nil {
		return Err[Unit](*e)
	}
	if err := os.Rename(of, ot); err != nil {
		return Err[Unit](mapFsErr(err, from))
	}
	return Ok[Unit, FsErr](Unit{})
}

func (f *realFs) M_remove(tk *Task, p string) Result[Unit, FsErr] {
	return fx(tk, "fs.remove", false, func() []any { return []any{p} }, func() Result[Unit, FsErr] { return f.remove(tk, p) })
}

func (f *realFs) remove(tk *Task, p string) Result[Unit, FsErr] {
	op, e := f.check(tk, p, true)
	if e != nil {
		return Err[Unit](*e)
	}
	if err := os.Remove(op); err != nil {
		return Err[Unit](mapFsErr(err, p))
	}
	return Ok[Unit, FsErr](Unit{})
}

func (f *realFs) M_remove_all(tk *Task, p string) Result[Unit, FsErr] {
	return fx(tk, "fs.remove_all", false, func() []any { return []any{p} }, func() Result[Unit, FsErr] { return f.removeAll(tk, p) })
}

func (f *realFs) removeAll(tk *Task, p string) Result[Unit, FsErr] {
	op, e := f.check(tk, p, true)
	if e != nil {
		return Err[Unit](*e)
	}
	if err := os.RemoveAll(op); err != nil {
		return Err[Unit](mapFsErr(err, p))
	}
	return Ok[Unit, FsErr](Unit{})
}

func (f *realFs) M_create_dir_all(tk *Task, p string) Result[Unit, FsErr] {
	return fx(tk, "fs.create_dir_all", false, func() []any { return []any{p} }, func() Result[Unit, FsErr] { return f.createDirAll(tk, p) })
}

func (f *realFs) createDirAll(tk *Task, p string) Result[Unit, FsErr] {
	op, e := f.check(tk, p, true)
	if e != nil {
		return Err[Unit](*e)
	}
	if err := os.MkdirAll(op, 0o755); err != nil {
		return Err[Unit](mapFsErr(err, p))
	}
	return Ok[Unit, FsErr](Unit{})
}

func (f *realFs) M_set_modified(tk *Task, p string, t Time) Result[Unit, FsErr] {
	return fx(tk, "fs.set_modified", false, func() []any { return []any{p, t} }, func() Result[Unit, FsErr] { return f.setModified(tk, p, t) })
}

func (f *realFs) setModified(tk *Task, p string, t Time) Result[Unit, FsErr] {
	op, e := f.check(tk, p, true)
	if e != nil {
		return Err[Unit](*e)
	}
	tt := time.Unix(0, int64(t))
	if err := os.Chtimes(op, tt, tt); err != nil {
		return Err[Unit](mapFsErr(err, p))
	}
	return Ok[Unit, FsErr](Unit{})
}

// XoDebug implements Debug.
func (f *realFs) XoDebug(b *strings.Builder) { b.WriteString("<fs>") }

// ---------------------------------------------------------------------------
// Net (std.md 3.2)

// Listener is a listening socket (real or in memory). id numbers the
// real listeners of a run (record and replay); a replayed listener has
// no socket.
type Listener struct {
	l      net.Listener
	mem    *memListener
	id     int64
	replay bool
	addr   string // the bound address, recorded for replay
}

// M_addr is `l.addr()` (std.md 3.2): the bound address, with the port the
// system chose for ":0". An unspecified host (":0", "0.0.0.0", "[::]")
// becomes the loopback address, so the result can be dialed locally.
func (l Listener) M_addr(*Task) string {
	if l.mem != nil {
		return l.mem.addr
	}
	return l.addr
}

// boundAddr is the dialable form of a listener's address.
func boundAddr(a net.Addr) string {
	host, port, err := net.SplitHostPort(a.String())
	if err != nil {
		return a.String()
	}
	if ip := net.ParseIP(host); ip != nil && ip.IsUnspecified() {
		if ip.To4() != nil {
			host = "127.0.0.1"
		} else {
			host = "::1"
		}
	}
	return net.JoinHostPort(host, port)
}

// Conn is a connection.
type Conn struct{ c net.Conn }

// ServerConn is an accepted connection (decision 0280): the same value
// as Conn, a distinct type in Xo only for the effects of its reads and
// writes (net.listen).
type ServerConn = Conn

// netClosedText is the message of an operation on a closed listener or
// connection (Go's net.ErrClosed, as the native runtime words it).
const netClosedText = "use of closed network connection"

// interruptible runs op, a blocking socket operation, so that canceling
// tk ends it: the socket's deadline is set to the past, and op's error
// then reports Canceled (true).
func interruptible(tk *Task, setDeadline func(time.Time) error, op func() error) (canceled bool, err error) {
	return interruptibleTo(tk, setDeadline, time.Time{}, op)
}

// interruptibleTo is interruptible on a connection whose user deadline
// (decision 0430) is restore: a cancellation sets the past, then restore.
func interruptibleTo(tk *Task, setDeadline func(time.Time) error, restore time.Time, op func() error) (canceled bool, err error) {
	done := make(chan struct{})
	stopped := make(chan bool, 1)
	go func() {
		select {
		case <-tk.Done():
			setDeadline(time.Unix(1, 0))
			stopped <- true
		case <-done:
			stopped <- false
		}
	}()
	err = op()
	close(done)
	if <-stopped {
		setDeadline(restore)
		return err != nil, err
	}
	return false, err
}

// netIoErr is the NetErr of a failed socket operation.
func netIoErr(err error) NetErr {
	if errors.Is(err, net.ErrClosed) {
		return netErr(4, netClosedText)
	}
	return netErr(4, err.Error())
}

// M_accept is `l.accept()` (std.md 3.2, decision 0071): waits for the
// next connection; canceling the task gives NetErr.Canceled.
func (l Listener) M_accept(tk *Task) Result[Conn, NetErr] {
	if tk.Canceled() {
		return Err[Conn](netErr(5, ""))
	}
	return fx(tk, "net.accept", true, nil, func() Result[Conn, NetErr] {
		if l.mem != nil {
			// FakeNet (decision 0292): the next dialed connection
			return l.mem.q.accept(tk)
		}
		if l.l == nil {
			return Err[Conn](netErr(4, "accept on a listener without a socket"))
		}
		var c net.Conn
		dl := func(t time.Time) error {
			if d, ok := l.l.(interface{ SetDeadline(time.Time) error }); ok {
				return d.SetDeadline(t)
			}
			return nil
		}
		canceled, err := interruptible(tk, dl, func() error {
			var e error
			c, e = l.l.Accept()
			return e
		})
		if canceled {
			return Err[Conn](netErr(5, ""))
		}
		if err != nil {
			return Err[Conn](netIoErr(err))
		}
		return Ok[Conn, NetErr](Conn{c: c})
	})
}

// M_close is `l.close()`: stops listening; a waiting accept fails.
func (l Listener) M_close(*Task) {
	if l.l != nil {
		l.l.Close()
	}
	if l.mem != nil {
		l.mem.close()
	}
}

// M_read is `c.read(max)` (decision 0071): at most max bytes (at most 1
// MiB), None at the end of the stream.
func (c Conn) M_read(tk *Task, max int64) Result[Option[Bytes], NetErr] {
	if tk.Canceled() {
		return Err[Option[Bytes]](netErr(5, ""))
	}
	return fx(tk, "net.read", true, func() []any { return []any{max} }, func() Result[Option[Bytes], NetErr] {
		if max <= 0 {
			return Ok[Option[Bytes], NetErr](Some(Bytes("")))
		}
		if c.c == nil {
			return Err[Option[Bytes]](netErr(4, netClosedText))
		}
		if connUpgraded(c.c) {
			return Err[Option[Bytes]](netErr(4, upgradedText))
		}
		if m, ok := c.c.(*memEnd); ok {
			// FakeNet: a deadline already passed fails at once; a waiting
			// read is not woken by one (decision 0430)
			if dl := userDeadline(c.c, false); !dl.IsZero() && !time.Now().Before(dl) {
				return Err[Option[Bytes]](netErr(3, ""))
			}
			return m.read(tk, max)
		}
		buf := make([]byte, min(max, 1<<20))
		var n int
		canceled, err := interruptibleTo(tk, c.c.SetReadDeadline, userDeadline(c.c, false), func() error {
			var e error
			n, e = c.c.Read(buf)
			return e
		})
		if canceled {
			return Err[Option[Bytes]](netErr(5, ""))
		}
		if n > 0 {
			return Ok[Option[Bytes], NetErr](Some(Bytes(buf[:n])))
		}
		if err == nil || errors.Is(err, io.EOF) {
			return Ok[Option[Bytes], NetErr](Option[Bytes]{})
		}
		if errors.Is(err, os.ErrDeadlineExceeded) {
			return Err[Option[Bytes]](netErr(3, ""))
		}
		return Err[Option[Bytes]](netIoErr(err))
	})
}

// M_write is `c.write(data)`: writes every byte.
func (c Conn) M_write(tk *Task, data Bytes) Result[Unit, NetErr] {
	if tk.Canceled() {
		return Err[Unit](netErr(5, ""))
	}
	return fx(tk, "net.write", true, func() []any { return []any{data} }, func() Result[Unit, NetErr] {
		if c.c == nil {
			return Err[Unit](netErr(4, netClosedText))
		}
		if connUpgraded(c.c) {
			return Err[Unit](netErr(4, upgradedText))
		}
		if m, ok := c.c.(*memEnd); ok {
			if dl := userDeadline(c.c, true); !dl.IsZero() && !time.Now().Before(dl) {
				return Err[Unit](netErr(3, ""))
			}
			return m.write(tk, data)
		}
		canceled, err := interruptibleTo(tk, c.c.SetWriteDeadline, userDeadline(c.c, true), func() error {
			_, e := c.c.Write([]byte(data))
			return e
		})
		if canceled {
			return Err[Unit](netErr(5, ""))
		}
		if errors.Is(err, os.ErrDeadlineExceeded) {
			return Err[Unit](netErr(3, ""))
		}
		if err != nil {
			return Err[Unit](netIoErr(err))
		}
		return Ok[Unit, NetErr](Unit{})
	})
}

// M_close is `c.close()`.
func (c Conn) M_close(*Task) {
	if c.c != nil {
		connDeadlines.Delete(c.c)
		c.c.Close()
	}
}

// connDeadlines holds each connection's user deadlines (decision 0430),
// so a cancellation (interruptibleTo) restores them, not none.
var connDeadlines sync.Map // net.Conn -> *connDL

type connDL struct{ rd, wr atomic.Int64 } // UnixNano, 0 for none

// userDeadline is c's user read (or write) deadline, the zero Time for none.
func userDeadline(c net.Conn, write bool) time.Time {
	v, ok := connDeadlines.Load(c)
	if !ok {
		return time.Time{}
	}
	d := v.(*connDL)
	n := d.rd.Load()
	if write {
		n = d.wr.Load()
	}
	if n == 0 {
		return time.Time{}
	}
	return time.Unix(0, n)
}

// setDeadline is the deadline setters' body: t on clock, made a time on
// the real clock from now (a past t fails the next operation at once).
func (c Conn) setDeadline(tk *Task, clock Clock, t Option[Time], read, write bool) {
	if c.c == nil {
		return
	}
	var at time.Time
	if t.Some {
		at = time.Now().Add(time.Duration(int64(t.V) - int64(clock.M_now(tk))))
	}
	v, _ := connDeadlines.LoadOrStore(c.c, &connDL{})
	d := v.(*connDL)
	n := int64(0)
	if !at.IsZero() {
		n = at.UnixNano()
	}
	if read {
		d.rd.Store(n)
		c.c.SetReadDeadline(at)
	}
	if write {
		d.wr.Store(n)
		c.c.SetWriteDeadline(at)
	}
}

// M_set_deadline is `c.set_deadline(clock, t)` (decision 0430): Go's
// SetDeadline.
func (c Conn) M_set_deadline(tk *Task, clock Clock, t Option[Time]) {
	c.setDeadline(tk, clock, t, true, true)
}

// M_set_read_deadline is Go's SetReadDeadline (decision 0430).
func (c Conn) M_set_read_deadline(tk *Task, clock Clock, t Option[Time]) {
	c.setDeadline(tk, clock, t, true, false)
}

// M_set_write_deadline is Go's SetWriteDeadline (decision 0430).
func (c Conn) M_set_write_deadline(tk *Task, clock Clock, t Option[Time]) {
	c.setDeadline(tk, clock, t, false, true)
}

// XoDebug implements Debug.
func (l Listener) XoDebug(b *strings.Builder) { b.WriteString("<listener>") }

// XoDebug implements Debug.
func (c Conn) XoDebug(b *strings.Builder) { b.WriteString("<conn>") }

type realNet struct{}

func netErr(tag int, msg string) NetErr {
	switch tag {
	case 1:
		return NetErr{Tag: 1, P: NetErr_InvalidAddr{F_msg: msg}}
	case 4:
		return NetErr{Tag: 4, P: NetErr_Io{F_msg: msg}}
	}
	return NetErr{Tag: tag}
}

func (n realNet) M_listen(tk *Task, addr string) Result[Listener, NetErr] {
	return fx(tk, "net.listen", false, func() []any { return []any{addr} }, func() Result[Listener, NetErr] { return n.listen(tk, addr) })
}

// noSockets is the error of a listen or dial on wasip1, where Go's net
// package is an in-memory fake: a listener there took no port and its
// accept loop spun at full CPU (a 0.25.2 field report), and the wasm
// backend refuses the same program (compiler/wasm call.xo).
const noSockets = "WASI preview1 has no sockets"

func (realNet) listen(tk *Task, addr string) Result[Listener, NetErr] {
	if tk.Canceled() {
		return Err[Listener](netErr(5, ""))
	}
	if runtime.GOOS == "wasip1" {
		return Err[Listener](netErr(4, "listen "+addr+": "+noSockets))
	}
	if path, ok := unixPath(addr); ok {
		// a Unix domain socket (decision 0190); closing the listener
		// removes the socket file
		l, err := net.Listen("unix", path)
		if err != nil {
			return Err[Listener](netErr(4, err.Error()))
		}
		return Ok[Listener, NetErr](Listener{l: l, id: fxListenerSeq.Add(1), addr: addr})
	}
	l, err := net.Listen("tcp", addr)
	if err != nil {
		return Err[Listener](netErr(4, err.Error()))
	}
	return Ok[Listener, NetErr](Listener{l: l, id: fxListenerSeq.Add(1), addr: boundAddr(l.Addr())})
}

func (n realNet) M_dial(tk *Task, addr string) Result[Conn, NetErr] {
	return fx(tk, "net.dial", false, func() []any { return []any{addr} }, func() Result[Conn, NetErr] { return n.dial(tk, addr) })
}

// M_dial_timeout is `n.dial_timeout(addr, d)` (decision 0406): Timeout
// when d passes.
func (n realNet) M_dial_timeout(tk *Task, addr string, d Duration) Result[Conn, NetErr] {
	return fx(tk, "net.dial", false, func() []any { return []any{addr} }, func() Result[Conn, NetErr] { return n.dialFor(tk, addr, time.Duration(d)) })
}

// M_lookup_host is `n.lookup_host(host)` (decision 0406): Go's
// net.LookupHost.
func (realNet) M_lookup_host(tk *Task, host string) Result[List[string], NetErr] {
	if tk.Canceled() {
		return Err[List[string]](netErr(5, ""))
	}
	addrs, err := net.LookupHost(host)
	if err != nil {
		var de *net.DNSError
		if errors.As(err, &de) && de.IsNotFound {
			return Err[List[string]](netErr(4, "lookup "+host+": no such host"))
		}
		return Err[List[string]](netErr(4, err.Error()))
	}
	return Ok[List[string], NetErr](ListFromSlice(addrs))
}

// dialFor dials with the limit d, Timeout when it passes.
func (realNet) dialFor(tk *Task, addr string, d time.Duration) Result[Conn, NetErr] {
	if tk.Canceled() {
		return Err[Conn](netErr(5, ""))
	}
	if runtime.GOOS == "wasip1" {
		return Err[Conn](netErr(4, "dial "+addr+": "+noSockets))
	}
	network := "tcp"
	if path, ok := unixPath(addr); ok {
		network, addr = "unix", path
	}
	if d <= 0 {
		d = 1
	}
	c, err := net.DialTimeout(network, addr, d)
	if err != nil {
		var ne net.Error
		if errors.As(err, &ne) && ne.Timeout() {
			return Err[Conn](netErr(3, ""))
		}
		if errors.Is(err, syscall.ECONNREFUSED) {
			return Err[Conn](netErr(2, ""))
		}
		return Err[Conn](netErr(4, err.Error()))
	}
	return Ok[Conn, NetErr](Conn{c: c})
}

func (realNet) dial(tk *Task, addr string) Result[Conn, NetErr] {
	if tk.Canceled() {
		return Err[Conn](netErr(5, ""))
	}
	if runtime.GOOS == "wasip1" {
		return Err[Conn](netErr(4, "dial "+addr+": "+noSockets))
	}
	network := "tcp"
	if path, ok := unixPath(addr); ok {
		network, addr = "unix", path
	}
	c, err := net.DialTimeout(network, addr, 30*time.Second)
	if err != nil {
		if errors.Is(err, syscall.ECONNREFUSED) {
			return Err[Conn](netErr(2, ""))
		}
		return Err[Conn](netErr(4, err.Error()))
	}
	return Ok[Conn, NetErr](Conn{c: c})
}

func (realNet) XoDebug(b *strings.Builder) { b.WriteString("<net>") }

// ---------------------------------------------------------------------------
// Clock (std.md 3.3)

type realClock struct{}

func (realClock) M_now(tk *Task) Time {
	tk.Yield()
	return fx(tk, "clock.now", false, nil, func() Time { return Time(time.Now().UnixNano()) })
}

// M_sleep waits through a waiter, like a channel operation, so a sleeping
// task shows in `xo dump` and gives the turn away under `xo run --seed`.
func (realClock) M_sleep(tk *Task, d Duration) {
	tk.Yield()
	tk.CheckCancel()
	if fxMode != fxOff {
		fx(tk, "clock.sleep", false, func() []any { return []any{d} }, func() Unit { return Unit{} })
	}
	if d <= 0 {
		return
	}
	if tk.w != nil {
		// a test with a real clock: the sleep counts as running for the
		// test's virtual clock
		t := time.NewTimer(time.Duration(d))
		defer t.Stop()
		select {
		case <-t.C:
		case <-tk.Done():
			tk.Unwind()
		}
		return
	}
	w := newWaiter(tk)
	stop := realAfter(d, func() {
		if w.claim() {
			w.signal()
		}
	})
	ok := tk.wait(w, waitSleep, nil)
	stop()
	if !ok {
		tk.Unwind()
	}
}

// M_sleep_or_cancel is `clock.sleep_or_cancel(d)`: sleep, with
// cancellation returned as ClockErr.Canceled instead of unwinding
// (decision 0088).
func (c realClock) M_sleep_or_cancel(tk *Task, d Duration) Result[Unit, ClockErr] {
	return sleepOrCancel(tk, func() { c.M_sleep(tk, d) })
}

// sleepOrCancel runs a sleep that unwinds on cancellation and turns the
// unwind into Err(ClockErr.Canceled). Cancellation stays sticky: the
// next cancellation point of the task reports it again.
func sleepOrCancel(tk *Task, sleep func()) (r Result[Unit, ClockErr]) {
	defer func() {
		if x := recover(); x != nil {
			if !IsUnwind(x) {
				panic(x)
			}
			r = Err[Unit](ClockErr{Tag: 1})
		}
	}()
	sleep()
	return Ok[Unit, ClockErr](Unit{})
}

func (c realClock) M_since(tk *Task, t Time) Duration { return Duration(c.M_now(tk) - t) }

func (realClock) XoDebug(b *strings.Builder) { b.WriteString("<clock>") }

// ---------------------------------------------------------------------------
// Rand (std.md 3.4)

type realRand struct{}

func randUint64() uint64 {
	var b [8]byte
	if _, err := crand.Read(b[:]); err != nil {
		panic(err)
	}
	return binary.LittleEndian.Uint64(b[:])
}

func (realRand) M_int(tk *Task, lo, hi int64) int64 {
	if hi <= lo {
		FaultHere("rand.int(%d, %d): the range is empty", lo, hi)
	}
	return fx(tk, "rand.int", false, func() []any { return []any{lo, hi} }, func() int64 { return randRange(randUint64, lo, hi) })
}

func (realRand) M_float(tk *Task) float64 {
	return fx(tk, "rand.float", false, nil, func() float64 { return float64(randUint64()>>11) / (1 << 53) })
}

func (realRand) M_bool(tk *Task) bool {
	return fx(tk, "rand.bool", false, nil, func() bool { return randUint64()&1 == 1 })
}

func (realRand) M_bytes(tk *Task, n int64) Bytes {
	return fx(tk, "rand.bytes", false, func() []any { return []any{n} }, func() Bytes {
		b := make([]byte, max(n, 0))
		if _, err := crand.Read(b); err != nil {
			panic(err)
		}
		return Bytes(b)
	})
}

func (realRand) XoDebug(b *strings.Builder) { b.WriteString("<rand>") }

// randRange returns a uniform value in [lo, hi).
func randRange(next func() uint64, lo, hi int64) int64 {
	if hi <= lo {
		FaultHere("rand.int(%d, %d): the range is empty", lo, hi)
	}
	span := uint64(hi - lo)
	limit := ^uint64(0) - (^uint64(0) % span)
	for {
		v := next()
		if v < limit {
			return lo + int64(v%span)
		}
	}
}

// Rand_shuffle is `r.shuffle(xs)`.
func Rand_shuffle[T any](tk *Task, r Rand, xs List[T]) List[T] {
	out := append([]T(nil), xs.s...)
	for i := len(out) - 1; i > 0; i-- {
		j := r.M_int(tk, 0, int64(i+1))
		out[i], out[j] = out[j], out[i]
	}
	return ListOf(out...)
}

// ---------------------------------------------------------------------------
// Env (std.md 3.5)

type realEnv struct{}

func (realEnv) M_get(tk *Task, name string) Option[string] {
	return fx(tk, "env.get", false, func() []any { return []any{name} }, func() Option[string] {
		v, ok := os.LookupEnv(name)
		return Option[string]{Some: ok, V: textOf(v)}
	})
}

func (e realEnv) M_vars(tk *Task) Map[string, string] {
	return fx(tk, "env.vars", false, nil, e.vars)
}

func (realEnv) vars() Map[string, string] {
	var m Map[string, string]
	env := os.Environ()
	sort.Strings(env)
	for _, kv := range env {
		k, v, _ := strings.Cut(kv, "=")
		Map_put(nil, &m, textOf(k), textOf(v))
	}
	return m
}

func (realEnv) M_args(tk *Task) List[string] {
	return fx(tk, "env.args", false, nil, func() List[string] {
		if len(os.Args) <= 1 {
			return List[string]{}
		}
		return ListOf(textsOf(os.Args[1:])...)
	})
}

func (realEnv) M_program(tk *Task) string {
	// with / separators on Windows, as every path Xo is given (std.md 3.1)
	return fx(tk, "env.program", false, nil, func() string { return textOf(filepath.ToSlash(os.Args[0])) })
}

func (realEnv) M_home(tk *Task) Result[string, EnvErr] {
	return fx(tk, "env.home", false, nil, func() Result[string, EnvErr] {
		h, err := os.UserHomeDir()
		if err != nil {
			return Err[string](EnvErr{Tag: 1, P: EnvErr_Missing{F_name: "HOME"}})
		}
		return Ok[string, EnvErr](textOf(filepath.ToSlash(h)))
	})
}

func (realEnv) XoDebug(b *strings.Builder) { b.WriteString("<env>") }

// ---------------------------------------------------------------------------
// Stdio (std.md 3.6)

type realStdio struct{}

var (
	stdinOnce sync.Once
	stdinR    *bufio.Reader
	stdoutMu  sync.Mutex
)

func stdin() *bufio.Reader {
	stdinOnce.Do(func() { stdinR = bufio.NewReader(os.Stdin) })
	return stdinR
}

func outWrite(w *os.File, s string) {
	stdoutMu.Lock()
	w.WriteString(s)
	stdoutMu.Unlock()
}

func (realStdio) M_print(tk *Task, s string)    { tk.Yield(); outWrite(os.Stdout, s) }
func (realStdio) M_println(tk *Task, s string)  { tk.Yield(); outWrite(os.Stdout, s+"\n") }
func (realStdio) M_eprint(tk *Task, s string)   { tk.Yield(); outWrite(os.Stderr, s) }
func (realStdio) M_eprintln(tk *Task, s string) { tk.Yield(); outWrite(os.Stderr, s+"\n") }

func (realStdio) M_write(tk *Task, data Bytes) Result[Unit, IoErr] {
	if tk.Canceled() {
		return Err[Unit](IoErr{Tag: 3})
	}
	stdoutMu.Lock()
	_, err := os.Stdout.WriteString(string(data))
	stdoutMu.Unlock()
	if err != nil {
		return Err[Unit](IoErr{Tag: 2, P: IoErr_Io{F_msg: err.Error()}})
	}
	return Ok[Unit, IoErr](Unit{})
}

func (s realStdio) M_read_line(tk *Task) Result[Option[string], IoErr] {
	if stdinStopped(tk) {
		return Err[Option[string]](IoErr{Tag: 3})
	}
	return fx(tk, "stdio.read_line", true, nil, func() Result[Option[string], IoErr] { return s.readLine(tk) })
}

// readLine is a cancellation point, and a stop request ends it too
// (core 7.5): both return IoErr.Canceled (stdin.go).
func (realStdio) readLine(tk *Task) Result[Option[string], IoErr] {
	line, ok, r := stdinRead(tk, false)
	if !r.Ok {
		return Err[Option[string]](r.E)
	}
	if !ok {
		return Ok[Option[string], IoErr](Option[string]{})
	}
	line = strings.TrimSuffix(strings.TrimSuffix(line, "\n"), "\r")
	return Ok[Option[string], IoErr](Some(textOf(line)))
}

func (s realStdio) M_read_all(tk *Task) Result[string, IoErr] {
	if stdinStopped(tk) {
		return Err[string](IoErr{Tag: 3})
	}
	return fx(tk, "stdio.read_all", true, nil, func() Result[string, IoErr] { return s.readAll(tk) })
}

func (realStdio) readAll(tk *Task) Result[string, IoErr] {
	data, _, r := stdinRead(tk, true)
	if !r.Ok {
		return Err[string](r.E)
	}
	return Ok[string, IoErr](textOf(data))
}

func (s realStdio) M_lines(tk *Task) Result[List[string], IoErr] {
	r := s.M_read_all(tk)
	if !r.Ok {
		return Err[List[string]](r.E)
	}
	return Ok[List[string], IoErr](Str_lines(tk, r.V))
}

func (realStdio) XoDebug(b *strings.Builder) { b.WriteString("<stdio>") }

// ---------------------------------------------------------------------------
// Proc (std.md 3.7)

type realProc struct{}

func (realProc) M_exit(tk *Task, code int64) Never {
	if fxMode != fxOff {
		fx(tk, "proc.exit", false, func() []any { return []any{code} }, func() Unit { return Unit{} })
	}
	fxFinish(int(code), nil)
	os.Exit(int(code))
	return Never{}
}

// M_set_grace is `os.proc.set_grace(d)`: the time between a stop request
// and the cancellation of main's root scope (core 7.5).
func (realProc) M_set_grace(tk *Task, d Duration) {
	if tk.stop != nil {
		tk.stop.setGrace(d)
	}
}

func (p realProc) M_run(tk *Task, cmd string, args List[string]) Result[ProcOutput, ProcErr] {
	if tk.Canceled() {
		return Err[ProcOutput](ProcErr{Tag: 3})
	}
	return fx(tk, "proc.run", true, func() []any { return []any{cmd, args} }, func() Result[ProcOutput, ProcErr] { return p.run(tk, cmd, args) })
}

func (realProc) run(tk *Task, cmd string, args List[string]) Result[ProcOutput, ProcErr] {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		select {
		case <-tk.Done():
			cancel()
		case <-ctx.Done():
		}
	}()
	c := exec.CommandContext(ctx, cmd, args.s...)
	var out, errb strings.Builder
	c.Stdout, c.Stderr = &out, &errb
	tk.leave()
	err := c.Run()
	tk.enter()
	code := int64(0)
	if err != nil {
		var ee *exec.ExitError
		switch {
		case errors.As(err, &ee):
			code = int64(ee.ExitCode())
		case errors.Is(err, exec.ErrNotFound):
			return Err[ProcOutput](ProcErr{Tag: 1, P: ProcErr_NotFound{F_cmd: cmd}})
		case tk.Canceled():
			return Err[ProcOutput](ProcErr{Tag: 3})
		default:
			return Err[ProcOutput](ProcErr{Tag: 2, P: ProcErr_Io{F_msg: err.Error()}})
		}
	}
	return Ok[ProcOutput, ProcErr](ProcOutput{F_code: code, F_stdout: Bytes(out.String()), F_stderr: Bytes(errb.String())})
}

func (realProc) XoDebug(b *strings.Builder) { b.WriteString("<proc>") }

// Prelude_read_text_or is `read_text_or(fs, path, default)` (std.md 3.1): the
// file's text, or default when it does not exist (FsErr.NotFound); any
// other error is the result.
func Prelude_read_text_or(tk *Task, fs Fs, path string, def string) Result[string, FsErr] {
	r := fs.M_read_text(tk, path)
	if !r.Ok && r.E.Tag == 1 {
		return Ok[string, FsErr](def)
	}
	return r
}

// Prelude_create_dir_all_mode is `create_dir_all_mode(fs, dir, mode)`:
// create_dir_all, then set_mode on each directory it made, deepest first.
func Prelude_create_dir_all_mode(tk *Task, fs Fs, dir string, mode int64) Result[Unit, FsErr] {
	var made []string
	for p := path.Clean(filepath.ToSlash(dir)); p != "."; {
		r := fs.M_exists(tk, p)
		if !r.Ok {
			return Err[Unit](r.E)
		}
		if r.V {
			break
		}
		made = append(made, p)
		up := path.Dir(p)
		if up == p {
			break
		}
		p = up
	}
	if r := fs.M_create_dir_all(tk, dir); !r.Ok {
		return r
	}
	for _, d := range made {
		if r := fs.M_set_mode(tk, d, mode); !r.Ok {
			return r
		}
	}
	return Ok[Unit, FsErr](Unit{})
}

// Prelude_write_atomic is `write_atomic(fs, path, data)` (std.md 3.1): a
// temporary file beside path, written, synced, given path's permission
// bits (0o644 for a new file) and renamed over path, then the directory
// synced on the real file system (not on Windows, which cannot open a
// directory for it); through Fs's methods, as the native prelude's.
func Prelude_write_atomic(tk *Task, fs Fs, p string, data Bytes) Result[Unit, FsErr] {
	dir := path.Dir(p)
	t := fs.M_create_temp(tk, dir, "."+path.Base(p)+".")
	if !t.Ok {
		return Err[Unit](t.E)
	}
	tmp := t.V
	if r := atomicFill(tk, fs, tmp, p, data); !r.Ok {
		fs.M_remove(tk, tmp)
		return r
	}
	if rf, ok := fs.(*realFs); ok && runtime.GOOS != "windows" {
		op, e := rf.check(tk, dir, true)
		if e != nil {
			return Err[Unit](*e)
		}
		d, err := os.Open(op)
		if err != nil {
			return Err[Unit](mapFsErr(err, dir))
		}
		err = d.Sync()
		d.Close()
		if err != nil {
			return Err[Unit](mapFsErr(err, dir))
		}
	}
	return Ok[Unit, FsErr](Unit{})
}

func atomicFill(tk *Task, fs Fs, tmp, p string, data Bytes) Result[Unit, FsErr] {
	mode := int64(0o644)
	if m := fs.M_mode(tk, p); m.Ok {
		mode = m.V
	} else if m.E.Tag != 1 {
		return Err[Unit](m.E)
	}
	c := fs.M_create(tk, tmp)
	if !c.Ok {
		return Err[Unit](c.E)
	}
	f := c.V
	if r := f.M_write(tk, data); !r.Ok {
		f.M_close(tk)
		return r
	}
	if r := f.M_sync(tk); !r.Ok {
		f.M_close(tk)
		return r
	}
	if r := f.M_close(tk); !r.Ok {
		return r
	}
	if r := fs.M_set_mode(tk, tmp, mode); !r.Ok {
		return r
	}
	return fs.M_rename(tk, tmp, p)
}

// Prelude_walk_dir is `walk_dir(fs, dir, visit)` (std.md 3.1, Go's
// filepath.WalkDir): through Fs.M_read_dir, as the native prelude's; dir
// itself is not visited and the first error ends the walk.
func Prelude_walk_dir(tk *Task, fs Fs, dir string, visit func(*Task, FileInfo) Result[WalkStep, Never]) Result[List[FileInfo], FsErr] {
	var out []FileInfo
	if _, err := walkDirIn(tk, fs, dir, visit, &out); err != nil {
		return Err[List[FileInfo]](*err)
	}
	return Ok[List[FileInfo], FsErr](ListOf(out...))
}

// walkDirIn walks dir, each visited entry added to out: true when visit
// stopped the walk.
func walkDirIn(tk *Task, fs Fs, dir string, visit func(*Task, FileInfo) Result[WalkStep, Never], out *[]FileInfo) (bool, *FsErr) {
	r := fs.M_read_dir(tk, dir)
	if !r.Ok {
		return false, &r.E
	}
	for _, fi := range List_Slice(r.V) {
		*out = append(*out, fi)
		switch visit(tk, fi).V.Tag {
		case 3: // Stop
			return true, nil
		case 2: // SkipDir
			if !fi.F_is_dir {
				return false, nil
			}
		default:
			if fi.F_is_dir {
				if stop, err := walkDirIn(tk, fs, fi.F_path, visit, out); stop || err != nil {
					return stop, err
				}
			}
		}
	}
	return false, nil
}

// Prelude_read_mapped is `read_mapped(fs, path)` (decision 0402): the Go
// backend never maps, so it is fs.read.
func Prelude_read_mapped(tk *Task, fs Fs, path string) Result[Bytes, FsErr] {
	return fs.M_read(tk, path)
}

// Prelude_read_or is `read_or(fs, path, default)`: read_text_or for bytes.
func Prelude_read_or(tk *Task, fs Fs, path string, def Bytes) Result[Bytes, FsErr] {
	r := fs.M_read(tk, path)
	if !r.Ok && r.E.Tag == 1 {
		return Ok[Bytes, FsErr](def)
	}
	return r
}
