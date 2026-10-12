package rt

import (
	"fmt"
	"strconv"
	"strings"
)

// Proc.kill (std.md 3.7, decision 0190): sends a signal to another
// process, by name. The names are the portable ones; an attenuated Proc
// must list "kill" (attenuate.go).

// signalNames are the signals Proc.kill sends, in the order std.md lists
// them.
var signalNames = []string{"HUP", "INT", "QUIT", "KILL", "TERM", "USR1", "USR2", "CONT", "STOP"}

// killCheck validates a kill's arguments: the error of an unknown signal
// name or a pid that is not one process, if any.
func killCheck(pid int64, sig string) *ProcErr {
	known := false
	for _, n := range signalNames {
		known = known || n == sig
	}
	if !known {
		e := ProcErr{Tag: 2, P: ProcErr_Io{F_msg: "unknown signal " + strconv.Quote(sig)}}
		return &e
	}
	if pid <= 0 {
		// 0 and negative pids name process groups: never sent
		e := ProcErr{Tag: 2, P: ProcErr_Io{F_msg: fmt.Sprintf("invalid process id %d", pid)}}
		return &e
	}
	return nil
}

// M_kill is `os.proc.kill(pid, signal)`.
func (p realProc) M_kill(tk *Task, pid int64, sig string) Result[Unit, ProcErr] {
	if tk.Canceled() {
		return Err[Unit](ProcErr{Tag: 3})
	}
	if e := killCheck(pid, sig); e != nil {
		return Err[Unit](*e)
	}
	return fx(tk, "proc.kill", false, func() []any { return []any{pid, sig} }, func() Result[Unit, ProcErr] {
		if err := sendSignal(int(pid), sig); err != nil {
			return Err[Unit](ProcErr{Tag: 2, P: ProcErr_Io{F_msg: err.Error()}})
		}
		return Ok[Unit, ProcErr](Unit{})
	})
}

// M_kill on an attenuated Proc: only when every list names "kill".
func (p *onlyProc) M_kill(tk *Task, pid int64, sig string) Result[Unit, ProcErr] {
	if !p.admits("kill") {
		return Err[Unit](ProcErr{Tag: 4, P: ProcErr_Denied{F_cmd: "kill"}})
	}
	return p.inner.M_kill(tk, pid, sig)
}

// M_kill on a test's FakeProc records the signal (M_killed); nothing is
// sent.
func (f Testing_FakeProc) M_kill(tk *Task, pid int64, sig string) Result[Unit, ProcErr] {
	if f.p.real != nil {
		return f.p.real.M_kill(tk, pid, sig)
	}
	if e := killCheck(pid, sig); e != nil {
		return Err[Unit](*e)
	}
	f.p.mu.Lock()
	f.p.killed = append(f.p.killed, sig+" "+strconv.FormatInt(pid, 10))
	f.p.mu.Unlock()
	return Ok[Unit, ProcErr](Unit{})
}

// M_killed lists the signals kill recorded, as "NAME PID".
func (f Testing_FakeProc) M_killed(*Task) List[string] {
	f.p.mu.Lock()
	defer f.p.mu.Unlock()
	return ListOf(append([]string(nil), f.p.killed...)...)
}

// unixPath reports whether a Net address names a Unix domain socket
// (`unix:PATH`, decision 0190) and returns the path.
func unixPath(addr string) (string, bool) {
	return strings.CutPrefix(addr, "unix:")
}
