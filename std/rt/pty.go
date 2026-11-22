package rt

import (
	"errors"
	"io"
	"os"
	"sync"
	"syscall"
	"time"
)

// std/pty on the Go backend (decision 0740): github.com/creack/pty's
// Open, Start, Getsize, Setsize and InheritSize, and golang.org/x/term's
// MakeRaw, over syscall. The platform parts are in pty_unix.go (darwin,
// linux) and pty_other.go (an Io error).

const ptyUnsupported = "pseudo-terminals are not supported on this system"

// ptyFile is a master and the slave open kept until a spawn takes it
// (macOS refuses the master's winsize and termios ioctls until the slave
// was opened; gotchas/pty-darwin-master-and-ctty.md).
type ptyFile struct {
	mu     sync.Mutex
	m      *os.File
	slave  *os.File
	path   string
	rdl    time.Time
	saved  []byte // the termios before set_raw(true), nil none
	closed bool
	con    *ptyCon // Windows: the pseudo console, m its output (pty_windows.go)
}

// Pty_Pty is `pty.Pty`, a handle.
type Pty_Pty struct{ p *ptyFile }

// Pty_RawState is `pty.RawState`: the Stdio make_raw changed.
type Pty_RawState struct {
	io      Stdio
	changed bool
}

func ptyIo(what string, err error) ProcErr { return procIo(what + ": " + err.Error()) }

func Pty_open(tk *Task, _ Proc) Result[Tuple2[Pty_Pty, string], ProcErr] {
	if ptyConOS {
		c, m, err := ptyConOpen()
		if c == nil && err == nil {
			// no ConPTY on this Windows (before 10 1809)
			return Err[Tuple2[Pty_Pty, string]](procIo(ptyUnsupported))
		}
		if err != nil {
			return Err[Tuple2[Pty_Pty, string]](ptyIo("open", err))
		}
		return Ok[Tuple2[Pty_Pty, string], ProcErr](Tuple2[Pty_Pty, string]{Pty_Pty{&ptyFile{m: m, con: c}}, ""})
	}
	if !ptySupported {
		return Err[Tuple2[Pty_Pty, string]](procIo(ptyUnsupported))
	}
	m, s, path, err := ptyOpen()
	if err != nil {
		return Err[Tuple2[Pty_Pty, string]](ptyIo("open", err))
	}
	return Ok[Tuple2[Pty_Pty, string], ProcErr](Tuple2[Pty_Pty, string]{Pty_Pty{&ptyFile{m: m, slave: s, path: path}}, path})
}

// takeSlave is the slave for a child: the one open kept, else the path's
// opened again.
func (p *ptyFile) takeSlave() (*os.File, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return nil, os.ErrClosed
	}
	if s := p.slave; s != nil {
		p.slave = nil
		return s, nil
	}
	return ptyOpenSlave(p.path)
}

func Pty_spawn(tk *Task, p Pty_Pty, cmd Cmd, attach Pty_Attach, size Option[Pty_Size]) Result[Child, ProcErr] {
	all := attach.Tag == 1 // Attach.All (tags count from 1)
	if all && (cmd.c.hasStdin || cmd.c.merge) {
		return Err[Child](procIo("a pty child's standard streams are the terminal"))
	}
	if _, ok := cmd.c.be.(realProc); !ok {
		return Err[Child](procIo("std/pty starts real processes only"))
	}
	if size.Some {
		if r := p.M_set_size(tk, size.V); !r.Ok {
			return Err[Child](r.E)
		}
	}
	if p.p.con != nil {
		return ptyConSpawn(tk, p.p, cmd.c, all)
	}
	s, err := p.p.takeSlave()
	if err != nil {
		return Err[Child](ptyIo("spawn", err))
	}
	x := cmd.clone()
	x.tty, x.ttyAll = s, all
	c := Cmd{x}.M_spawn(tk)
	pid := 0
	if c.Ok {
		pid = int(c.V.M_pid(tk))
	}
	ptyRelease(s, pid)
	return c
}

func Pty_start(tk *Task, proc Proc, cmd Cmd, attach Pty_Attach, size Option[Pty_Size]) Result[Tuple2[Pty_Pty, Child], ProcErr] {
	o := Pty_open(tk, proc)
	if !o.Ok {
		return Err[Tuple2[Pty_Pty, Child]](o.E)
	}
	p := o.V.F0
	c := Pty_spawn(tk, p, cmd, attach, size)
	if !c.Ok {
		p.M_close(tk)
		return Err[Tuple2[Pty_Pty, Child]](c.E)
	}
	return Ok[Tuple2[Pty_Pty, Child], ProcErr](Tuple2[Pty_Pty, Child]{p, c.V})
}

func (p *Pty_Pty) master() (*os.File, time.Time) {
	p.p.mu.Lock()
	defer p.p.mu.Unlock()
	if p.p.closed {
		return nil, time.Time{}
	}
	return p.p.m, p.p.rdl
}

func (p *Pty_Pty) M_read(tk *Task, max int64) Result[Option[Bytes], ProcErr] {
	if max < 0 || max > 1<<20 {
		return Err[Option[Bytes]](procIo("read: max is 0 to 1 MiB"))
	}
	if tk.Canceled() {
		return Err[Option[Bytes]](ProcErr{Tag: 3})
	}
	m, dl := p.master()
	if m == nil {
		return Err[Option[Bytes]](procIo("read: the pty is closed"))
	}
	if max == 0 {
		return Ok[Option[Bytes], ProcErr](Some(Bytes("")))
	}
	buf := make([]byte, max)
	var n int
	var canceled bool
	var err error
	if c := p.p.con; c != nil {
		n, canceled, err = c.read(tk, buf, dl)
	} else {
		n, canceled, err = ptyRead(tk, m, buf, dl)
	}
	if canceled {
		return Err[Option[Bytes]](ProcErr{Tag: 3})
	}
	if n > 0 {
		return Ok[Option[Bytes], ProcErr](Some(Bytes(buf[:n])))
	}
	// linux reads EIO once no process holds the slave
	if err == nil || errors.Is(err, io.EOF) || errors.Is(err, syscall.EIO) {
		return Ok[Option[Bytes], ProcErr](Option[Bytes]{})
	}
	if errors.Is(err, os.ErrDeadlineExceeded) {
		return Err[Option[Bytes]](ProcErr{Tag: 5})
	}
	return Err[Option[Bytes]](ptyIo("read", err))
}

func (p *Pty_Pty) M_write(tk *Task, data Bytes) Result[Unit, ProcErr] {
	m, _ := p.master()
	if m == nil {
		return Err[Unit](procIo("write: the pty is closed"))
	}
	if c := p.p.con; c != nil {
		if err := c.write([]byte(data)); err != nil {
			return Err[Unit](ptyIo("write", err))
		}
		return Ok[Unit, ProcErr](Unit{})
	}
	if _, err := m.WriteString(string(data)); err != nil {
		return Err[Unit](ptyIo("write", err))
	}
	return Ok[Unit, ProcErr](Unit{})
}

func (p *Pty_Pty) M_set_read_deadline(tk *Task, clock Clock, t Option[Time]) {
	var at time.Time
	if t.Some {
		at = time.Now().Add(time.Duration(int64(t.V) - int64(clock.M_now(tk))))
	}
	p.p.mu.Lock()
	p.p.rdl = at
	m := p.p.m
	closed := p.p.closed
	p.p.mu.Unlock()
	if !closed {
		ptySetDeadline(m, at)
	}
}

func (p *Pty_Pty) M_size(tk *Task) Result[Pty_Size, ProcErr] {
	m, _ := p.master()
	if m == nil {
		return Err[Pty_Size](procIo("size: the pty is closed"))
	}
	if c := p.p.con; c != nil {
		return Ok[Pty_Size, ProcErr](c.size())
	}
	s, err := ptyGetSize(m)
	if err != nil {
		return Err[Pty_Size](ptyIo("size", err))
	}
	return Ok[Pty_Size, ProcErr](s)
}

func (p *Pty_Pty) M_set_size(tk *Task, s Pty_Size) Result[Unit, ProcErr] {
	m, _ := p.master()
	if m == nil {
		return Err[Unit](procIo("set_size: the pty is closed"))
	}
	if c := p.p.con; c != nil {
		if err := c.resize(s); err != nil {
			return Err[Unit](ptyIo("set_size", err))
		}
		return Ok[Unit, ProcErr](Unit{})
	}
	if err := ptySetSize(m, s); err != nil {
		return Err[Unit](ptyIo("set_size", err))
	}
	return Ok[Unit, ProcErr](Unit{})
}

func (p *Pty_Pty) M_set_raw(tk *Task, on bool) Result[Unit, ProcErr] {
	p.p.mu.Lock()
	defer p.p.mu.Unlock()
	if p.p.closed {
		return Err[Unit](procIo("set_raw: the pty is closed"))
	}
	if p.p.con != nil {
		// a pseudo console has no line discipline the parent sets
		return Ok[Unit, ProcErr](Unit{})
	}
	saved, err := ptySetRaw(p.p.m, on, p.p.saved)
	if err != nil {
		return Err[Unit](ptyIo("set_raw", err))
	}
	p.p.saved = saved
	return Ok[Unit, ProcErr](Unit{})
}

func (p *Pty_Pty) M_close(*Task) {
	p.p.mu.Lock()
	defer p.p.mu.Unlock()
	if p.p.closed {
		return
	}
	p.p.closed = true
	if p.p.slave != nil {
		p.p.slave.Close()
		p.p.slave = nil
	}
	if p.p.con != nil {
		p.p.con.close()
	}
	p.p.m.Close()
}

func Pty_inherit_size(tk *Task, from Stdio, to Pty_Pty) Result[Unit, ProcErr] {
	for _, f := range []*os.File{os.Stdin, os.Stdout, os.Stderr} {
		if s, err := ptyGetSize(f); err == nil {
			return to.M_set_size(tk, s)
		}
	}
	return Err[Unit](procIo("inherit_size: this program has no terminal"))
}

func Pty_make_raw(tk *Task, io Stdio) Result[Pty_RawState, ProcErr] {
	if !io.M_stdin_is_tty(tk) {
		return Err[Pty_RawState](procIo("make_raw: standard input is not a terminal"))
	}
	return Ok[Pty_RawState, ProcErr](Pty_RawState{io: io, changed: io.M_raw_input(tk, true)})
}

func (s *Pty_RawState) M_restore(tk *Task) Result[Unit, ProcErr] {
	if s.changed {
		s.io.M_raw_input(tk, false)
	}
	return Ok[Unit, ProcErr](Unit{})
}
