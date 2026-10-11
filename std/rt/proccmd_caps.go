package rt

import (
	"strings"
	"sync"
)

// ---------------------------------------------------------------------------
// The capabilities' methods (std.md 3.7, decision 0161)

// M_cmd is `p.cmd(name, args)`.
func (p realProc) M_cmd(_ *Task, name string, args List[string]) Cmd { return newCmd(p, name, args) }

// M_pipeline is `p.pipeline(cmds)`.
func (realProc) M_pipeline(tk *Task, cmds List[Cmd]) Result[ProcOutput, ProcErr] {
	return procPipeline(tk, cmds)
}

// M_cmd of an attenuated Proc: the command keeps the lists.
func (p *onlyProc) M_cmd(tk *Task, name string, args List[string]) Cmd {
	c := p.inner.M_cmd(tk, name, args)
	x := c.clone()
	x.lists = append(append([][]string(nil), x.lists...), p.lists...)
	return Cmd{x}
}

// M_pipeline of an attenuated Proc checks every command against its
// lists too, wherever the command was made.
func (p *onlyProc) M_pipeline(tk *Task, cmds List[Cmd]) Result[ProcOutput, ProcErr] {
	for _, c := range cmds.s {
		if !p.admits(c.c.name) {
			return Err[ProcOutput](ProcErr{Tag: 4, P: ProcErr_Denied{F_cmd: c.c.name}})
		}
	}
	return procPipeline(tk, cmds)
}

func (f Testing_FakeProc) M_cmd(tk *Task, name string, args List[string]) Cmd {
	if f.p.real != nil {
		return f.p.real.M_cmd(tk, name, args)
	}
	return newCmd(f.p, name, args)
}

func (f Testing_FakeProc) M_pipeline(tk *Task, cmds List[Cmd]) Result[ProcOutput, ProcErr] {
	return procPipeline(tk, cmds)
}

// ---------------------------------------------------------------------------
// A test's FakeProc: commands answer from on_run (std.md 8.4)

func (p *fakeProc) lookup(name string) (ProcOutput, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	out, ok := p.runs[name]
	return out, ok
}

func (p *fakeProc) output(_ *Task, c *procCmd) Result[ProcOutput, ProcErr] {
	out, ok := p.lookup(c.name)
	if !ok {
		return Err[ProcOutput](procNotFound(c.name))
	}
	if c.merge {
		out.F_stdout, out.F_stderr = out.F_stdout+out.F_stderr, Bytes("")
	}
	return Ok[ProcOutput, ProcErr](out)
}

func (p *fakeProc) runTerm(_ *Task, c *procCmd) Result[int64, ProcErr] {
	out, ok := p.lookup(c.name)
	if !ok {
		return Err[int64](procNotFound(c.name))
	}
	if s := p.stdio; s != nil {
		Testing_FakeStdio{s}.write(false, string(out.F_stdout))
		Testing_FakeStdio{s}.write(!c.merge, string(out.F_stderr))
	}
	return Ok[int64, ProcErr](out.F_code)
}

// fakeChild is a FakeProc child: its streams hold the registered output.
type fakeChild struct {
	mu       sync.Mutex
	out, err string
	code     int64
}

func (p *fakeProc) spawn(_ *Task, c *procCmd) Result[Child, ProcErr] {
	out, ok := p.lookup(c.name)
	if !ok {
		return Err[Child](procNotFound(c.name))
	}
	fc := &fakeChild{out: string(out.F_stdout), err: string(out.F_stderr), code: out.F_code}
	if c.merge {
		fc.out, fc.err = fc.out+fc.err, ""
	}
	return Ok[Child, ProcErr](Child{&child{fake: fc}})
}

func (fc *fakeChild) read(tk *Task, errStream bool, max int64) Result[Option[Bytes], ProcErr] {
	if tk.Canceled() {
		return Err[Option[Bytes]](ProcErr{Tag: 3})
	}
	fc.mu.Lock()
	defer fc.mu.Unlock()
	s := &fc.out
	if errStream {
		s = &fc.err
	}
	if *s == "" {
		return Ok[Option[Bytes], ProcErr](Option[Bytes]{})
	}
	if max <= 0 {
		return Ok[Option[Bytes], ProcErr](Some(Bytes("")))
	}
	n := min(int64(len(*s)), max)
	b := (*s)[:n]
	*s = (*s)[n:]
	return Ok[Option[Bytes], ProcErr](Some(Bytes(b)))
}

func (fc *fakeChild) readLine(tk *Task) Result[Option[string], ProcErr] {
	if tk.Canceled() {
		return Err[Option[string]](ProcErr{Tag: 3})
	}
	fc.mu.Lock()
	defer fc.mu.Unlock()
	if fc.out == "" {
		return Ok[Option[string], ProcErr](Option[string]{})
	}
	line := fc.out
	if i := strings.IndexByte(fc.out, '\n'); i >= 0 {
		line, fc.out = fc.out[:i], fc.out[i+1:]
	} else {
		fc.out = ""
	}
	return Ok[Option[string], ProcErr](Some(strings.TrimSuffix(line, "\r")))
}

func (p *fakeProc) pipeline(tk *Task, cs []*procCmd) Result[ProcOutput, ProcErr] {
	var res ProcOutput
	var eb strings.Builder
	for _, c := range cs {
		r := p.output(tk, c)
		if !r.Ok {
			return r
		}
		eb.WriteString(string(r.V.F_stderr))
		if r.V.F_code != 0 {
			res.F_code = r.V.F_code
		}
		res.F_stdout = r.V.F_stdout
	}
	res.F_stderr = Bytes(eb.String())
	return Ok[ProcOutput, ProcErr](res)
}
