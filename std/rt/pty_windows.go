//go:build windows

package rt

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
	"unsafe"
)

// std/pty on Windows (decision 0740 stage 2, task #704): a pseudo console
// (ConPTY) over two pipes, as std/native/llrt/ptyos.xo does. os/exec cannot
// give a child the pseudo console, so ptyConSpawn calls CreateProcessW
// with PROC_THREAD_ATTRIBUTE_PSEUDOCONSOLE itself and hands the pid to an
// exec.Cmd's Process, which Child.wait and kill use as for any child.
// gotchas/conpty-null-handles-and-end.md has the traps.

var errPty = errors.New(ptyUnsupported)

const ptySupported = false
const ptyConOS = true

func ptyOpen() (m, s *os.File, path string, err error) { return nil, nil, "", errPty }
func ptyOpenSlave(string) (*os.File, error)            { return nil, errPty }
func ptyGetSize(*os.File) (Pty_Size, error)            { return Pty_Size{}, errPty }
func ptySetSize(*os.File, Pty_Size) error              { return errPty }
func ptySetRaw(*os.File, bool, []byte) ([]byte, error) { return nil, errPty }
func ptyAttach(*exec.Cmd, *os.File, bool)              {}
func ptySetDeadline(*os.File, time.Time)               {}
func ptyRelease(s *os.File, _ int) {
	if s != nil {
		s.Close()
	}
}
func ptyRead(*Task, *os.File, []byte, time.Time) (int, bool, error) { return 0, false, errPty }

var (
	ptyK32          = syscall.NewLazyDLL("kernel32.dll")
	ptyCreatePC     = ptyK32.NewProc("CreatePseudoConsole")
	ptyResizePC     = ptyK32.NewProc("ResizePseudoConsole")
	ptyClosePC      = ptyK32.NewProc("ClosePseudoConsole")
	ptyInitAttrs    = ptyK32.NewProc("InitializeProcThreadAttributeList")
	ptyUpdateAttr   = ptyK32.NewProc("UpdateProcThreadAttribute")
	ptyDeleteAttrs  = ptyK32.NewProc("DeleteProcThreadAttributeList")
	ptyConAvailable = sync.OnceValue(func() bool {
		return ptyCreatePC.Find() == nil && ptyResizePC.Find() == nil && ptyClosePC.Find() == nil
	})
)

const (
	ptyAttrPseudoConsole = 0x20016
	ptyAttrHandleList    = 0x20002
	ptyExtendedStartup   = 0x80000
	ptyUnicodeEnv        = 0x400
	// how long the output stays quiet after the child's exit before the
	// pseudo console closes (ptyos.xo PW_GRACE_NS)
	ptyGrace = 100 * time.Millisecond
)

type ptyChunk struct {
	b   []byte
	err error
}

// ptyCon is the pseudo console beside the ptyFile's m, the read end of
// its output pipe; in is the write end of its input pipe.
type ptyCon struct {
	mu         sync.Mutex
	hpc        uintptr
	in         *os.File
	rows, cols int64
	pcClosed   bool
	chunks     chan ptyChunk
	done       chan struct{}
	doneOnce   sync.Once
	left       []byte
	end        bool
	last       atomic.Int64 // when output last came, unix ns
}

func ptyCoord(rows, cols int64) uintptr {
	return uintptr(uint32(cols&0xffff) | uint32(rows&0xffff)<<16)
}

func ptyHresult(what string, r uintptr) error {
	if int32(r) >= 0 {
		return nil
	}
	return errors.New(what + ": " + syscall.Errno(r&0xffff).Error())
}

func ptyConOpen() (*ptyCon, *os.File, error) {
	if !ptyConAvailable() {
		return nil, nil, nil
	}
	ir, iw, err := os.Pipe()
	if err != nil {
		return nil, nil, err
	}
	or, ow, err := os.Pipe()
	if err != nil {
		ir.Close()
		iw.Close()
		return nil, nil, err
	}
	var hpc uintptr
	r, _, _ := ptyCreatePC.Call(ptyCoord(24, 80), ir.Fd(), ow.Fd(), 0, uintptr(unsafe.Pointer(&hpc)))
	// the pseudo console holds its own copies of its ends
	ir.Close()
	ow.Close()
	if err := ptyHresult("CreatePseudoConsole", r); err != nil {
		iw.Close()
		or.Close()
		return nil, nil, err
	}
	c := &ptyCon{hpc: hpc, in: iw, rows: 24, cols: 80, chunks: make(chan ptyChunk), done: make(chan struct{})}
	go c.pump(or)
	return c, or, nil
}

// pump reads the output pipe for read: anonymous pipes have no deadlines
// on Windows, so read waits on this goroutine's chunks. After close it
// drains the pipe to its end, so conhost never blocks on a full pipe.
func (c *ptyCon) pump(m *os.File) {
	for {
		buf := make([]byte, 32<<10)
		n, err := m.Read(buf)
		if n > 0 {
			c.last.Store(time.Now().UnixNano())
		}
		select {
		case c.chunks <- ptyChunk{buf[:n], err}:
		case <-c.done:
		}
		if err != nil {
			return
		}
	}
}

func (c *ptyCon) read(tk *Task, buf []byte, dl time.Time) (int, bool, error) {
	c.mu.Lock()
	if len(c.left) > 0 {
		n := copy(buf, c.left)
		c.left = c.left[n:]
		c.mu.Unlock()
		return n, false, nil
	}
	if c.end {
		c.mu.Unlock()
		return 0, false, nil
	}
	c.mu.Unlock()
	var tm <-chan time.Time
	if !dl.IsZero() {
		d := time.Until(dl)
		if d <= 0 {
			return 0, false, os.ErrDeadlineExceeded
		}
		t := time.NewTimer(d)
		defer t.Stop()
		tm = t.C
	}
	tk.leave()
	defer tk.enter()
	for {
		select {
		case ch := <-c.chunks:
			c.mu.Lock()
			if ch.err != nil {
				// the pipe's end (ERROR_BROKEN_PIPE once the pseudo console closed)
				c.end = true
			}
			n := copy(buf, ch.b)
			c.left = append(c.left[:0], ch.b[n:]...)
			end := c.end
			c.mu.Unlock()
			if n == 0 && !end {
				continue
			}
			return n, false, nil
		case <-tm:
			return 0, false, os.ErrDeadlineExceeded
		case <-tk.Done():
			return 0, true, nil
		case <-c.done:
			return 0, false, nil
		}
	}
}

func (c *ptyCon) write(b []byte) error {
	_, err := c.in.Write(b)
	return err
}

func (c *ptyCon) size() Pty_Size {
	c.mu.Lock()
	defer c.mu.Unlock()
	return Pty_Size{F_rows: c.rows, F_cols: c.cols}
}

func (c *ptyCon) resize(s Pty_Size) error {
	if s.F_rows < 1 || s.F_cols < 1 || s.F_rows > 32767 || s.F_cols > 32767 {
		return errors.New("rows and cols are 1 to 32767")
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.pcClosed {
		r, _, _ := ptyResizePC.Call(c.hpc, ptyCoord(s.F_rows, s.F_cols))
		if err := ptyHresult("ResizePseudoConsole", r); err != nil {
			return err
		}
	}
	c.rows, c.cols = s.F_rows, s.F_cols
	return nil
}

// closePC closes the pseudo console once: its last bytes, then the
// output pipe's end.
func (c *ptyCon) closePC() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.pcClosed {
		c.pcClosed = true
		ptyClosePC.Call(c.hpc)
	}
}

// close is Pty.close's part before m closes.
func (c *ptyCon) close() {
	c.doneOnce.Do(func() { close(c.done) })
	c.closePC()
	c.in.Close()
}

// watch closes the pseudo console once the child exited and its output
// has been quiet for ptyGrace (the output pipe never ends otherwise).
func (c *ptyCon) watch(h syscall.Handle) {
	syscall.WaitForSingleObject(h, syscall.INFINITE)
	syscall.CloseHandle(h)
	now := time.Now().UnixNano()
	if c.last.Load() < now {
		c.last.Store(now)
	}
	for {
		q := time.Since(time.Unix(0, c.last.Load()))
		if q >= ptyGrace {
			break
		}
		select {
		case <-time.After(ptyGrace - q):
		case <-c.done:
			return
		}
	}
	c.closePC()
}

// ptyCmdLine is os/exec's command line: each argument escaped as
// CommandLineToArgvW reads it.
func ptyCmdLine(args []string) string {
	var b strings.Builder
	for i, a := range args {
		if i > 0 {
			b.WriteByte(' ')
		}
		b.WriteString(syscall.EscapeArg(a))
	}
	return b.String()
}

// ptyEnvBlock is a UTF-16 environment block; SYSTEMROOT is added when
// missing, as os/exec does.
func ptyEnvBlock(env []string) (*uint16, error) {
	has := false
	for _, kv := range env {
		if len(kv) > 11 && strings.EqualFold(kv[:11], "SYSTEMROOT=") {
			has = true
		}
	}
	if !has {
		if v, ok := syscall.Getenv("SYSTEMROOT"); ok {
			env = append(env, "SYSTEMROOT="+v)
		}
	}
	var u []uint16
	for _, kv := range env {
		if strings.IndexByte(kv, 0) >= 0 {
			return nil, errors.New("environment holds a NUL")
		}
		p, err := syscall.UTF16FromString(kv)
		if err != nil {
			return nil, err
		}
		u = append(u, p...)
	}
	u = append(u, 0)
	if len(env) == 0 {
		u = append(u, 0)
	}
	return &u[0], nil
}

type ptyStartupInfoEx struct {
	syscall.StartupInfo
	attrs uintptr
}

// ptyInheritable is an inheritable duplicate of f's handle, closed by the
// caller after CreateProcessW.
func ptyInheritable(f *os.File) (syscall.Handle, error) {
	me, _ := syscall.GetCurrentProcess()
	var h syscall.Handle
	err := syscall.DuplicateHandle(me, syscall.Handle(f.Fd()), me, &h, 0, true, syscall.DUPLICATE_SAME_ACCESS)
	return h, err
}

// ptyConSpawn starts c with p's pseudo console: on its standard streams
// too (null handles under STARTF_USESTDHANDLES) with all, else with the
// streams the Cmd sets, as realProc.spawn makes them.
func ptyConSpawn(tk *Task, p *ptyFile, c *procCmd, all bool) Result[Child, ProcErr] {
	con := p.con
	p.mu.Lock()
	closed := p.closed
	p.mu.Unlock()
	if closed {
		return Err[Child](procIo("spawn: the pty is closed"))
	}
	ctx, cancel := context.WithCancel(context.Background())
	x := c.exec(ctx)
	ch := &child{x: x, cancel: cancel, group: true}
	var closeAfter []*os.File
	var dups []syscall.Handle
	defer func() {
		for _, h := range dups {
			syscall.CloseHandle(h)
		}
	}()
	fail := func(err error) Result[Child, ProcErr] {
		cancel()
		for _, f := range closeAfter {
			f.Close()
		}
		for _, f := range []*os.File{ch.stdout, ch.stderr} {
			if f != nil {
				f.Close()
			}
		}
		if w, ok := ch.stdin.(*os.File); ok && w != nil {
			w.Close()
		}
		return Err[Child](startErr(tk, c.name, err))
	}
	if x.Err != nil {
		return fail(x.Err)
	}
	var hs [3]syscall.Handle
	if !all {
		std := func(k int, f *os.File) error {
			h, err := ptyInheritable(f)
			if err != nil {
				return err
			}
			dups = append(dups, h)
			hs[k] = h
			return nil
		}
		var inData []byte
		switch {
		case c.hasStdin:
			r, w, err := os.Pipe()
			if err != nil {
				return fail(err)
			}
			closeAfter = append(closeAfter, r)
			if err := std(0, r); err != nil {
				w.Close()
				return fail(err)
			}
			inData = c.stdin
			go func() {
				w.Write(inData)
				w.Close()
			}()
		case c.inherit:
			if err := std(0, os.Stdin); err != nil {
				return fail(err)
			}
		default:
			r, w, err := os.Pipe()
			if err != nil {
				return fail(err)
			}
			closeAfter = append(closeAfter, r)
			ch.stdin = w
			if err := std(0, r); err != nil {
				return fail(err)
			}
		}
		if c.inherit {
			if err := std(1, os.Stdout); err != nil {
				return fail(err)
			}
			e := os.Stderr
			if c.merge {
				e = os.Stdout
			}
			if err := std(2, e); err != nil {
				return fail(err)
			}
		} else {
			r, w, err := os.Pipe()
			if err != nil {
				return fail(err)
			}
			closeAfter = append(closeAfter, w)
			ch.stdout = r
			if err := std(1, w); err != nil {
				return fail(err)
			}
			if c.merge {
				hs[2] = hs[1]
			} else {
				r2, w2, err := os.Pipe()
				if err != nil {
					return fail(err)
				}
				closeAfter = append(closeAfter, w2)
				ch.stderr = r2
				if err := std(2, w2); err != nil {
					return fail(err)
				}
			}
		}
	}
	// the attribute list: the pseudo console, and the handles to inherit
	var list []syscall.Handle
	for _, h := range hs {
		dup := h == 0
		for _, l := range list {
			dup = dup || l == h
		}
		if !dup {
			list = append(list, h)
		}
	}
	na := uintptr(1)
	if len(list) > 0 {
		na = 2
	}
	var sz uintptr
	ptyInitAttrs.Call(0, na, 0, uintptr(unsafe.Pointer(&sz)))
	ab := make([]byte, sz)
	al := uintptr(unsafe.Pointer(&ab[0]))
	if r, _, e := ptyInitAttrs.Call(al, na, 0, uintptr(unsafe.Pointer(&sz))); r == 0 {
		return fail(os.NewSyscallError("InitializeProcThreadAttributeList", e))
	}
	defer ptyDeleteAttrs.Call(al)
	if r, _, e := ptyUpdateAttr.Call(al, 0, ptyAttrPseudoConsole, con.hpc, unsafe.Sizeof(con.hpc), 0, 0); r == 0 {
		return fail(os.NewSyscallError("UpdateProcThreadAttribute", e))
	}
	if len(list) > 0 {
		if r, _, e := ptyUpdateAttr.Call(al, 0, ptyAttrHandleList, uintptr(unsafe.Pointer(&list[0])), uintptr(len(list))*unsafe.Sizeof(list[0]), 0, 0); r == 0 {
			return fail(os.NewSyscallError("UpdateProcThreadAttribute", e))
		}
	}
	var si ptyStartupInfoEx
	si.Cb = uint32(unsafe.Sizeof(si))
	si.Flags = syscall.STARTF_USESTDHANDLES
	si.StdInput, si.StdOutput, si.StdErr = hs[0], hs[1], hs[2]
	si.attrs = al
	cl, err := syscall.UTF16PtrFromString(ptyCmdLine(x.Args))
	if err != nil {
		return fail(err)
	}
	app, err := syscall.UTF16PtrFromString(x.Path)
	if err != nil {
		return fail(err)
	}
	var dir *uint16
	if x.Dir != "" {
		if dir, err = syscall.UTF16PtrFromString(x.Dir); err != nil {
			return fail(err)
		}
	}
	var env *uint16
	if x.Env != nil {
		if env, err = ptyEnvBlock(x.Env); err != nil {
			return fail(err)
		}
	}
	var pi syscall.ProcessInformation
	flags := uint32(ptyExtendedStartup | ptyUnicodeEnv)
	err = syscall.CreateProcess(app, cl, nil, nil, len(list) > 0, flags, env, dir, &si.StartupInfo, &pi)
	runtime.KeepAlive(ab)
	runtime.KeepAlive(list)
	if err != nil {
		return fail(os.NewSyscallError("CreateProcess", err))
	}
	syscall.CloseHandle(pi.Thread)
	proc, err := os.FindProcess(int(pi.ProcessId))
	if err != nil {
		syscall.TerminateProcess(pi.Process, 137)
		syscall.CloseHandle(pi.Process)
		return fail(err)
	}
	x.Process = proc
	for _, f := range closeAfter {
		f.Close()
	}
	go con.watch(pi.Process)
	// canceling the task that spawned the child kills it, as for spawn
	go func() {
		select {
		case <-tk.Done():
			killProcess(proc)
			cancel()
		case <-ctx.Done():
		}
	}()
	return Ok[Child, ProcErr](Child{ch})
}
