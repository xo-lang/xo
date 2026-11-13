package selfhost

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/xo-lang/xo/internal/testtmp"
	"github.com/xo-lang/xo/internal/xotool"
)

type goldenReq struct {
	method, path, body string
	hdr                [][2]string
}

func gr(method, path string) goldenReq { return goldenReq{method: method, path: path} }

const multipartCT = "multipart/form-data; boundary=BB"
const multipartBody = "--BB\r\nContent-Disposition: form-data; name=\"title\"\r\n\r\nhello\r\n" +
	"--BB\r\nContent-Disposition: form-data; name=\"tag\"\r\n\r\na\r\n" +
	"--BB\r\nContent-Disposition: form-data; name=\"f\"; filename=\"a.txt\"\r\nContent-Type: text/plain\r\n\r\nfile body\r\n--BB--\r\n"

func gzipReqs() []goldenReq {
	var rs []goldenReq
	for _, p := range []string{"/text", "/small", "/html", "/png", "/etag", "/weak", "/coded", "/partial", "/stream"} {
		for _, ae := range []string{"gzip", "identity", "gzip;q=0"} {
			rs = append(rs, goldenReq{method: "GET", path: p, hdr: [][2]string{{"Accept-Encoding", ae}}})
		}
	}
	return rs
}

// componentGoldens are the native goldens (internal/llvm/testdata/golden, or
// testdata/wasm_goldens for a golden that builds its router in main) whose
// router a component can serve: decision 0447's "goldens under wasmtime serve
// with a request script, beside the native goldens of the same programs". The
// golden's source with its main replaced runs both ways, and each request's
// answer must be the same.
var componentGoldens = []struct {
	name, routes string
	reqs         []goldenReq
}{
	{"http_server", "routes()", []goldenReq{
		gr("GET", "/hello/ann"), gr("GET", "/notes/7"), gr("GET", "/notes/8"), gr("GET", "/notes/x"), gr("GET", "/files/a/b/c"),
		gr("GET", "/search?q=x&page=2"), gr("GET", "/search"), gr("GET", "/broken"), gr("DELETE", "/notes/7"),
		{method: "POST", path: "/echo", body: "hi", hdr: [][2]string{{"Content-Type", "text/plain"}}},
		gr("GET", "/api/me"), {method: "GET", path: "/api/me", hdr: [][2]string{{"X-User", "ann"}}},
		{method: "POST", path: "/api/notes", body: `{"title":"t","tags":["x"]}`, hdr: [][2]string{{"X-User", "ann"}}},
		{method: "POST", path: "/api/notes", body: `{"title":""}`, hdr: [][2]string{{"X-User", "ann"}}},
		{method: "POST", path: "/api/notes", body: `not json`, hdr: [][2]string{{"X-User", "ann"}}},
		gr("GET", "/nope"), gr("PUT", "/hello/x"),
	}},
	{"http_form", "routes()", []goldenReq{
		{method: "POST", path: "/login", body: "user=ann+lee&tag=a&tag=b%26c", hdr: [][2]string{{"Content-Type", "application/x-www-form-urlencoded"}}},
		{method: "POST", path: "/login", body: "user=x"},
		{method: "POST", path: "/login", body: "user=%zz", hdr: [][2]string{{"Content-Type", "application/x-www-form-urlencoded"}}},
		gr("GET", "/search?q=1&q=two+words&q=%E2%82%AC"), gr("GET", "/search"),
	}},
	{"http_server_request", "routes()", []goldenReq{
		gr("GET", "/see?q=a%20b&x=1"), gr("GET", "/esc/a%20b%2B"), gr("GET", "/go"), gr("GET", "/missing"), gr("GET", "/oops"),
	}},
	{"http_header_fields", "routes()", []goldenReq{
		gr("GET", "/set"),
		{method: "GET", path: "/echo", hdr: [][2]string{{"X-Tag", "one"}, {"X-Tag", "two"}, {"Cookie", "a=1; b=2"}}},
	}},
	{"http_gzip_responses", "routes()", gzipReqs()},
	{"http_gzip", "routes()", []goldenReq{
		gr("GET", "/page"), gr("GET", "/always"), gr("GET", "/upper"), gr("GET", "/members"), gr("GET", "/corrupt"), gr("GET", "/truncated"), gr("GET", "/deflate"), gr("GET", "/redirect"),
	}},
	{"http_stream", "routes()", []goldenReq{
		gr("GET", "/small"), gr("GET", "/big"), gr("GET", "/pieces"), gr("GET", "/many"), gr("GET", "/flushfirst"), gr("GET", "/empty"),
	}},
	{"http_stream_body", "routes()", []goldenReq{
		{method: "POST", path: "/stream", body: strings.Repeat("abcdefg", 40)}, {method: "POST", path: "/stream"}, {method: "POST", path: "/all", body: strings.Repeat("z", 200000)},
		{method: "POST", path: "/ignore", body: "xx"}, {method: "POST", path: "/first", body: "hello"}, {method: "POST", path: "/first"},
		{method: "POST", path: "/try", body: "hello"}, {method: "POST", path: "/echo", body: "hello"},
	}},
	{"http_pipe", "routes()", []goldenReq{
		{method: "POST", path: "/sum", body: "1\n2\n3\n"}, {method: "POST", path: "/count", body: "abc"}, {method: "POST", path: "/early", body: "abc"},
	}},
	{"multipart_basic", "routes()", []goldenReq{
		{method: "POST", path: "/upload", body: multipartBody, hdr: [][2]string{{"Content-Type", multipartCT}}},
		{method: "POST", path: "/upload", body: "not multipart", hdr: [][2]string{{"Content-Type", "text/plain"}}},
	}},
	{"http_multipart_stream", "routes()", []goldenReq{
		{method: "POST", path: "/parts", body: multipartBody, hdr: [][2]string{{"Content-Type", multipartCT}}},
		{method: "POST", path: "/big", body: multipartBody, hdr: [][2]string{{"Content-Type", multipartCT}}},
		{method: "POST", path: "/skip", body: multipartBody, hdr: [][2]string{{"Content-Type", multipartCT}}},
	}},
	{"http_wire", "routes()", []goldenReq{
		gr("GET", "/hello"), {method: "POST", path: "/echo", body: "hi"}, gr("GET", "/big"), gr("GET", "/html"), gr("GET", "/none"), gr("GET", "/host?q=1"),
	}},
	{"http_client_stream", "routes(os)", []goldenReq{gr("GET", "/big"), gr("GET", "/chunked"), gr("GET", "/gzip"), gr("GET", "/redirect")}},
	{"http_client_policy", "routes(os)", []goldenReq{gr("GET", "/target"), gr("GET", "/moved"), gr("GET", "/hop2"), gr("GET", "/loop")}},
	{"http_server_limits", "routes()", []goldenReq{{method: "POST", path: "/echo", body: "hi"}, {method: "POST", path: "/tiny", body: "short"}, {method: "POST", path: "/tiny", body: strings.Repeat("x", 100)}}},
	{"http_fault", "routes(os.stdio)", []goldenReq{gr("GET", "/ok/1"), gr("GET", "/boom/1"), gr("GET", "/boom/9"), gr("GET", "/fault"), gr("GET", "/mw/fault"), gr("GET", "/ok/2")}},
	{"http_router_paths", "routes()", []goldenReq{
		gr("GET", "/"), gr("GET", "/users"), gr("GET", "/users/"), gr("GET", "//users//"), gr("GET", "/users/7"), gr("GET", "/users/07"), gr("GET", "/users/x"), gr("GET", "/users/me"),
		gr("GET", "/a/1/b/2"), gr("GET", "/a//b/2"), gr("GET", "/a/%41/b/%2F"), gr("GET", "/files/a/b//c/"), gr("GET", "/files/a%2Fb/c"), gr("GET", "/any/q"), gr("DELETE", "/any/q"),
		gr("GET", "/x//y"), gr("GET", "/x/y"), gr("GET", "/users/%37"), gr("GET", "/USERS"), gr("GET", "/users/-3"), gr("PATCH", "/users/7"), gr("POST", "/users/7"), gr("GET", "/users/99999999999999999999"),
	}},
	{"http_router_hooks", "routes()", []goldenReq{
		gr("GET", "/items/3"), gr("GET", "/nope"), gr("DELETE", "/items"), gr("POST", "/items/7"), gr("DELETE", "/any"), gr("GET", "/m/s"), gr("GET", "/m/none"), gr("POST", "/m/s"),
	}},
}

// The rest of internal/llvm/testdata/golden that uses a router, and why no
// component runs it (ADR 0447 has the same list): a program built on a client,
// a connection, TLS, HTTP/2 or a raw socket, which the wasi:http handler has
// no way to reach or leaves to the host.
var componentGoldensNotRun = map[string]string{
	"http_escaped_path":          "it listens on a socket of its own for new_request's client to call, which a component cannot; TestWasmComponentClient runs the client",
	"http_raw_path":              "it listens on a socket of its own for the client to call, which a component cannot; TestWasmComponentClient runs the client",
	"http_client":                "it listens on a socket of its own for the client to call, which a component cannot; TestWasmComponentClient runs the client",
	"http_client_body_reader":    "it listens on a socket of its own for the client to call, which a component cannot; TestWasmComponentClient runs the client",
	"http_client_proxy":          "it listens on sockets of its own (the proxy and the target) for the client to call, which a component cannot",
	"http_client_jar":            "it listens on a socket of its own for the client to call, which a component cannot",
	"http_reverse_proxy":         "it listens on a socket of its own for the client to call, which a component cannot; TestWasmComponentClient runs the client",
	"http_conn_fault":            "faults on a connection, which the host owns",
	"http_duplex":                "reads and writes a stream at once; handle_stream on the input-stream is not decided",
	"http_duplex_h2tls":          "http_duplex over HTTP/2 on TLS; a component serves neither",
	"http_ipv6_url":              "listens on [::1] and requests it; a component serves no sockets",
	"http_h2_config":             "HTTP/2 server settings, which the host owns",
	"http_continue_after_head":   "a raw connection to its own socket reads the interim and final heads, which a component cannot open",
	"http_h2_empty_continuation": "raw HTTP/2 frames to its own socket, which a component cannot open",
	"http_h2_body_budget":        "an HTTP/2 client and server on one connection (the server's body budget), which a component cannot open",
	"http_request_guards":        "raw connections to its own socket (request smuggling and header guards of the native server and client), which a component cannot open",
	"http_h2_max_header_bytes":   "HTTP/2 frames on the wire, which the host owns",
	"http_h2c":                   "HTTP/2 on the wire, which the host owns",
	"http_h2c_sniff":             "HTTP/2 on the wire, which the host owns",
	"http_handler_fault":         "its router only faults; http_fault covers that",
	"http_reverse_proxy_ws":      "needs the client and an upgrade",
	"http_stream_body_h2":        "HTTP/2 on the wire, which the host owns; http_stream_body covers the routes",
	"http_tls":                   "TLS, which the host owns",
	"netconn_deadline":           "TLS and connection deadlines, which the host owns; it dials its own listener as a client",
	"http_reverse_proxy_retry":   "the target closes a kept connection to force a retry on a fresh one, which the host's outgoing-handler owns",
	"http_proxy_body_next_race":  "a raw socket client pipelines a second request behind a streamed body on one kept connection, which the host's incoming-handler owns",
	"http_tls_keys":              "TLS, which the host owns",
	"http_tls_state":             "TLS, which the host owns",
	"otlp_export":                "an in process collector on a listener, which a wasi component does not have",
	"tls_verify_conn":            "TLS, which the host owns",
	"http_trailers":              "trailers are not decided",
	"metrics_text":               "its router is in main, over registries it prints; its handlers' effects are stdout's",
	"net_only":                   "raw sockets",
	"tls_client":                 "TLS, which the host owns",
	"tls_coalesced_finish":       "TLS, which the host owns",
	"tls_deadline":               "TLS, which the host owns",
	"tls_mtls":                   "TLS, which the host owns",
	"tls_peer_certs":             "TLS, which the host owns",
	"trace_ring":                 "its router is in main, and its answers carry random ids",
	"http_stop_keepalive":        "its router is in main, and it tests the server's own connections at a stop, which the host owns",
	"websocket":                  "an upgrade, which wasi:http 0.2 has no way to take",
	"websocket_deadline":         "an upgrade, which wasi:http 0.2 has no way to take",
	"websocket_stop":             "an upgrade, which wasi:http 0.2 has no way to take",
	"websocket_tls":              "an upgrade, which wasi:http 0.2 has no way to take",
	"websocket_tls_deadline":     "an upgrade, which wasi:http 0.2 has no way to take",
	"amqp_tls_hostname":          "TLS, which the host owns, and an amqps dial over a socket",
}

var goldenMain = regexp.MustCompile(`(?ms)^fn main\(.*?^}\n`)
var goldenUse = regexp.MustCompile(`(?m)^use std/[a-z0-9_/]+\n`)

// goldenProgram writes the golden's source with main replaced to a new directory.
func goldenProgram(t *testing.T, src, main string) string {
	t.Helper()
	dir := t.TempDir()
	text := goldenMain.ReplaceAllString(src, "")
	// what only the golden's main used
	text = goldenUse.ReplaceAllStringFunc(text, func(use string) string {
		mod := use[strings.LastIndex(use, "/")+1 : len(use)-1]
		if strings.Contains(strings.Replace(text, use, "", 1), mod+".") {
			return use
		}
		return ""
	})
	text += "\n" + main
	if err := os.WriteFile(filepath.Join(dir, "main.xo"), []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "xo.mod"), []byte("module m\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

// goldenAnswer is what must match: the status, the body, and the header fields
// other than those of the framing or the time, which the host chooses.
func goldenAnswer(t *testing.T, client *http.Client, base string, r goldenReq) string {
	t.Helper()
	req, err := http.NewRequest(r.method, base+r.path, strings.NewReader(r.body))
	if err != nil {
		t.Fatal(err)
	}
	for _, h := range r.hdr {
		req.Header.Add(h[0], h[1])
	}
	resp, err := client.Do(req)
	if err != nil {
		return "error: " + err.Error()
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Sprintf("%d, the body failed: %v", resp.StatusCode, err)
	}
	// a component's stream goes out whole, so its flushes do not split the
	// compressed bytes: what must match is what they decode to
	if resp.Header.Get("Content-Encoding") == "gzip" {
		if z, err := gzip.NewReader(bytes.NewReader(body)); err == nil {
			if d, err := io.ReadAll(z); err == nil {
				body = d
			}
		}
	}
	var fields []string
	for k, vs := range resp.Header {
		if k == "Date" || k == "Connection" || k == "Transfer-Encoding" || k == "Content-Length" {
			continue
		}
		fields = append(fields, k+": "+strings.Join(vs, " | "))
	}
	sort.Strings(fields)
	return fmt.Sprintf("%d\n%s\n%q", resp.StatusCode, strings.Join(fields, "\n"), body)
}

func waitListening(t *testing.T, addr string, logs func() string) {
	t.Helper()
	up := time.Now().Add(30 * time.Second)
	for {
		c, err := net.Dial("tcp", addr)
		if err == nil {
			c.Close()
			return
		}
		if time.Now().After(up) {
			t.Fatalf("%s did not listen\n%s", addr, logs())
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// TestWasmComponentGoldens runs each of componentGoldens natively and as a
// component under `wasmtime serve` (after `wasm-tools validate`, which decision
// 0447 requires of every component golden) and compares the answers. It skips
// where the tools are not installed; the linux/arm64 offload hosts have them.
func TestWasmComponentGoldens(t *testing.T) {
	if testing.Short() {
		t.Skip("builds programs")
	}
	for _, tool := range []string{"wasm-tools", "wasmtime"} {
		if _, err := exec.LookPath(tool); err != nil {
			t.Skipf("%s not found", tool)
		}
	}
	xo, err := xotool.Path()
	if err != nil {
		t.Fatal(err)
	}
	root := repoRoot(t)
	env := append(os.Environ(), "XO_STD="+filepath.Join(root, "std"), "XO_BIN_CACHE=off")
	client := &http.Client{
		Timeout:   30 * time.Second,
		Transport: &http.Transport{DisableCompression: true},
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	for _, g := range componentGoldens {
		t.Run(g.name, func(t *testing.T) {
			if why, ok := componentGoldensNotRun[g.name]; ok {
				t.Fatalf("both run and not run (%s)", why)
			}
			src, err := os.ReadFile(filepath.Join(root, "internal", "selfhost", "testdata", "wasm_goldens", g.name, "main.xo"))
			if err != nil {
				src, err = os.ReadFile(filepath.Join(root, "internal", "llvm", "testdata", "golden", g.name, "main.xo"))
			}
			if err != nil {
				t.Fatal(err)
			}
			// the component
			compDir := goldenProgram(t, string(src), "fn main(os: Os) {\n  http.serve_component("+g.routes+")\n}\n")
			comp := filepath.Join(t.TempDir(), "c.wasm")
			build := testtmp.Guard(exec.Command(xo, "build", "--targets=wasip2/wasm", "-o", comp, "."))
			build.Dir, build.Env = compDir, env
			if b, err := build.CombinedOutput(); err != nil {
				t.Fatalf("the component's build: %v\n%s", err, b)
			}
			if b, err := testtmp.Guard(exec.Command("wasm-tools", "validate", comp)).CombinedOutput(); err != nil {
				t.Fatalf("wasm-tools validate: %v\n%s", err, b)
			}
			// the native server, its address on its first line
			natDir := goldenProgram(t, string(src), "fn main(os: Os) {\n  let l = os.net.listen(\"127.0.0.1:0\").or_fault(\"listen\")\n  os.stdio.println(l.addr())\n  http.server(l, "+g.routes+").serve().ignore_err(\"serve\")\n}\n")
			bin := filepath.Join(t.TempDir(), "nat")
			build = testtmp.Guard(exec.Command(xo, "build", "-o", bin, "."))
			build.Dir, build.Env = natDir, env
			if b, err := build.CombinedOutput(); err != nil {
				t.Fatalf("the native build: %v\n%s", err, b)
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			nat := exec.CommandContext(ctx, bin)
			natOut, err := nat.StdoutPipe()
			if err != nil {
				t.Fatal(err)
			}
			if err := nat.Start(); err != nil {
				t.Fatal(err)
			}
			defer nat.Wait()
			defer cancel()
			line, err := bufio.NewReader(natOut).ReadString('\n')
			if err != nil {
				t.Fatalf("the native server's address: %v", err)
			}
			natBase := "http://" + strings.TrimSpace(line)
			l, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			addr := l.Addr().String()
			l.Close()
			var logs strings.Builder
			srv := exec.CommandContext(ctx, "wasmtime", "serve", "--addr", addr, comp)
			srv.Stdout, srv.Stderr = &logs, &logs
			if err := srv.Start(); err != nil {
				t.Fatal(err)
			}
			defer srv.Wait()
			defer cancel()
			waitListening(t, addr, logs.String)
			for _, r := range g.reqs {
				want := goldenAnswer(t, client, natBase, r)
				got := goldenAnswer(t, client, "http://"+addr, r)
				if got != want {
					w, h := want, got
					if len(w) > 600 {
						w = w[:600] + "..."
					}
					if len(h) > 600 {
						h = h[:600] + "..."
					}
					t.Errorf("%s %s\n--- component\n%s\n--- native\n%s", r.method, r.path, h, w)
				}
			}
		})
	}
}

// TestWasmComponentGoldensListed keeps the two lists honest: every golden with
// a router is run or has a reason.
func TestWasmComponentGoldensListed(t *testing.T) {
	root := repoRoot(t)
	run := map[string]bool{}
	for _, g := range componentGoldens {
		run[g.name] = true
	}
	mains, err := filepath.Glob(filepath.Join(root, "internal", "llvm", "testdata", "golden", "*", "main.xo"))
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range mains {
		b, err := os.ReadFile(m)
		if err != nil {
			t.Fatal(err)
		}
		name := filepath.Base(filepath.Dir(m))
		if strings.Contains(string(b), "http.router()") && !run[name] && componentGoldensNotRun[name] == "" {
			t.Errorf("golden %s has a router: run it in TestWasmComponentGoldens or say why not", name)
		}
	}
}
