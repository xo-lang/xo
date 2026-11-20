//go:build !xo_nohttp

package rt

import (
	"net"
	"strconv"
	"strings"
)

// isFake and dispatch pass a FakeNet's in process HTTP through (the
// client checks the request's host first, httpDenied).
func (n *onlyNet) isFake() bool {
	fd, ok := n.inner.(fakeDispatcher)
	return ok && fd.isFake()
}

func (n *onlyNet) dispatch(tk *Task, c Http_Client, req Http_Request, rawURL string) Result[Http_Response, Http_ClientErr] {
	return n.inner.(fakeDispatcher).dispatch(tk, c, req, rawURL)
}

func (n *onlyNet) serveScope(tk *Task, req Http_Request) *Scope {
	return n.inner.(fakeDispatcher).serveScope(tk, req)
}

func (n *onlyNet) dispatchRaw(tk *Task, c Http_Client, req Http_Request, rawURL string) Result[Http_Response, Http_ClientErr] {
	return n.inner.(fakeDispatcher).dispatchRaw(tk, c, req, rawURL)
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
