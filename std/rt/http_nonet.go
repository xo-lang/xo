//go:build xo_nohttp

package rt

// The real server and client of std/http (http_net.go) in a program that
// does not load std/http: such a program has no Http_Server or
// Http_Client value to call them with, so they are never reached. The
// build adds the tag xo_nohttp (internal/build) so the binary does not link
// net/http.

const noHTTP = "rt: std/http is not linked into this program (built with the tag xo_nohttp)"

func (s Http_Server) serve(tk *Task) Result[Unit, Http_HttpErr] { panic(noHTTP) }

func (c Http_Client) send(tk *Task, req Http_Request, rawURL string) Result[Http_Response, Http_ClientErr] {
	panic(noHTTP)
}
