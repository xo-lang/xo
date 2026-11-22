//go:build windows

package rt

import (
	"strings"
	"testing"
	"time"
)

// std/pty over a ConPTY on the Go backend (decision 0740 stage 2, task
// #704): the checks the native backend's Windows program makes.

func ptyWinDrain(t *testing.T, tk *Task, p Pty_Pty) string {
	t.Helper()
	var out strings.Builder
	for {
		r := p.M_read(tk, 4096)
		if !r.Ok {
			t.Fatalf("read: %v (so far %q)", r.E, out.String())
		}
		if !r.V.Some {
			return out.String()
		}
		out.WriteString(string(r.V.V))
	}
}

func ptyWinStart(t *testing.T, tk *Task, attach int, name string, args ...string) (Pty_Pty, Child) {
	t.Helper()
	o := Pty_open(tk, realProc{})
	if !o.Ok {
		t.Fatalf("open: %v", o.E)
	}
	if o.V.F1 != "" {
		t.Errorf("slave path %q, want empty", o.V.F1)
	}
	p := o.V.F0
	c := Pty_spawn(tk, p, realProc{}.M_cmd(tk, name, ListOf(args...)), Pty_Attach{Tag: attach}, None[Pty_Size]())
	if !c.Ok {
		t.Fatalf("spawn: %v", c.E)
	}
	return p, c.V
}

func TestPtyWindowsOutputThenEnd(t *testing.T) {
	tk := NewRootTask()
	p, c := ptyWinStart(t, tk, 1, "cmd.exe", "/c", "echo early-xo")
	defer p.M_close(tk)
	if out := ptyWinDrain(t, tk, p); !strings.Contains(out, "early-xo") {
		t.Errorf("output %q, want early-xo", out)
	}
	if w := c.M_wait(tk); !w.Ok || w.V != 0 {
		t.Errorf("wait %+v", w)
	}
}

func TestPtyWindowsExitCode(t *testing.T) {
	tk := NewRootTask()
	p, c := ptyWinStart(t, tk, 1, "cmd.exe", "/c", "exit 7")
	defer p.M_close(tk)
	ptyWinDrain(t, tk, p)
	if w := c.M_wait(tk); !w.Ok || w.V != 7 {
		t.Errorf("wait %+v, want 7", w)
	}
}

func TestPtyWindowsWrite(t *testing.T) {
	tk := NewRootTask()
	// cmd expands %L% before set /p runs, so the line is read by powershell
	p, c := ptyWinStart(t, tk, 1, "powershell.exe", "-NoProfile", "-Command", `$l = Read-Host; Write-Output "got-$l"`)
	defer p.M_close(tk)
	if r := p.M_write(tk, Bytes("abc\r")); !r.Ok {
		t.Fatalf("write: %v", r.E)
	}
	if out := ptyWinDrain(t, tk, p); !strings.Contains(out, "got-abc") {
		t.Errorf("output %q, want got-abc", out)
	}
	c.M_wait(tk)
}

func TestPtyWindowsSize(t *testing.T) {
	tk := NewRootTask()
	o := Pty_open(tk, realProc{})
	if !o.Ok {
		t.Fatalf("open: %v", o.E)
	}
	p := o.V.F0
	defer p.M_close(tk)
	if s := p.M_size(tk); !s.Ok || s.V.F_rows != 24 || s.V.F_cols != 80 {
		t.Errorf("size at open %+v, want 24 by 80", s)
	}
	if r := p.M_set_size(tk, Pty_Size{F_rows: 30, F_cols: 100}); !r.Ok {
		t.Fatalf("set_size: %v", r.E)
	}
	if s := p.M_size(tk); !s.Ok || s.V.F_rows != 30 || s.V.F_cols != 100 {
		t.Errorf("size %+v, want 30 by 100", s)
	}
	if r := p.M_set_raw(tk, true); !r.Ok {
		t.Errorf("set_raw: %v", r.E)
	}
	// the child sees the size
	c := Pty_spawn(tk, p, realProc{}.M_cmd(tk, "powershell.exe", ListOf("-NoProfile", "-Command", `Write-Output "sz-$([Console]::WindowWidth)x$([Console]::WindowHeight)"`)), Pty_Attach{Tag: 1}, None[Pty_Size]())
	if !c.Ok {
		t.Fatalf("spawn: %v", c.E)
	}
	if out := ptyWinDrain(t, tk, p); !strings.Contains(out, "sz-100x30") {
		t.Errorf("output %q, want sz-100x30", out)
	}
	c.V.M_wait(tk)
}

func TestPtyWindowsControllingOnly(t *testing.T) {
	tk := NewRootTask()
	p, c := ptyWinStart(t, tk, 2, "cmd.exe", "/c", "echo piped-xo")
	defer p.M_close(tk)
	var out strings.Builder
	for {
		r := c.M_read(tk, 4096)
		if !r.Ok {
			t.Fatalf("child read: %v", r.E)
		}
		if !r.V.Some {
			break
		}
		out.WriteString(string(r.V.V))
	}
	if !strings.Contains(out.String(), "piped-xo") {
		t.Errorf("child stdout %q, want piped-xo", out.String())
	}
	if w := c.M_wait(tk); !w.Ok || w.V != 0 {
		t.Errorf("wait %+v", w)
	}
}

func TestPtyWindowsReadDeadline(t *testing.T) {
	tk := NewRootTask()
	p, c := ptyWinStart(t, tk, 1, "powershell.exe", "-NoProfile", "-Command", "Start-Sleep 5")
	defer p.M_close(tk)
	ptyWinDrainUntil := func() ProcErr {
		for {
			r := p.M_read(tk, 4096)
			if !r.Ok {
				return r.E
			}
			if !r.V.Some {
				return ProcErr{}
			}
		}
	}
	p.p.mu.Lock()
	p.p.rdl = time.Now().Add(1500 * time.Millisecond)
	p.p.mu.Unlock()
	t0 := time.Now()
	if e := ptyWinDrainUntil(); e.Tag != 5 {
		t.Errorf("read ended with %+v, want the deadline (tag 5)", e)
	}
	if d := time.Since(t0); d > 4*time.Second {
		t.Errorf("deadline read took %v", d)
	}
	c.M_kill(tk)
	c.M_wait(tk)
}
