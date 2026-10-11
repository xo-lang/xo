package rt

import (
	"os"
	"strings"
	"unicode/utf8"
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

// ---------------------------------------------------------------------------
// std/term

// Term_color_enabled is `term.color_enabled(out, env)`: NO_COLOR set
// and not empty turns color off; FORCE_COLOR or CLICOLOR_FORCE set,
// not empty, and not "0" turns it on; TERM=dumb turns it off; otherwise
// it is on when standard output is a terminal.
func Term_color_enabled(tk *Task, out Stdio, env Env) bool {
	if v := env.M_get(tk, "NO_COLOR"); v.Some && v.V != "" {
		return false
	}
	for _, n := range []string{"FORCE_COLOR", "CLICOLOR_FORCE"} {
		if v := env.M_get(tk, n); v.Some && v.V != "" && v.V != "0" {
			return true
		}
	}
	if v := env.M_get(tk, "TERM"); v.Some && v.V == "dumb" {
		return false
	}
	return out.M_stdout_is_tty(tk)
}

// Term_painter is `term.painter(out, env)`.
func Term_painter(tk *Task, out Stdio, env Env) Term_Painter {
	return Term_Painter{F_on: Term_color_enabled(tk, out, env)}
}

func (p Term_Painter) sgr(code, s string) string {
	if !p.F_on || s == "" {
		return s
	}
	return "\x1b[" + code + "m" + s + "\x1b[0m"
}

func (p Term_Painter) M_bold(_ *Task, s string) string      { return p.sgr("1", s) }
func (p Term_Painter) M_dim(_ *Task, s string) string       { return p.sgr("2", s) }
func (p Term_Painter) M_underline(_ *Task, s string) string { return p.sgr("4", s) }
func (p Term_Painter) M_red(_ *Task, s string) string       { return p.sgr("31", s) }
func (p Term_Painter) M_green(_ *Task, s string) string     { return p.sgr("32", s) }
func (p Term_Painter) M_yellow(_ *Task, s string) string    { return p.sgr("33", s) }
func (p Term_Painter) M_blue(_ *Task, s string) string      { return p.sgr("34", s) }
func (p Term_Painter) M_magenta(_ *Task, s string) string   { return p.sgr("35", s) }
func (p Term_Painter) M_cyan(_ *Task, s string) string      { return p.sgr("36", s) }
func (p Term_Painter) M_gray(_ *Task, s string) string      { return p.sgr("90", s) }

// Term_strip is `term.strip(s)`: s without ANSI escape sequences (CSI
// sequences `ESC [ ... final byte`, and any other `ESC x` pair).
func Term_strip(_ *Task, s string) string {
	if !strings.Contains(s, "\x1b") {
		return s
	}
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] != 0x1b {
			b.WriteByte(s[i])
			continue
		}
		if i+1 < len(s) && s[i+1] == '[' {
			j := i + 2
			for j < len(s) && !(s[j] >= 0x40 && s[j] <= 0x7e) {
				j++
			}
			i = j
			continue
		}
		i++
	}
	if r := b.String(); utf8.ValidString(r) {
		return r
	}
	// an escape that swallowed part of a UTF-8 sequence: the text as it was
	return s
}
