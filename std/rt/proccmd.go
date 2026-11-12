package rt

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"syscall"
	"time"
)

// Commands, children, and pipelines (std.md 3.7, decision 0161): `Proc.cmd`
// builds a Cmd; `output`, `run`, `spawn`, and `Proc.pipeline` start it.
// A Cmd remembers the attenuation lists of the Proc that built it
// (Proc.only, decision 0156) and its backend: the real processes, or a
// test's FakeProc.

// procBackend starts commands: realProc or a test's fakeProc.
type procBackend interface {
	output(tk *Task, c *procCmd) Result[ProcOutput, ProcErr]
	runTerm(tk *Task, c *procCmd) Result[int64, ProcErr]
	spawn(tk *Task, c *procCmd) Result[Child, ProcErr]
	pipeline(tk *Task, cs []*procCmd) Result[ProcOutput, ProcErr]
}

type procCmd struct {
	be       procBackend
	lists    [][]string // Proc.only lists, each must admit name
	name     string
	args     []string
	dir      string
	env      []string // NAME=VALUE; nil inherits the program's
	envSet   bool
	extra    []string // set_env, after env
	stdin    []byte
	hasStdin bool
	inherit  bool
	merge    bool
}

// Cmd is `Cmd`, a command that has not run: a value (each builder
// method returns a changed copy).
type Cmd struct{ c *procCmd }

func (Cmd) XoDebug(b *strings.Builder) { b.WriteString("<cmd>") }
func (c Cmd) M_debug(*Task) string     { return Debug(c) }

func newCmd(be procBackend, name string, args List[string]) Cmd {
	return Cmd{&procCmd{be: be, name: name, args: append([]string(nil), args.s...)}}
}

func (c Cmd) clone() *procCmd {
	x := *c.c
	x.extra = append([]string(nil), c.c.extra...)
	return &x
}

func (c Cmd) M_dir(_ *Task, path string) Cmd {
	x := c.clone()
	x.dir = path
	return Cmd{x}
}

// M_env makes the child's environment exactly the variables e shows.
func (c Cmd) M_env(tk *Task, e Env) Cmd {
	x := c.clone()
	m := e.M_vars(tk)
	x.env = nil
	for _, k := range Map_keys(tk, m).s {
		x.env = append(x.env, k+"="+Map_get(tk, m, k).V)
	}
	x.envSet = true
	return Cmd{x}
}

func (c Cmd) M_set_env(_ *Task, name, value string) Cmd {
	x := c.clone()
	x.extra = append(x.extra, name+"="+value)
	return Cmd{x}
}

func (c Cmd) M_stdin(_ *Task, data Bytes) Cmd {
	x := c.clone()
	x.stdin, x.hasStdin = []byte(data), true
	return Cmd{x}
}

func (c Cmd) M_inherit(*Task) Cmd {
	x := c.clone()
	x.inherit = true
	return Cmd{x}
}

func (c Cmd) M_merge_stderr(*Task) Cmd {
	x := c.clone()
	x.merge = true
	return Cmd{x}
}

// denied is the ProcErr.Denied of a command outside an attenuation.
func (c *procCmd) denied() *ProcErr {
	for _, l := range c.lists {
		if !cliContains(l, c.name) {
			e := ProcErr{Tag: 4, P: ProcErr_Denied{F_cmd: c.name}}
			return &e
		}
	}
	return nil
}

func (c Cmd) M_output(tk *Task) Result[ProcOutput, ProcErr] {
	if e := c.c.denied(); e != nil {
		return Err[ProcOutput](*e)
	}
	if tk.Canceled() {
		return Err[ProcOutput](ProcErr{Tag: 3})
	}
	return fx(tk, "proc.output", true, c.c.fxArgs, func() Result[ProcOutput, ProcErr] { return c.c.be.output(tk, c.c) })
}

func (c Cmd) M_run(tk *Task) Result[int64, ProcErr] {
	if e := c.c.denied(); e != nil {
		return Err[int64](*e)
	}
	if tk.Canceled() {
		return Err[int64](ProcErr{Tag: 3})
	}
	return fx(tk, "proc.run_cmd", true, c.c.fxArgs, func() Result[int64, ProcErr] { return c.c.be.runTerm(tk, c.c) })
}

func (c Cmd) M_spawn(tk *Task) Result[Child, ProcErr] {
	if e := c.c.denied(); e != nil {
		return Err[Child](*e)
	}
	if tk.Canceled() {
		return Err[Child](ProcErr{Tag: 3})
	}
	return c.c.be.spawn(tk, c.c)
}

// fxArgs are the recorded arguments of a command (record and replay).
func (c *procCmd) fxArgs() []any {
	return []any{c.name, ListOf(c.args...), c.dir, ListOf(c.env...), ListOf(c.extra...), Bytes(c.stdin), c.inherit, c.merge}
}

// procPipeline is `p.pipeline(cmds)` of any Proc.
func procPipeline(tk *Task, cmds List[Cmd]) Result[ProcOutput, ProcErr] {
	if len(cmds.s) == 0 {
		return Ok[ProcOutput, ProcErr](ProcOutput{F_stdout: Bytes(""), F_stderr: Bytes("")})
	}
	var cs []*procCmd
	for _, c := range cmds.s {
		if e := c.c.denied(); e != nil {
			return Err[ProcOutput](*e)
		}
		cs = append(cs, c.c)
	}
	if tk.Canceled() {
		return Err[ProcOutput](ProcErr{Tag: 3})
	}
	return fx(tk, "proc.pipeline", true, func() []any {
		var a []any
		for _, c := range cs {
			a = append(a, c.fxArgs()...)
		}
		return a
	}, func() Result[ProcOutput, ProcErr] { return cs[0].be.pipeline(tk, cs) })
}

// ---------------------------------------------------------------------------
// Real processes

func procNotFound(name string) ProcErr {
	return ProcErr{Tag: 1, P: ProcErr_NotFound{F_cmd: name}}
}

func procIo(msg string) ProcErr { return ProcErr{Tag: 2, P: ProcErr_Io{F_msg: msg}} }

// exitCode is the status of a finished command: its exit code, or 128
// plus the signal that ended it.
func exitCode(err error) (int64, bool) {
	var ee *exec.ExitError
	if !errors.As(err, &ee) {
		return 0, false
	}
	if ws, ok := ee.Sys().(syscall.WaitStatus); ok && ws.Signaled() {
		return 128 + int64(ws.Signal()), true
	}
	return int64(ee.ExitCode()), true
}

func (c *procCmd) exec(ctx context.Context) *exec.Cmd {
	x := exec.CommandContext(ctx, c.name, c.args...)
	x.Dir = c.dir
	if c.envSet || len(c.extra) > 0 {
		env := c.env
		if !c.envSet {
			env = os.Environ()
		}
		x.Env = append(append([]string{}, env...), c.extra...)
	}
	return x
}

// startErr is the ProcErr of a command that could not start.
func startErr(tk *Task, name string, err error) ProcErr {
	switch {
	case errors.Is(err, exec.ErrNotFound), errors.Is(err, os.ErrNotExist):
		return procNotFound(name)
	case tk.Canceled():
		return ProcErr{Tag: 3}
	}
	return procIo(err.Error())
}

// cancelCtx is a context canceled with tk.
func cancelCtx(tk *Task) (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		select {
		case <-tk.Done():
			cancel()
		case <-ctx.Done():
		}
	}()
	return ctx, cancel
}

func (realProc) output(tk *Task, c *procCmd) Result[ProcOutput, ProcErr] {
	ctx, cancel := cancelCtx(tk)
	defer cancel()
	x := c.exec(ctx)
	var out, errb bytes.Buffer
	x.Stdout = &out
	x.Stderr = &errb
	if c.merge {
		x.Stderr = &out
	}
	if c.hasStdin {
		x.Stdin = bytes.NewReader(c.stdin)
	} else if c.inherit {
		x.Stdin = os.Stdin
	}
	tk.leave()
	err := x.Run()
	usage := noteChild(x.ProcessState)
	tk.enter()
	code := int64(0)
	if err != nil {
		n, ok := exitCode(err)
		if !ok {
			return Err[ProcOutput](startErr(tk, c.name, err))
		}
		code = n
	}
	return Ok[ProcOutput, ProcErr](ProcOutput{F_code: code, F_stdout: Bytes(out.String()), F_stderr: Bytes(errb.String()), F_usage: usage})
}

func (realProc) runTerm(tk *Task, c *procCmd) Result[int64, ProcErr] {
	ctx, cancel := cancelCtx(tk)
	defer cancel()
	x := c.exec(ctx)
	x.Stdin, x.Stdout, x.Stderr = os.Stdin, os.Stdout, os.Stderr
	if c.merge {
		x.Stderr = os.Stdout
	}
	if c.hasStdin {
		x.Stdin = bytes.NewReader(c.stdin)
	}
	tk.leave()
	err := x.Run()
	_ = noteChild(x.ProcessState)
	tk.enter()
	if err != nil {
		n, ok := exitCode(err)
		if !ok {
			return Err[int64](startErr(tk, c.name, err))
		}
		return Ok[int64, ProcErr](n)
	}
	return Ok[int64, ProcErr](0)
}

// child is a started command.
type child struct {
	x      *exec.Cmd
	cancel context.CancelFunc
	stdin  io.WriteCloser
	stdout *os.File
	stderr *os.File
	lines  *bufio.Reader // over pr, for read_line
	pr     *pipeReader
	mu     sync.Mutex
	done   bool
	code   int64
	err    error
	usage  ProcUsage
	fake   *fakeChild
	rdl    time.Time // the read deadline of both pipes (decision 0611), zero none
	part   string    // a line's start read before a failed read_line, kept for the next
}

// readDL is the child's read deadline.
func (ch *child) readDL() time.Time {
	ch.mu.Lock()
	defer ch.mu.Unlock()
	return ch.rdl
}

// M_set_read_deadline is `c.set_read_deadline(clock, t)` (decision 0611):
// os.File.SetReadDeadline on the child's output and error pipes.
func (c Child) M_set_read_deadline(tk *Task, clock Clock, t Option[Time]) {
	var at time.Time
	if t.Some {
		at = time.Now().Add(time.Duration(int64(t.V) - int64(clock.M_now(tk))))
	}
	ch := c.c
	ch.mu.Lock()
	ch.rdl = at
	ch.mu.Unlock()
	if ch.fake != nil {
		return
	}
	for _, f := range []*os.File{ch.stdout, ch.stderr} {
		if f != nil {
			f.SetReadDeadline(at)
		}
	}
}

// Child is `Child`: a started command (a handle).
type Child struct{ c *child }

func (Child) XoDebug(b *strings.Builder) { b.WriteString("<child>") }
func (c Child) M_debug(*Task) string     { return Debug(c) }

func (realProc) spawn(tk *Task, c *procCmd) Result[Child, ProcErr] {
	ctx, cancel := context.WithCancel(context.Background())
	x := c.exec(ctx)
	ch := &child{x: x, cancel: cancel}
	var closeAfter []*os.File
	pipe := func() (*os.File, *os.File, error) {
		r, w, err := os.Pipe()
		if err == nil {
			closeAfter = append(closeAfter, w)
		}
		return r, w, err
	}
	fail := func(err error) Result[Child, ProcErr] {
		cancel()
		for _, f := range closeAfter {
			f.Close()
		}
		return Err[Child](startErr(tk, c.name, err))
	}
	switch {
	case c.hasStdin:
		x.Stdin = bytes.NewReader(c.stdin)
	case c.inherit:
		x.Stdin = os.Stdin
	default:
		w, err := x.StdinPipe()
		if err != nil {
			return fail(err)
		}
		ch.stdin = w
	}
	if c.inherit {
		x.Stdout, x.Stderr = os.Stdout, os.Stderr
		if c.merge {
			x.Stderr = os.Stdout
		}
	} else {
		r, w, err := pipe()
		if err != nil {
			return fail(err)
		}
		ch.stdout, x.Stdout = r, w
		if c.merge {
			x.Stderr = w
		} else {
			r2, w2, err := pipe()
			if err != nil {
				return fail(err)
			}
			ch.stderr, x.Stderr = r2, w2
		}
	}
	if err := x.Start(); err != nil {
		if ch.stdout != nil {
			ch.stdout.Close()
		}
		if ch.stderr != nil {
			ch.stderr.Close()
		}
		return fail(err)
	}
	for _, f := range closeAfter {
		f.Close()
	}
	// canceling the task that spawned the child kills it
	go func() {
		select {
		case <-tk.Done():
			cancel()
		case <-ctx.Done():
		}
	}()
	return Ok[Child, ProcErr](Child{ch})
}

func (c Child) M_pid(*Task) int64 {
	if c.c.fake != nil || c.c.x.Process == nil {
		return 0
	}
	return int64(c.c.x.Process.Pid)
}

func (c Child) M_write(tk *Task, data Bytes) Result[Unit, ProcErr] {
	if tk.Canceled() {
		return Err[Unit](ProcErr{Tag: 3})
	}
	if c.c.fake != nil {
		return Ok[Unit, ProcErr](Unit{})
	}
	if c.c.stdin == nil {
		return Err[Unit](procIo("the child's input is not a pipe (stdin data or inherit was set)"))
	}
	tk.leave()
	_, err := c.c.stdin.Write([]byte(data))
	tk.enter()
	if err != nil {
		return Err[Unit](procIo(err.Error()))
	}
	return Ok[Unit, ProcErr](Unit{})
}

func (c Child) M_close_stdin(*Task) {
	if c.c.stdin != nil {
		c.c.stdin.Close()
	}
}

// readPipe reads at most max bytes of f; None at the end.
func readPipe(tk *Task, ch *child, f *os.File, max int64, what string) Result[Option[Bytes], ProcErr] {
	if tk.Canceled() {
		return Err[Option[Bytes]](ProcErr{Tag: 3})
	}
	if f == nil {
		return Err[Option[Bytes]](procIo("the child's " + what + " is not a pipe (inherit or merge_stderr was set)"))
	}
	if max <= 0 {
		return Ok[Option[Bytes], ProcErr](Some(Bytes("")))
	}
	buf := make([]byte, min(max, 1<<20))
	var n int
	canceled, err := interruptibleTo(tk, f.SetReadDeadline, ch.readDL(), func() error {
		var e error
		n, e = f.Read(buf)
		return e
	})
	if canceled {
		return Err[Option[Bytes]](ProcErr{Tag: 3})
	}
	if n > 0 {
		return Ok[Option[Bytes], ProcErr](Some(Bytes(buf[:n])))
	}
	if err == nil || errors.Is(err, io.EOF) || errors.Is(err, os.ErrClosed) {
		return Ok[Option[Bytes], ProcErr](Option[Bytes]{})
	}
	if errors.Is(err, os.ErrDeadlineExceeded) {
		return Err[Option[Bytes]](ProcErr{Tag: 5})
	}
	return Err[Option[Bytes]](procIo(err.Error()))
}

func (c Child) M_read(tk *Task, max int64) Result[Option[Bytes], ProcErr] {
	if c.c.fake != nil {
		return c.c.fake.read(tk, false, max)
	}
	c.c.mu.Lock()
	lr := c.c.lines
	if part := c.c.part; part != "" && max > 0 {
		// a line's start kept by a timed out read_line comes first
		k := min(int64(len(part)), max)
		c.c.part = part[k:]
		c.c.mu.Unlock()
		return Ok[Option[Bytes], ProcErr](Some(Bytes(part[:k])))
	}
	c.c.mu.Unlock()
	if lr != nil && lr.Buffered() > 0 {
		// bytes read ahead by read_line come first
		buf := make([]byte, min(int64(lr.Buffered()), max))
		n, _ := lr.Read(buf)
		return Ok[Option[Bytes], ProcErr](Some(Bytes(buf[:n])))
	}
	return readPipe(tk, c.c, c.c.stdout, max, "output")
}

func (c Child) M_read_err(tk *Task, max int64) Result[Option[Bytes], ProcErr] {
	if c.c.fake != nil {
		return c.c.fake.read(tk, true, max)
	}
	return readPipe(tk, c.c, c.c.stderr, max, "error stream")
}

// pipeReader reads the child's output for read_line, through
// readPipe (so a read is a cancellation point).
type pipeReader struct {
	tk *Task
	ch *child
	f  *os.File
	e  *ProcErr
}

func (r *pipeReader) Read(p []byte) (int, error) {
	res := readPipe(r.tk, r.ch, r.f, int64(len(p)), "output")
	if !res.Ok {
		r.e = &res.E
		return 0, io.ErrUnexpectedEOF
	}
	if !res.V.Some {
		return 0, io.EOF
	}
	return copy(p, res.V.V), nil
}

func (c Child) M_read_line(tk *Task) Result[Option[string], ProcErr] {
	if c.c.fake != nil {
		return c.c.fake.readLine(tk)
	}
	if c.c.stdout == nil {
		return Err[Option[string]](procIo("the child's output is not a pipe (inherit was set)"))
	}
	c.c.mu.Lock()
	if c.c.lines == nil {
		c.c.pr = &pipeReader{ch: c.c, f: c.c.stdout}
		c.c.lines = bufio.NewReader(c.c.pr)
	}
	pr, lr := c.c.pr, c.c.lines
	c.c.mu.Unlock()
	// the reader's source reads with this task
	pr.tk, pr.e = tk, nil
	line, err := lr.ReadString('\n')
	if pr.e != nil {
		// a timeout keeps the line's start (native keeps it read ahead)
		c.c.part += line
		return Err[Option[string]](*pr.e)
	}
	line, c.c.part = c.c.part+line, ""
	if line == "" && err != nil {
		return Ok[Option[string], ProcErr](Option[string]{})
	}
	line = strings.TrimSuffix(strings.TrimSuffix(line, "\n"), "\r")
	return Ok[Option[string], ProcErr](Some(textOf(line)))
}

func (c Child) M_wait(tk *Task) Result[int64, ProcErr] {
	if c.c.fake != nil {
		return Ok[int64, ProcErr](c.c.fake.code)
	}
	ch := c.c
	ch.mu.Lock()
	if ch.done {
		defer ch.mu.Unlock()
		return ch.result(tk)
	}
	ch.mu.Unlock()
	if ch.stdin != nil {
		ch.stdin.Close()
	}
	tk.leave()
	err := ch.x.Wait()
	u := noteChild(ch.x.ProcessState)
	tk.enter()
	ch.mu.Lock()
	defer ch.mu.Unlock()
	if !ch.done {
		ch.done, ch.err, ch.usage = true, err, u
		ch.cancel()
	}
	return ch.result(tk)
}

// M_wait_usage is Child.wait_usage (decision 0426's addendum): wait and
// the child's own usage (zeros for a fake child).
func (c Child) M_wait_usage(tk *Task) Result[Tuple2[int64, ProcUsage], ProcErr] {
	r := c.M_wait(tk)
	if !r.Ok {
		return Result[Tuple2[int64, ProcUsage], ProcErr]{E: r.E, Tr: r.Tr}
	}
	var u ProcUsage
	if c.c.fake == nil {
		c.c.mu.Lock()
		u = c.c.usage
		c.c.mu.Unlock()
	}
	return Ok[Tuple2[int64, ProcUsage], ProcErr](Tuple2[int64, ProcUsage]{F0: r.V, F1: u})
}

func (ch *child) result(tk *Task) Result[int64, ProcErr] {
	if ch.err == nil {
		return Ok[int64, ProcErr](0)
	}
	if n, ok := exitCode(ch.err); ok {
		return Ok[int64, ProcErr](n)
	}
	if tk.Canceled() {
		return Err[int64](ProcErr{Tag: 3})
	}
	return Err[int64](procIo(ch.err.Error()))
}

func (c Child) M_kill(*Task) {
	if c.c.fake != nil || c.c.x.Process == nil {
		return
	}
	killProcess(c.c.x.Process)
}

func (realProc) pipeline(tk *Task, cs []*procCmd) Result[ProcOutput, ProcErr] {
	ctx, cancel := cancelCtx(tk)
	defer cancel()
	xs := make([]*exec.Cmd, len(cs))
	errs := make([]bytes.Buffer, len(cs))
	var out bytes.Buffer
	var closeAfter []*os.File
	for i, c := range cs {
		xs[i] = c.exec(ctx)
		xs[i].Stderr = &errs[i]
	}
	if cs[0].hasStdin {
		xs[0].Stdin = bytes.NewReader(cs[0].stdin)
	}
	for i := 0; i+1 < len(xs); i++ {
		r, w, err := os.Pipe()
		if err != nil {
			return Err[ProcOutput](procIo(err.Error()))
		}
		xs[i].Stdout, xs[i+1].Stdin = w, r
		if cs[i].merge {
			xs[i].Stderr = w
		}
		closeAfter = append(closeAfter, r, w)
	}
	last := len(xs) - 1
	xs[last].Stdout = &out
	if cs[last].merge {
		xs[last].Stderr = &out
	}
	started := 0
	var startFail error
	for i, x := range xs {
		if err := x.Start(); err != nil {
			startFail = err
			for _, y := range xs[:i] {
				killProcess(y.Process)
			}
			break
		}
		started++
	}
	for _, f := range closeAfter {
		f.Close()
	}
	tk.leave()
	waits := make([]error, started)
	// the commands' usage: their times added, the largest peak
	var usage ProcUsage
	for i := 0; i < started; i++ {
		waits[i] = xs[i].Wait()
		u := noteChild(xs[i].ProcessState)
		usage.F_user += u.F_user
		usage.F_system += u.F_system
		usage.F_max_rss = max(usage.F_max_rss, u.F_max_rss)
	}
	tk.enter()
	if startFail != nil {
		return Err[ProcOutput](startErr(tk, cs[started].name, startFail))
	}
	code := int64(0)
	for _, err := range waits {
		if err == nil {
			continue
		}
		n, ok := exitCode(err)
		if !ok {
			if tk.Canceled() {
				return Err[ProcOutput](ProcErr{Tag: 3})
			}
			return Err[ProcOutput](procIo(err.Error()))
		}
		if n != 0 {
			code = n
		}
	}
	var eb bytes.Buffer
	for i := range errs {
		eb.Write(errs[i].Bytes())
	}
	return Ok[ProcOutput, ProcErr](ProcOutput{F_code: code, F_stdout: Bytes(out.String()), F_stderr: Bytes(eb.String()), F_usage: usage})
}
