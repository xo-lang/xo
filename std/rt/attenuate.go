package rt

import (
	"fmt"
	"net"
	"runtime"
	"slices"
	"strconv"
	"strings"
)

// Capability attenuation (std.md 3.2, 3.5, 3.7; decision 0156):
// `Net.only(hosts)`, `Env.only(names)`, and `Proc.only(cmds)` return a
// capability of the same type whose operations outside the listed
// authority fail before any effect is performed. An attenuated value
// attenuated again keeps both lists (each operation must pass every
// list).

// ---------------------------------------------------------------------------
// Net.only

// onlyNet is a Net restricted to hosts.
type onlyNet struct {
	inner Net
	lists [][]string // every list must admit an address
}

func newOnlyNet(inner Net, hosts List[string]) Net {
	l := normHosts(hosts.s)
	if o, ok := inner.(*onlyNet); ok {
		return &onlyNet{inner: o.inner, lists: append(append([][]string{}, o.lists...), l)}
	}
	return &onlyNet{inner: inner, lists: [][]string{l}}
}

func normHosts(hs []string) []string {
	out := make([]string, len(hs))
	for i, h := range hs {
		h = strings.TrimSpace(h)
		if len(h) >= 5 && strings.EqualFold(h[:5], "unix:") {
			// a socket path keeps its case (decision 0190)
			out[i] = "unix:" + h[5:]
			continue
		}
		out[i] = strings.ToLower(h)
	}
	return out
}

// hostAdmits reports whether entry e admits host:port. Entries are a
// host (any port), `*.domain` (any subdomain, any port), or `host:port`.
func hostAdmits(e, host, port string) bool {
	switch {
	case strings.HasPrefix(e, "unix:"):
		return false // admits only its socket (admitsAddr)
	case strings.HasPrefix(e, "*."):
		suf := e[1:]
		return strings.HasSuffix(host, suf) && len(host) > len(suf)
	case strings.Count(e, ":") == 1 || strings.HasPrefix(e, "["):
		eh, ep, err := net.SplitHostPort(e)
		if err != nil {
			return false
		}
		return strings.ToLower(eh) == host && ep == port
	}
	return strings.Trim(e, "[]") == host
}

// admits reports whether every list admits host and port.
func (n *onlyNet) admits(host, port string) bool {
	host = strings.ToLower(strings.Trim(host, "[]"))
	for _, l := range n.lists {
		ok := false
		for _, e := range l {
			if hostAdmits(e, host, port) {
				ok = true
				break
			}
		}
		if !ok {
			return false
		}
	}
	return true
}

// admitsAddr checks a dial or listen address; an empty host (`:8080`)
// is the unspecified address 0.0.0.0.
func (n *onlyNet) admitsAddr(addr string) bool {
	if _, ok := unixPath(addr); ok {
		// a Unix domain socket: the entry `unix:PATH`, exactly
		// (decision 0190)
		for _, l := range n.lists {
			if !slices.Contains(l, addr) {
				return false
			}
		}
		return true
	}
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return false
	}
	if host == "" {
		host = "0.0.0.0"
	}
	return n.admits(host, port)
}

func netDenied(addr string) NetErr { return NetErr{Tag: 6, P: NetErr_Denied{F_addr: addr}} }

func (n *onlyNet) M_listen(tk *Task, addr string) Result[Listener, NetErr] {
	if !n.admitsAddr(addr) {
		return Err[Listener](netDenied(addr))
	}
	return n.inner.M_listen(tk, addr)
}

func (n *onlyNet) M_dial(tk *Task, addr string) Result[Conn, NetErr] {
	if !n.admitsAddr(addr) {
		return Err[Conn](netDenied(addr))
	}
	return n.inner.M_dial(tk, addr)
}

func (n *onlyNet) M_dial_timeout(tk *Task, addr string, d Duration) Result[Conn, NetErr] {
	if !n.admitsAddr(addr) {
		return Err[Conn](netDenied(addr))
	}
	return n.inner.M_dial_timeout(tk, addr, d)
}

func (n *onlyNet) M_lookup_host(tk *Task, host string) Result[List[string], NetErr] {
	if !n.admits(strings.ToLower(host), "0") {
		return Err[List[string]](netDenied(host))
	}
	return n.inner.M_lookup_host(tk, host)
}

func (n *onlyNet) M_only(_ *Task, hosts List[string]) Net { return newOnlyNet(n, hosts) }

// XoDebug implements Debug.
func (n *onlyNet) XoDebug(b *strings.Builder) {
	b.WriteString("<net only")
	for _, l := range n.lists {
		fmt.Fprintf(b, " %v", l)
	}
	b.WriteString(">")
}

// isFake and dispatch pass a FakeNet's in process HTTP through (the
// client checks the request's host first, httpDenied).
func (n *onlyNet) isFake() bool {
	fd, ok := n.inner.(fakeDispatcher)
	return ok && fd.isFake()
}

func (n *onlyNet) dispatch(tk *Task, req Http_Request, rawURL string) Result[Http_Response, Http_ClientErr] {
	return n.inner.(fakeDispatcher).dispatch(tk, req, rawURL)
}

// httpDenied is the error of an HTTP request a client's attenuated Net
// does not admit, if any.
func httpDenied(n Net, u Http_Url) (Http_ClientErr, bool) {
	o, ok := n.(*onlyNet)
	if !ok {
		return Http_ClientErr{}, false
	}
	port := ""
	switch {
	case u.F_port.Some:
		port = strconv.FormatInt(u.F_port.V, 10)
	case strings.EqualFold(u.F_scheme, "https"):
		port = "443"
	default:
		port = "80"
	}
	if o.admits(u.F_host, port) {
		return Http_ClientErr{}, false
	}
	return clientErr(2, "denied by Net.only: "+net.JoinHostPort(u.F_host, port)), true
}

// RealNet.M_only and the fakes' (std.md 3.2).
func (n realNet) M_only(_ *Task, hosts List[string]) Net { return newOnlyNet(n, hosts) }

// M_only is `n.only(hosts)` on the fake network of a test.
func (f Testing_FakeNet) M_only(_ *Task, hosts List[string]) Net { return newOnlyNet(f, hosts) }

// ---------------------------------------------------------------------------
// Env.only

// onlyEnv is an Env that reads only listed variables.
type onlyEnv struct {
	inner Env
	lists [][]string
}

func newOnlyEnv(inner Env, names List[string]) Env {
	l := append([]string{}, names.s...)
	if o, ok := inner.(*onlyEnv); ok {
		return &onlyEnv{inner: o.inner, lists: append(append([][]string{}, o.lists...), l)}
	}
	return &onlyEnv{inner: inner, lists: [][]string{l}}
}

func (e *onlyEnv) admits(name string) bool {
	for _, l := range e.lists {
		ok := false
		for _, n := range l {
			// Windows compares variable names without case (os.Getenv)
			if n == name || (runtime.GOOS == "windows" && strings.EqualFold(n, name)) {
				ok = true
				break
			}
		}
		if !ok {
			return false
		}
	}
	return true
}

func (e *onlyEnv) M_get(tk *Task, name string) Option[string] {
	if !e.admits(name) {
		return Option[string]{}
	}
	return e.inner.M_get(tk, name)
}

func (e *onlyEnv) M_vars(tk *Task) Map[string, string] {
	var out Map[string, string]
	for k, v := range Map_All(e.inner.M_vars(tk)) {
		if e.admits(k) {
			Map_put(nil, &out, k, v)
		}
	}
	return out
}

func (e *onlyEnv) M_args(tk *Task) List[string] { return e.inner.M_args(tk) }
func (e *onlyEnv) M_program(tk *Task) string    { return e.inner.M_program(tk) }

func (e *onlyEnv) M_home(tk *Task) Result[string, EnvErr] {
	if !e.admits("HOME") {
		return Err[string](EnvErr{Tag: 1, P: EnvErr_Missing{F_name: "HOME"}})
	}
	return e.inner.M_home(tk)
}

func (e *onlyEnv) M_only(_ *Task, names List[string]) Env { return newOnlyEnv(e, names) }

// XoDebug implements Debug.
func (e *onlyEnv) XoDebug(b *strings.Builder) {
	b.WriteString("<env only")
	for _, l := range e.lists {
		fmt.Fprintf(b, " %v", l)
	}
	b.WriteString(">")
}

func (e realEnv) M_only(_ *Task, names List[string]) Env { return newOnlyEnv(e, names) }

// M_only is `e.only(names)` on the fake environment of a test.
func (f Testing_FakeEnv) M_only(_ *Task, names List[string]) Env { return newOnlyEnv(f, names) }

// ---------------------------------------------------------------------------
// Proc.only

// onlyProc is a Proc that runs only listed commands.
type onlyProc struct {
	inner Proc
	lists [][]string
}

func newOnlyProc(inner Proc, cmds List[string]) Proc {
	l := append([]string{}, cmds.s...)
	if o, ok := inner.(*onlyProc); ok {
		return &onlyProc{inner: o.inner, lists: append(append([][]string{}, o.lists...), l)}
	}
	return &onlyProc{inner: inner, lists: [][]string{l}}
}

func (p *onlyProc) admits(cmd string) bool {
	for _, l := range p.lists {
		ok := false
		for _, c := range l {
			if c == cmd {
				ok = true
				break
			}
		}
		if !ok {
			return false
		}
	}
	return true
}

func (p *onlyProc) M_exit(tk *Task, code int64) Never { return p.inner.M_exit(tk, code) }
func (p *onlyProc) M_set_grace(tk *Task, d Duration)  { p.inner.M_set_grace(tk, d) }

func (p *onlyProc) M_run(tk *Task, cmd string, args List[string]) Result[ProcOutput, ProcErr] {
	if !p.admits(cmd) {
		return Err[ProcOutput](ProcErr{Tag: 4, P: ProcErr_Denied{F_cmd: cmd}})
	}
	return p.inner.M_run(tk, cmd, args)
}

func (p *onlyProc) M_only(_ *Task, cmds List[string]) Proc { return newOnlyProc(p, cmds) }

// XoDebug implements Debug.
func (p *onlyProc) XoDebug(b *strings.Builder) {
	b.WriteString("<proc only")
	for _, l := range p.lists {
		fmt.Fprintf(b, " %v", l)
	}
	b.WriteString(">")
}

func (p realProc) M_only(_ *Task, cmds List[string]) Proc { return newOnlyProc(p, cmds) }

// M_only is `p.only(cmds)` on the fake process control of a test.
func (f Testing_FakeProc) M_only(_ *Task, cmds List[string]) Proc { return newOnlyProc(f, cmds) }
