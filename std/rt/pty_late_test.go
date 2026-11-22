//go:build darwin || freebsd || (linux && (amd64 || arm64 || riscv64))

package rt

import (
	"strings"
	"testing"
	"time"
)

// TestPtyOutputBeforeFirstRead: a child that writes and exits before the
// master's first read keeps its output. On FreeBSD the parent's slave copy
// was still open when such a child (its session leader) exited, and the
// revoke at that exit dropped the unread output: empty in 40 of 40 runs on
// a FreeBSD VM (gotchas/pty-darwin-master-and-ctty.md).
func TestPtyOutputBeforeFirstRead(t *testing.T) {
	tk := NewRootTask()
	for i := 0; i < 5; i++ {
		o := Pty_open(tk, realProc{})
		if !o.Ok {
			t.Fatalf("open: %v", o.E)
		}
		p, path := o.V.F0, o.V.F1
		c := Pty_spawn(tk, p, realProc{}.M_cmd(tk, "/bin/sh", ListOf("-c", "echo early "+path)), Pty_Attach{Tag: 1}, None[Pty_Size]())
		if !c.Ok {
			t.Fatalf("spawn: %v", c.E)
		}
		time.Sleep(100 * time.Millisecond)
		var out strings.Builder
		for {
			r := p.M_read(tk, 4096)
			if !r.Ok {
				t.Fatalf("read: %v", r.E)
			}
			if !r.V.Some {
				break
			}
			out.WriteString(string(r.V.V))
		}
		if !strings.Contains(out.String(), "early "+path) {
			t.Errorf("run %d: output %q, want early %s", i, out.String(), path)
		}
		if w := c.V.M_wait(tk); !w.Ok || w.V != 0 {
			t.Errorf("run %d: wait %+v", i, w)
		}
		p.M_close(tk)
	}
}

// TestPtyCloseEndsRead (creack/pty io_test.go TestReadClose): a close from
// another goroutine ends a read waiting on the master as the end, as on
// the native backends. Before, Linux's read returned os.ErrClosed as an Io
// error, and the BSD select loop went on polling the closed descriptor's
// number (EBADF, or a file opened after it under the same number).
func TestPtyCloseEndsRead(t *testing.T) {
	tk := NewRootTask()
	o := Pty_open(tk, realProc{})
	if !o.Ok {
		t.Fatalf("open: %v", o.E)
	}
	p := o.V.F0
	c := Pty_spawn(tk, p, realProc{}.M_cmd(tk, "/bin/sleep", ListOf("10")), Pty_Attach{Tag: 1}, None[Pty_Size]())
	if !c.Ok {
		t.Fatalf("spawn: %v", c.E)
	}
	defer func() {
		c.V.M_kill(tk)
		c.V.M_wait(tk)
	}()
	go func() {
		time.Sleep(100 * time.Millisecond)
		p.M_close(tk)
	}()
	t0 := time.Now()
	r := p.M_read(tk, 16)
	if !r.Ok || r.V.Some {
		t.Errorf("read after close %+v, want the end (Ok none)", r)
	}
	if d := time.Since(t0); d > 5*time.Second {
		t.Errorf("read took %v", d)
	}
}
