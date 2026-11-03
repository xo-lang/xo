package selfhost

import (
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/xo-lang/xo/internal/testtmp"
	"github.com/xo-lang/xo/internal/xotool"
)

// TestWasmComponentClient runs std's HTTP client in a wasi:http component
// under `wasmtime serve` against a Go server (decision 0447): fields, body,
// redirects, gzip, errors. XO_WASM_HOST=spin serves it with `spin up`, whose
// spin.toml allows the test servers' origins (allowed_outbound_hosts) and no
// other, so a request to an unlisted origin must fail.
func TestWasmComponentClient(t *testing.T) {
	if testing.Short() {
		t.Skip("builds a program")
	}
	host := os.Getenv("XO_WASM_HOST")
	if host == "" {
		host = "wasmtime"
	}
	if host != "wasmtime" && host != "spin" {
		t.Fatalf("XO_WASM_HOST=%s: want wasmtime or spin", host)
	}
	for _, tool := range []string{"wasm-tools", host} {
		if _, err := exec.LookPath(tool); err != nil {
			t.Skipf("%s not found", tool)
		}
	}
	xo, err := xotool.Path()
	if err != nil {
		t.Fatal(err)
	}
	root := repoRoot(t)
	comp := filepath.Join(t.TempDir(), "client.wasm")
	build := testtmp.Guard(exec.Command(xo, "build", "--targets=wasip2/wasm", "-o", comp, "."))
	build.Dir = filepath.Join(root, "internal", "selfhost", "testdata", "wasm_http_client")
	build.Env = append(os.Environ(), "XO_STD="+filepath.Join(root, "std"), "XO_BIN_CACHE=off")
	if b, err := build.CombinedOutput(); err != nil {
		t.Fatalf("xo build --targets=wasip2/wasm: %v\n%s", err, b)
	}
	if b, err := testtmp.Guard(exec.Command("wasm-tools", "validate", comp)).CombinedOutput(); err != nil {
		t.Fatalf("wasm-tools validate: %v\n%s", err, b)
	}

	back := http.NewServeMux()
	back.HandleFunc("/hello", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-A", "b")
		fmt.Fprintf(w, "hi %s %s %s q=%s auth=%q", r.Method, r.Header.Get("X-Sent"), r.UserAgent(), r.URL.RawQuery, r.Header.Get("Authorization"))
	})
	back.HandleFunc("/echo", func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		fmt.Fprintf(w, "len %d cl %d sum %d", len(b), r.ContentLength, bytes.Count(b, []byte("z")))
	})
	back.HandleFunc("/redir", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/hello?from=redir", http.StatusFound)
	})
	back.HandleFunc("/gzip", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Accept-Encoding") != "gzip" {
			t.Errorf("/gzip: Accept-Encoding %q", r.Header.Get("Accept-Encoding"))
		}
		w.Header().Set("Content-Encoding", "gzip")
		z := gzip.NewWriter(w)
		io.WriteString(z, "unzipped")
		z.Close()
	})
	back.HandleFunc("/slow", func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-time.After(10 * time.Second):
		case <-r.Context().Done():
		}
	})
	back.HandleFunc("/big", func(w http.ResponseWriter, r *http.Request) { w.Write(bytes.Repeat([]byte("y"), 300000)) })
	bs := httptest.NewServer(back)
	defer bs.Close()
	// another host, by name, for a redirect that must drop Authorization
	other := strings.Replace(bs.URL, "127.0.0.1", "localhost", 1)
	back.HandleFunc("/away", func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, other+"/hello", http.StatusFound) })

	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := l.Addr().String()
	l.Close()
	l2, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	closed := "http://" + l2.Addr().String() + "/"
	l2.Close()
	// a server Spin's manifest does not list
	unlisted := httptest.NewServer(back)
	defer unlisted.Close()

	var logs bytes.Buffer
	srv := wasmHostCommand(t, host, comp, addr, []string{bs.URL, other, strings.TrimSuffix(closed, "/")})
	srv.Stdout, srv.Stderr = &logs, &logs
	// the lifeline stops the server's process group (spin up runs its
	// trigger as a child) when the test ends
	cut := testtmp.Lifeline(t, srv, 10*time.Minute)
	if err := srv.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		cut()
		srv.Wait()
	}()
	up := time.Now().Add(30 * time.Second)
	for {
		c, err := net.Dial("tcp", addr)
		if err == nil {
			c.Close()
			break
		}
		if time.Now().After(up) {
			t.Fatalf("%s did not listen\n%s", host, logs.String())
		}
		time.Sleep(10 * time.Millisecond)
	}
	client := &http.Client{Timeout: 30 * time.Second}
	type clientCase struct {
		name, path, to, method, body, timeout, auth string
		want                                        string
	}
	cases := []clientCase{
		{name: "get", path: "/call", to: bs.URL + "/hello?a=1", method: "GET", want: "status 200 x-a b ce - body hi GET by-xo Go-http-client/1.1 q=a=1 auth=\"\""},
		{name: "post", path: "/call", to: bs.URL + "/echo", method: "POST", body: strings.Repeat("z", 100000), want: "status 200 x-a - ce - body len 100000 cl 100000 sum 100000"},
		{name: "put", path: "/call", to: bs.URL + "/echo", method: "PUT", body: "zz", want: "status 200 x-a - ce - body len 2 cl 2 sum 2"},
		{name: "not found", path: "/call", to: bs.URL + "/nope", method: "GET", want: "status 404 x-a - ce - body 404 page not found\n"},
		{name: "redirect", path: "/call", to: bs.URL + "/redir", method: "GET", want: "status 200 x-a b ce - body hi GET by-xo Go-http-client/1.1 q=from=redir auth=\"\""},
		{name: "redirect keeps auth", path: "/call", to: bs.URL + "/redir", method: "GET", auth: "secret", want: "status 200 x-a b ce - body hi GET by-xo Go-http-client/1.1 q=from=redir auth=\"secret\""},
		{name: "redirect to another host drops auth", path: "/call", to: bs.URL + "/away", method: "GET", auth: "secret", want: "status 200 x-a b ce - body hi GET by-xo Go-http-client/1.1 q= auth=\"\""},
		{name: "gzip", path: "/call", to: bs.URL + "/gzip", method: "GET", want: "status 200 x-a - ce - body unzipped"},
		{name: "refused", path: "/call", to: closed, method: "GET", want: "connection refused"},
		{name: "timeout", path: "/call", to: bs.URL + "/slow", method: "GET", timeout: "200", want: "err request timed out"},
		{name: "open", path: "/open", to: bs.URL + "/big", want: "status 200 read 300000"},
		{name: "with_roots", path: "/roots", want: "err a wasi:http component's host does TLS: with_roots has no place to go"},
		{name: "piped body", path: "/piped", to: bs.URL + "/echo", body: "hello", want: "status 200 body len 5 cl 5 sum 0"},
	}
	if host == "spin" {
		cases = append(cases, clientCase{name: "unlisted origin", path: "/call", to: unlisted.URL + "/hello", method: "GET", want: "HTTP request denied"})
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			req, err := http.NewRequest("POST", "http://"+addr+c.path, strings.NewReader(c.body))
			if err != nil {
				t.Fatal(err)
			}
			req.Header.Set("X-To", c.to)
			if c.method != "" {
				req.Header.Set("X-Method", c.method)
			}
			if c.timeout != "" {
				req.Header.Set("X-Timeout-Ms", c.timeout)
			}
			if c.auth != "" {
				req.Header.Set("X-Auth", c.auth)
			}
			resp, err := client.Do(req)
			if err != nil {
				t.Fatalf("%v\n%s", err, logs.String())
			}
			defer resp.Body.Close()
			b, _ := io.ReadAll(resp.Body)
			got := string(b)
			if c.want == "connection refused" || c.want == "HTTP request denied" {
				t.Logf("%s under %s: %q", c.name, host, got)
				if !strings.Contains(got, c.want) {
					t.Errorf("got %d %q, want an error naming %q\n%.2000s", resp.StatusCode, got, c.want, logs.String())
				}
			} else if got != c.want {
				t.Errorf("got %d %q, want %q\n%.2000s", resp.StatusCode, got, c.want, logs.String())
			}
		})
	}
}
