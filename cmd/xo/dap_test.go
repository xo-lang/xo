package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/xo-lang/xo/internal/xotool"
)

// dapClient speaks the Debug Adapter Protocol to `xo debug --dap`.
type dapClient struct {
	t   *testing.T
	in  io.Writer
	out *bufio.Reader
	seq int
	log []string
}

func (c *dapClient) send(cmd string, args any) {
	c.t.Helper()
	c.seq++
	m := map[string]any{"seq": c.seq, "type": "request", "command": cmd}
	if args != nil {
		m["arguments"] = args
	}
	b, _ := json.Marshal(m)
	if _, err := fmt.Fprintf(c.in, "Content-Length: %d\r\n\r\n%s", len(b), b); err != nil {
		c.t.Fatalf("send %s: %v", cmd, err)
	}
}

func (c *dapClient) recv() map[string]any {
	c.t.Helper()
	n := -1
	for {
		line, err := c.out.ReadString('\n')
		if err != nil {
			c.t.Fatalf("reading a header: %v\n%s", err, strings.Join(c.log, "\n"))
		}
		line = strings.TrimSpace(line)
		if line == "" {
			break
		}
		if v, ok := strings.CutPrefix(line, "Content-Length:"); ok {
			n, _ = strconv.Atoi(strings.TrimSpace(v))
		}
	}
	b := make([]byte, n)
	if _, err := io.ReadFull(c.out, b); err != nil {
		c.t.Fatalf("reading a body: %v", err)
	}
	c.log = append(c.log, string(b))
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		c.t.Fatalf("bad JSON %q: %v", b, err)
	}
	return m
}

// until reads messages until the response to cmd (or, with an event
// name, that event), failing on an unsuccessful response to cmd.
func (c *dapClient) until(kind, name string) map[string]any {
	c.t.Helper()
	for {
		m := c.recv()
		if m["type"] == kind && (m["command"] == name || m["event"] == name) {
			if kind == "response" && m["success"] != true {
				c.t.Fatalf("%s failed: %v\n%s", name, m["message"], strings.Join(c.log, "\n"))
			}
			return m
		}
		if m["type"] == "event" && m["event"] == "terminated" && name != "terminated" {
			c.t.Fatalf("terminated waiting for %s %s:\n%s", kind, name, strings.Join(c.log, "\n"))
		}
	}
}

func body(m map[string]any) map[string]any {
	b, _ := m["body"].(map[string]any)
	return b
}

func list(m map[string]any, k string) []map[string]any {
	var out []map[string]any
	xs, _ := m[k].([]any)
	for _, x := range xs {
		if o, ok := x.(map[string]any); ok {
			out = append(out, o)
		}
	}
	return out
}

// TestDebugDAP drives `xo debug --dap` (decision 0504) as an editor
// would: a breakpoint by full path, the stop, the stack with source
// paths, a frame's locals and a struct's fields, evaluate, next and
// stepOut, the program's output as an output event, and its exit.
func TestDebugDAP(t *testing.T) {
	if !debugLaunches() {
		t.Skip("xo debug's process control is macOS arm64 and Linux so far (decision 0253 stages 2 and 6)")
	}
	if testing.Short() {
		t.Skip("builds and debugs a native program")
	}
	setenv(t, "XO_BUILD_DIR", filepath.Join(t.TempDir(), "build"))
	dir := writeFiles(t, map[string]string{"main.xo": debugProg})
	res, err := xotool.Build([]string{dir}, buildOpts(t, xotool.Options{Backend: debugBackend(), Out: filepath.Join(t.TempDir(), "prog")}))
	if err != nil || res.Failed {
		t.Fatalf("build: %v %s", err, res.Output)
	}
	prog := res.Binaries[xotool.Host()]
	cmd := command(t, xoBin(t), "debug", "--dap")
	in, _ := cmd.StdinPipe()
	outp, _ := cmd.StdoutPipe()
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	defer func() {
		in.Close()
		select {
		case <-done:
		case <-time.After(10 * time.Second):
			cmd.Process.Kill()
		}
	}()
	c := &dapClient{t: t, in: in, out: bufio.NewReader(outp)}

	c.send("initialize", map[string]any{"adapterID": "xo"})
	if b := body(c.until("response", "initialize")); b["supportsConfigurationDoneRequest"] != true {
		t.Errorf("capabilities: %v", b)
	}
	c.send("launch", map[string]any{"program": prog})
	c.until("response", "launch")
	c.until("event", "initialized")
	src := filepath.Join(dir, "main.xo")
	c.send("setBreakpoints", map[string]any{"source": map[string]any{"path": src}, "breakpoints": []any{map[string]any{"line": 9}}})
	if bs := list(body(c.until("response", "setBreakpoints")), "breakpoints"); len(bs) != 1 || bs[0]["verified"] != true {
		t.Fatalf("breakpoints: %v", bs)
	}
	c.send("configurationDone", nil)
	st := body(c.until("event", "stopped"))
	if st["reason"] != "breakpoint" {
		t.Errorf("stopped: %v", st)
	}
	th := st["threadId"]
	c.send("stackTrace", map[string]any{"threadId": th})
	fs := list(body(c.until("response", "stackTrace")), "stackFrames")
	if len(fs) < 2 || fs[0]["name"] != "sum" || fs[0]["line"] != float64(9) || fs[1]["name"] != "main" || fs[1]["line"] != float64(17) {
		t.Fatalf("stack: %v", fs)
	}
	if s, _ := fs[0]["source"].(map[string]any); s == nil || !strings.HasSuffix(fmt.Sprint(s["path"]), "/main.xo") || !strings.HasPrefix(fmt.Sprint(s["path"]), "/") {
		t.Errorf("frame 0's source: %v", fs[0]["source"])
	}
	c.send("scopes", map[string]any{"frameId": 0})
	sc := list(body(c.until("response", "scopes")), "scopes")
	c.send("variables", map[string]any{"variablesReference": sc[0]["variablesReference"]})
	vars := map[string]map[string]any{}
	for _, v := range list(body(c.until("response", "variables")), "variables") {
		vars[fmt.Sprint(v["name"])] = v
	}
	if vars["n"]["value"] != "4" || vars["total"]["value"] != "0" || vars["p"]["value"] != "Point{x: 8, y: 7}" {
		t.Errorf("locals: %v", vars)
	}
	c.send("variables", map[string]any{"variablesReference": vars["p"]["variablesReference"]})
	if fl := list(body(c.until("response", "variables")), "variables"); len(fl) != 2 || fl[0]["name"] != "x" || fl[0]["value"] != "8" || fl[1]["value"] != "7" {
		t.Errorf("p's fields: %v", fl)
	}
	c.send("evaluate", map[string]any{"expression": "p.y", "frameId": 0})
	if r := body(c.until("response", "evaluate")); r["result"] != "7" {
		t.Errorf("evaluate p.y: %v", r)
	}
	c.send("next", map[string]any{"threadId": th})
	c.until("response", "next")
	if st := body(c.until("event", "stopped")); st["reason"] != "step" {
		t.Errorf("after next: %v", st)
	}
	c.send("stepOut", map[string]any{"threadId": th})
	c.until("response", "stepOut")
	c.until("event", "stopped")
	c.send("stackTrace", map[string]any{"threadId": th})
	fs = list(body(c.until("response", "stackTrace")), "stackFrames")
	if len(fs) == 0 || fs[0]["name"] != "main" || fs[0]["line"] != float64(18) {
		t.Errorf("after stepOut, frame 0: %v", fs)
	}
	// a std frame's source is a file of xo's std
	c.send("stepIn", map[string]any{"threadId": th})
	c.until("response", "stepIn")
	c.until("event", "stopped")
	c.send("stackTrace", map[string]any{"threadId": th})
	fs = list(body(c.until("response", "stackTrace")), "stackFrames")
	if s, _ := fs[0]["source"].(map[string]any); s == nil || !fileExists(fmt.Sprint(s["path"])) {
		t.Errorf("after stepIn, frame 0's source is no file: %v", fs[0])
	}
	c.send("setBreakpoints", map[string]any{"source": map[string]any{"path": src}, "breakpoints": []any{}})
	c.until("response", "setBreakpoints")
	c.send("continue", map[string]any{"threadId": th})
	var output strings.Builder
	exit := -1.0
	for {
		m := c.recv()
		if m["type"] == "event" && m["event"] == "output" {
			output.WriteString(fmt.Sprint(body(m)["output"]))
		}
		if m["type"] == "event" && m["event"] == "exited" {
			exit, _ = body(m)["exitCode"].(float64)
		}
		if m["type"] == "event" && m["event"] == "terminated" {
			break
		}
	}
	if output.String() != "sum 76\n" || exit != 0 {
		t.Errorf("output %q, exit %v", output.String(), exit)
	}
	c.send("disconnect", nil)
	c.until("response", "disconnect")
}

func fileExists(p string) bool {
	st, err := os.Stat(p)
	return err == nil && !st.IsDir()
}

// TestDebugDAPAttach checks DAP's attach request (decision 0504 with 0253
// stage 4): configurationDone attaches to a running program and reports
// it stopped (reason pause), its stack and threads are there, and
// disconnect detaches, leaving the program running.
func TestDebugDAPAttach(t *testing.T) {
	if runtime.GOOS != "darwin" || runtime.GOARCH != "arm64" {
		t.Skip("xo debug attaches on macOS arm64 so far (decision 0253 stage 4; Linux's needs /proc/PID/task, stage 6)")
	}
	if testing.Short() {
		t.Skip("builds and debugs a native program")
	}
	setenv(t, "XO_BUILD_DIR", filepath.Join(t.TempDir(), "build"))
	dir := writeFiles(t, map[string]string{"main.xo": debugTasks})
	res, err := xotool.Build([]string{dir}, buildOpts(t, xotool.Options{Backend: debugBackend(), Out: filepath.Join(t.TempDir(), "prog")}))
	if err != nil || res.Failed {
		t.Fatalf("build: %v %s", err, res.Output)
	}
	prog := res.Binaries[xotool.Host()]
	p := exec.Command(prog)
	if err := p.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		p.Process.Kill()
		p.Wait()
	}()
	// past dyld: a thread in the program's code (attach finds the slide
	// from one), each try detaching
	for try := 0; ; try++ {
		var o, e bytes.Buffer
		if runXo(t, []string{"debug", "-p", strconv.Itoa(p.Process.Pid), "--batch", "-c", "threads", prog}, &o, &e) == 0 {
			break
		}
		if strings.Contains(o.String()+e.String(), "task_for_pid") {
			t.Skipf("this xo cannot control the program:\n%s%s", o.String(), e.String())
		}
		if try == 20 {
			t.Fatalf("xo debug -p never attached:\n%s%s", o.String(), e.String())
		}
		time.Sleep(250 * time.Millisecond)
	}
	cmd := command(t, xoBin(t), "debug", "--dap")
	in, _ := cmd.StdinPipe()
	outp, _ := cmd.StdoutPipe()
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	defer func() {
		in.Close()
		select {
		case <-done:
		case <-time.After(10 * time.Second):
			cmd.Process.Kill()
		}
	}()
	c := &dapClient{t: t, in: in, out: bufio.NewReader(outp)}
	c.send("initialize", map[string]any{"adapterID": "xo"})
	c.until("response", "initialize")
	c.send("attach", map[string]any{"program": prog, "processId": p.Process.Pid})
	c.until("response", "attach")
	c.until("event", "initialized")
	c.send("configurationDone", nil)
	st := body(c.until("event", "stopped"))
	if st["reason"] != "pause" || !strings.Contains(fmt.Sprint(st["description"]), "attached to process") {
		t.Fatalf("stopped: %v\n%s", st, strings.Join(c.log, "\n"))
	}
	c.send("threads", nil)
	if ts := list(body(c.until("response", "threads")), "threads"); len(ts) == 0 {
		t.Errorf("threads: %v", ts)
	}
	c.send("stackTrace", map[string]any{"threadId": st["threadId"]})
	c.until("response", "stackTrace")
	c.send("disconnect", nil)
	c.until("response", "disconnect")
	if err := p.Process.Signal(syscall.Signal(0)); err != nil {
		t.Errorf("the program did not survive the detach: %v", err)
	}
}
