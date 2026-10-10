package rt

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

// h2Payload is the response text of request n: a size per class, the
// shortest held in net/http's 4 KB handler buffer, the others passing
// through it in chunks, and one past arenaKeep (dropped at release).
func h2Payload(n int) string {
	sizes := []int{100, 3000, 9000, 40000, 70000}
	return strconv.Itoa(n) + ":" + strings.Repeat(string(rune('a'+n%26)), sizes[n%len(sizes)])
}

// TestServeNoH2C: on a plain TCP listener (every Xo listener) the server
// speaks HTTP/1 only: a client that insists on unencrypted HTTP/2 (h2c
// with prior knowledge) fails, an HTTP/1 client gets HTTP/1.1.
func TestServeNoH2C(t *testing.T) {
	if testing.Short() {
		t.Skip("server round trips")
	}
	var r Http_Router
	Http_Router_handle[Never](nil, &r, "GET /x", func(tk *Task, req Http_Request) Result[Http_Response, Never] {
		return Ok[Http_Response, Never](Http_text(tk, 200, "x"))
	})
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	srv := Http_Server{l: Listener{l: ln, addr: addr}, r: r}
	tk := NewRootTask()
	served := make(chan Result[Unit, Http_HttpErr], 1)
	go func() { served <- srv.serve(tk) }()
	defer func() {
		tk.node.cancel()
		<-served
	}()
	var p http.Protocols
	p.SetUnencryptedHTTP2(true)
	h2c := &http.Client{Transport: &http.Transport{Protocols: &p}, Timeout: 5 * time.Second}
	if resp, err := h2c.Get("http://" + addr + "/x"); err == nil {
		resp.Body.Close()
		t.Errorf("h2c request served over %s; the arena audit assumes HTTP/1 on plain listeners", resp.Proto)
	}
	resp, err := http.Get("http://" + addr + "/x")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.ProtoMajor != 1 {
		t.Errorf("served over %s", resp.Proto)
	}
}

// TestArenaHTTP2 serves arena responses over HTTP/2 (decision 0066 audit
// update): many concurrent streams on one connection, every arena
// poisoned when it is released, so a body or header that net/http still
// read after the handler returned would reach the client as '#' bytes.
// Some requests are canceled mid body (RST_STREAM) to take the error
// paths of the HTTP/2 writer. The Xo server listens on plain TCP and does
// not speak h2c; HTTP/2 is reached here through a TLS listener made
// directly: `Server.with_tls` serves HTTP/1.1 only (decision 0106), so
// this test is the guard for the day it speaks HTTP/2.
func TestArenaHTTP2(t *testing.T) {
	if testing.Short() {
		t.Skip("HTTP/2 server round trips")
	}
	oldOn, oldPoison := arenasOn, arenaPoison
	arenasOn, arenaPoison = true, true
	defer func() { arenasOn, arenaPoison = oldOn, oldPoison }()

	// a certificate and a client that trusts it, from httptest
	ts := httptest.NewUnstartedServer(http.NotFoundHandler())
	ts.EnableHTTP2 = true
	ts.StartTLS()
	cfg := ts.TLS.Clone()
	client := ts.Client()
	ts.Close()
	cfg.NextProtos = []string{"h2", "http/1.1"}

	var r Http_Router
	Http_Router_handle[Never](nil, &r, "GET /j/{n:Int}", func(tk *Task, req Http_Request) Result[Http_Response, Never] {
		n, _ := strconv.Atoi(req.M_param(tk, "n"))
		return Ok[Http_Response, Never](Http_json_arena(tk, 200, h2Payload(n)))
	})
	Http_Router_arenaLast(&r)
	Http_Router_handle[Never](nil, &r, "POST /p", func(tk *Task, req Http_Request) Result[Http_Response, Never] {
		v := Http_Request_json[string](tk, req)
		if !v.Ok {
			return Ok[Http_Response, Never](Http_text_arena(tk, 400, "bad body"))
		}
		return Ok[Http_Response, Never](Http_json_arena(tk, 200, v.V))
	})
	Http_Router_arenaLast(&r)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	srv := Http_Server{l: Listener{l: tls.NewListener(ln, cfg), addr: addr}, r: r}
	tk := NewRootTask()
	served := make(chan Result[Unit, Http_HttpErr], 1)
	go func() { served <- srv.serve(tk) }()
	defer func() {
		tk.node.cancel()
		select {
		case <-served:
		case <-time.After(10 * time.Second):
			t.Error("serve did not end")
		}
	}()

	const workers, per = 12, 40
	var wg sync.WaitGroup
	var mu sync.Mutex
	var bad []string
	report := func(format string, args ...any) {
		mu.Lock()
		defer mu.Unlock()
		if len(bad) < 10 {
			bad = append(bad, fmt.Sprintf(format, args...))
		}
	}
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			for i := 0; i < per; i++ {
				n := w*per + i
				want := `"` + h2Payload(n) + `"`
				var req *http.Request
				ctx, cancel := context.WithCancel(context.Background())
				if n%2 == 0 {
					req, _ = http.NewRequestWithContext(ctx, "GET", "https://"+addr+"/j/"+strconv.Itoa(n), nil)
				} else {
					req, _ = http.NewRequestWithContext(ctx, "POST", "https://"+addr+"/p", strings.NewReader(want))
				}
				resp, err := client.Do(req)
				if err != nil {
					cancel()
					report("request %d: %v", n, err)
					continue
				}
				if resp.ProtoMajor != 2 {
					report("request %d: served over %s", n, resp.Proto)
				}
				if n%5 == 4 {
					// read a little, then reset the stream
					io.ReadFull(resp.Body, make([]byte, 50))
					cancel()
					resp.Body.Close()
					continue
				}
				got, err := io.ReadAll(resp.Body)
				resp.Body.Close()
				cancel()
				switch {
				case err != nil:
					report("request %d: body: %v", n, err)
				case resp.StatusCode != 200 || resp.Header.Get("Content-Type") != "application/json":
					report("request %d: status %d type %q", n, resp.StatusCode, resp.Header.Get("Content-Type"))
				case string(got) != want:
					i := 0
					for i < len(got) && i < len(want) && got[i] == want[i] {
						i++
					}
					report("request %d: body differs at byte %d of %d (got %d bytes, %q...)", n, i, len(want), len(got), got[i:min(len(got), i+20)])
				}
			}
		}(w)
	}
	wg.Wait()
	for _, b := range bad {
		t.Error(b)
	}
}
