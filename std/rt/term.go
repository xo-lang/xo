package rt

import (
	"os"
	"sync"
)

// Terminals (std.md 3.6 and 15, decision 0162): whether the standard
// streams are terminals, the terminal's width, input without echo, binary
// standard input, and std/term's colors. The platform parts are in
// term_unix.go, term_windows.go, and term_other.go.

func (realStdio) M_stdin_is_tty(*Task) bool  { return isTerminal(os.Stdin) }
func (realStdio) M_stdout_is_tty(*Task) bool { return isTerminal(os.Stdout) }
func (realStdio) M_stderr_is_tty(*Task) bool { return isTerminal(os.Stderr) }

// M_width is the columns of the first of standard output, error, and
// input that is a terminal.
func (realStdio) M_width(*Task) Option[int64] {
	for _, f := range []*os.File{os.Stdout, os.Stderr, os.Stdin} {
		if w := termWidth(f); w > 0 {
			return Some(int64(w))
		}
	}
	return None[int64]()
}

// rawRestore restores standard input's terminal after M_raw_input(true).
var (
	rawMu      sync.Mutex
	rawRestore func()
)

// M_raw_input sets standard input's terminal to raw input (on true) or
// restores it (on false), decision 0581: whether it changed a terminal.
func (realStdio) M_raw_input(_ *Task, on bool) bool {
	rawMu.Lock()
	defer rawMu.Unlock()
	if !on {
		if rawRestore != nil {
			rawRestore()
			rawRestore = nil
		}
		return false
	}
	if rawRestore != nil {
		return true
	}
	rawRestore = rawOn(os.Stdin)
	return rawRestore != nil
}

// rawReset restores the terminal if raw input is on (fxFinish: every
// exit, fault and interrupt).
func rawReset() {
	rawMu.Lock()
	defer rawMu.Unlock()
	if rawRestore != nil {
		rawRestore()
		rawRestore = nil
	}
}

// M_read_secret writes prompt to standard error and reads a line with
// the terminal's echo off (when standard input is a terminal).
func (s realStdio) M_read_secret(tk *Task, prompt string) Result[Option[string], IoErr] {
	if stdinStopped(tk) {
		return Err[Option[string]](IoErr{Tag: 3})
	}
	outWrite(os.Stderr, prompt)
	return fx(tk, "stdio.read_secret", true, nil, func() Result[Option[string], IoErr] {
		restore := echoOff(os.Stdin)
		r := s.readLine(tk)
		if restore != nil {
			restore()
			// the newline the user typed was not echoed
			outWrite(os.Stderr, "\n")
		}
		return r
	})
}

// M_read_bytes reads at most max bytes of standard input, as they come.
func (realStdio) M_read_bytes(tk *Task, max int64) Result[Option[Bytes], IoErr] {
	if stdinStopped(tk) {
		return Err[Option[Bytes]](IoErr{Tag: 3})
	}
	return fx(tk, "stdio.read_bytes", true, func() []any { return []any{max} }, func() Result[Option[Bytes], IoErr] {
		if max <= 0 {
			return Ok[Option[Bytes], IoErr](Some(Bytes("")))
		}
		data, ok, r := stdinReadSome(tk, int(min(max, 1<<20)))
		if !r.Ok {
			return Err[Option[Bytes]](r.E)
		}
		if !ok {
			return Ok[Option[Bytes], IoErr](Option[Bytes]{})
		}
		return Ok[Option[Bytes], IoErr](Some(Bytes(data)))
	})
}

// M_read_all_bytes is the rest of standard input.
func (s realStdio) M_read_all_bytes(tk *Task) Result[Bytes, IoErr] {
	r := s.M_read_all(tk)
	if !r.Ok {
		return Err[Bytes](r.E)
	}
	return Ok[Bytes, IoErr](Bytes(r.V))
}

// ---------------------------------------------------------------------------
// A test's FakeStdio: not a terminal unless set_tty

// M_raw_input: a test's FakeStdio has no terminal to set.
func (f Testing_FakeStdio) M_raw_input(tk *Task, on bool) bool {
	if f.s.real != nil {
		return f.s.real.M_raw_input(tk, on)
	}
	return false
}

func (f Testing_FakeStdio) M_stdin_is_tty(tk *Task) bool {
	if f.s.real != nil {
		return f.s.real.M_stdin_is_tty(tk)
	}
	return f.tty() > 0
}

func (f Testing_FakeStdio) M_stdout_is_tty(tk *Task) bool {
	if f.s.real != nil {
		return f.s.real.M_stdout_is_tty(tk)
	}
	return f.tty() > 0
}

func (f Testing_FakeStdio) M_stderr_is_tty(tk *Task) bool {
	if f.s.real != nil {
		return f.s.real.M_stderr_is_tty(tk)
	}
	return f.tty() > 0
}

func (f Testing_FakeStdio) M_width(tk *Task) Option[int64] {
	if f.s.real != nil {
		return f.s.real.M_width(tk)
	}
	if w := f.tty(); w > 0 {
		return Some(w)
	}
	return None[int64]()
}

func (f Testing_FakeStdio) tty() int64 {
	f.s.mu.Lock()
	defer f.s.mu.Unlock()
	return f.s.width
}

// M_set_tty makes the fake streams terminals of width columns (0: not
// terminals).
func (f Testing_FakeStdio) M_set_tty(_ *Task, width int64) {
	f.s.mu.Lock()
	f.s.width = max(width, 0)
	f.s.mu.Unlock()
}

// M_set_input_bytes is set_input for any bytes.
func (f Testing_FakeStdio) M_set_input_bytes(tk *Task, data Bytes) { f.M_set_input(tk, string(data)) }

// M_read_secret writes the prompt to the fake standard error and reads a
// line of the fake input.
func (f Testing_FakeStdio) M_read_secret(tk *Task, prompt string) Result[Option[string], IoErr] {
	if f.s.real != nil {
		return f.s.real.M_read_secret(tk, prompt)
	}
	f.write(true, prompt)
	return f.M_read_line(tk)
}

func (f Testing_FakeStdio) M_read_bytes(tk *Task, limit int64) Result[Option[Bytes], IoErr] {
	if f.s.real != nil {
		return f.s.real.M_read_bytes(tk, limit)
	}
	if stdinStopped(tk) {
		return Err[Option[Bytes]](IoErr{Tag: 3})
	}
	f.s.mu.Lock()
	defer f.s.mu.Unlock()
	rest := f.s.input[f.s.offset:]
	if rest == "" {
		return Ok[Option[Bytes], IoErr](Option[Bytes]{})
	}
	if limit <= 0 {
		return Ok[Option[Bytes], IoErr](Some(Bytes("")))
	}
	n := min(int64(len(rest)), limit)
	f.s.offset += int(n)
	return Ok[Option[Bytes], IoErr](Some(Bytes(rest[:n])))
}

func (f Testing_FakeStdio) M_read_all_bytes(tk *Task) Result[Bytes, IoErr] {
	if f.s.real != nil {
		return f.s.real.M_read_all_bytes(tk)
	}
	r := f.M_read_all(tk)
	if !r.Ok {
		return Err[Bytes](r.E)
	}
	return Ok[Bytes, IoErr](Bytes(r.V))
}
