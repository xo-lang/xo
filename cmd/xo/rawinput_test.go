package main

import (
	"context"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// TestStdioRawInput: decision 0581. Under a pseudo terminal (script(1)),
// Stdio.raw_input(true) returns a key with no Enter, a pipe is no
// terminal (false), and a program that faults with raw input on leaves
// the terminal cooked (stty shows icanon and echo again), on the native
// backend and the Go backend.
func TestStdioRawInput(t *testing.T) {
	if testing.Short() {
		t.Skip("builds programs")
	}
	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" {
		t.Skip("script(1) and stty on darwin and linux")
	}
	if _, err := exec.LookPath("script"); err != nil {
		t.Skip("no script(1) for a pseudo terminal")
	}
	t.Parallel()
	dir := writeFiles(t, map[string]string{
		"key/xo.mod":    "module example.com/key\n",
		"key/main.xo":   "fn main(os: Os) {\n  let set = os.stdio.raw_input(true)\n  let b = os.stdio.read_bytes(1).ok() ?? None\n  let off = os.stdio.raw_input(false)\n  os.stdio.println(\"set=${set} byte=${(b ?? b\"\").len()} off=${off}\")\n}\n",
		"fault/xo.mod":  "module example.com/fault\n",
		"fault/main.xo": "fn main(os: Os) {\n  let _s = os.stdio.raw_input(true)\n  let xs: List[Int] = []\n  os.stdio.println(\"${xs[3]}\")\n}\n",
	})
	// script(1) runs cmd under a pseudo terminal, its input from in
	pty := func(in string, cmd string) string {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		var c *exec.Cmd
		if runtime.GOOS == "darwin" {
			c = exec.CommandContext(ctx, "script", "-q", "/dev/null", "sh", "-c", cmd)
		} else {
			c = exec.CommandContext(ctx, "script", "-qec", cmd, "/dev/null")
		}
		c.Stdin = strings.NewReader(in)
		out, _ := c.CombinedOutput()
		return strings.ReplaceAll(string(out), "\r", "")
	}
	// the default (native) backend and the Go backend
	for _, be := range []string{"", "go"} {
		flags := []string{"build"}
		if be != "" {
			flags = append(flags, "--backend="+be)
		}
		key := filepath.Join(dir, "key-"+be)
		fault := filepath.Join(dir, "fault-"+be)
		chdir(t, filepath.Join(dir, "key"))
		xo(t, append(flags, "-o", key, ".")...).want(0)
		chdir(t, filepath.Join(dir, "fault"))
		xo(t, append(flags, "-o", fault, ".")...).want(0)
		if out := pty("a", key); !strings.Contains(out, "set=true byte=1 off=false") {
			t.Errorf("%s: a key under a terminal: %q", be, out)
		}
		c := exec.Command(key)
		c.Stdin = strings.NewReader("a")
		if out, _ := c.CombinedOutput(); !strings.Contains(string(out), "set=false byte=1 off=false") {
			t.Errorf("%s: a pipe: %q", be, out)
		}
		out := pty("", fault+" >/dev/null 2>&1; stty -a")
		if strings.Contains(out, "-icanon") || strings.Contains(out, "-echo ") || !strings.Contains(out, "icanon") {
			t.Errorf("%s: the terminal after a fault with raw input on is not cooked: %q", be, out)
		}
	}
}
