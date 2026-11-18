package stdtest

// A wire differential of std/grpc's client against grpc-go's (task #115):
// seeded responses, served by a raw HTTP/2 server (internal/stdtest/grpcref
// cannedserve), to a stream call that each client makes and reads to its
// end. A response is an HTTP status (mostly 200), a content-type (gRPC's
// and its +proto and ;params forms, others, none), a grpc-encoding
// (identity, gzip, an unknown one, none), a body of gRPC frames
// (uncompressed and gzip compressed messages, empty ones, a flag byte
// other than 0 and 1, a declared length past the body or past the 4 MiB
// limit, a body cut inside a frame's prefix or message, bytes after the
// last frame) cut into DATA frames of seeded sizes, and trailers
// (grpc-status of every code, out of range or not a number, or none;
// grpc-message percent encoded, raw, malformed), or a trailers-only
// response. Both clients print each message and the final status code; the
// status message is compared when the status came from the server's
// grpc-status (the clients' own messages for a failure they detect are
// their own wording).
//
//   XO_GRPCFUZZ_SEED (2), _N (2000), _BACKENDS (arm64,llvm), _SHOW (25: the differing calls printed)
//   XO_GRPCFUZZ_KEEP=DIR keeps the cases (DIR/cases.jsonl, line K is call K)

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/xo-lang/xo/internal/testtmp"
	"github.com/xo-lang/xo/internal/xotool"
)

type grfCase struct {
	H     [][2]string `json:"h"`
	Body  string      `json:"body"`
	Split []int       `json:"split"`
	T     [][2]string `json:"t"`
	// the grpc-status the server sent (-1 for none or not a number) and its decoded message
	status int
	msg    string
}

func grfGzip(b []byte) []byte {
	var o bytes.Buffer
	w := gzip.NewWriter(&o)
	w.Write(b)
	w.Close()
	return o.Bytes()
}

func grfFrame(flag byte, payload []byte, declared int) []byte {
	b := []byte{flag, 0, 0, 0, 0}
	binary.BigEndian.PutUint32(b[1:], uint32(declared))
	return append(b, payload...)
}

func grfPayload(r *rand.Rand) []byte {
	switch r.Intn(6) {
	case 0:
		return nil
	case 1:
		return bytes.Repeat([]byte{byte(r.Intn(256))}, r.Intn(5000))
	default:
		b := make([]byte, r.Intn(64))
		r.Read(b)
		return b
	}
}

func grfMessage(r *rand.Rand) (sent, decoded string) {
	switch r.Intn(6) {
	case 0:
		return "", ""
	case 1:
		return "plain text", "plain text"
	case 2:
		return "a%20b%0Ac%25", "a b\nc%"
	case 3:
		// malformed escapes: grpc-go keeps them as they are
		s := []string{"%", "%4", "%zz", "a%2", "%%41", "%E9", "%C3%A9", "%C3"}[r.Intn(8)]
		return s, ""
	case 4:
		return "é raw", ""
	default:
		var b strings.Builder
		for i := r.Intn(10); i >= 0; i-- {
			if r.Intn(3) == 0 {
				fmt.Fprintf(&b, "%%%02X", r.Intn(256))
			} else {
				b.WriteByte("abcXYZ 09~"[r.Intn(10)])
			}
		}
		return b.String(), ""
	}
}

func grfGen(r *rand.Rand) grfCase {
	var c grfCase
	c.status = -1
	st := "200"
	if r.Intn(12) == 0 {
		st = []string{"404", "500", "503", "429", "401", "403", "502", "504", "400", "302"}[r.Intn(10)]
	}
	c.H = append(c.H, [2]string{":status", st})
	switch r.Intn(14) {
	case 0:
	case 1:
		c.H = append(c.H, [2]string{"content-type", "text/html"})
	case 2:
		c.H = append(c.H, [2]string{"content-type", "application/grpc+proto"})
	case 3:
		c.H = append(c.H, [2]string{"content-type", "application/grpc; charset=utf-8"})
	case 4:
		c.H = append(c.H, [2]string{"content-type", "application/grpcx"})
	case 5:
		c.H = append(c.H, [2]string{"content-type", "Application/GRPC"})
	default:
		c.H = append(c.H, [2]string{"content-type", "application/grpc"})
	}
	enc := ""
	switch r.Intn(8) {
	case 0:
		enc = "gzip"
	case 1:
		enc = "identity"
	case 2:
		enc = "br"
	}
	if enc != "" {
		c.H = append(c.H, [2]string{"grpc-encoding", enc})
	}
	status := func() [][2]string {
		var t [][2]string
		switch r.Intn(10) {
		case 0:
		case 1:
			t = append(t, [2]string{"grpc-status", []string{"17", "99", "-1", "x", "", " 0", "00"}[r.Intn(7)]})
		case 2, 3, 4:
			n := r.Intn(17)
			t = append(t, [2]string{"grpc-status", strconv.Itoa(n)})
			c.status = n
		default:
			t = append(t, [2]string{"grpc-status", "0"})
			c.status = 0
		}
		if r.Intn(2) == 0 {
			sent, dec := grfMessage(r)
			t = append(t, [2]string{"grpc-message", sent})
			c.msg = dec
		}
		return t
	}
	if r.Intn(10) == 0 {
		// trailers-only
		c.H = append(c.H, status()...)
		return c
	}
	var body []byte
	for i, n := 0, r.Intn(5); i < n; i++ {
		p := grfPayload(r)
		switch r.Intn(14) {
		case 0:
			// compressed (with gzip negotiated or not)
			z := grfGzip(p)
			body = append(body, grfFrame(1, z, len(z))...)
		case 1:
			body = append(body, grfFrame(byte(2+r.Intn(254)), p, len(p))...)
		case 2:
			body = append(body, grfFrame(0, p, len(p)+1+r.Intn(100))...)
		case 3:
			body = append(body, grfFrame(0, p, 4194305+r.Intn(1000000))...)
		case 4:
			body = append(body, grfFrame(0, p, 4194304)...)
		case 5:
			body = append(body, []byte{0, 0, 0}...)
		case 6:
			// compressed flag but not gzip
			body = append(body, grfFrame(1, p, len(p))...)
		default:
			body = append(body, grfFrame(0, p, len(p))...)
		}
	}
	if r.Intn(15) == 0 && len(body) > 0 {
		body = body[:r.Intn(len(body))]
	}
	c.Body = hex.EncodeToString(body)
	for left := len(body); left > 0; {
		m := 1 + r.Intn(1+left)
		if r.Intn(3) == 0 {
			m = left
		}
		c.Split = append(c.Split, m)
		left -= m
	}
	c.T = status()
	if c.T == nil {
		c.T = [][2]string{}
	}
	return c
}

func grfBlocks(s string) []string {
	var bs []string
	for _, l := range strings.Split(s, "\n") {
		if strings.HasPrefix(l, "== ") {
			bs = append(bs, "")
			continue
		}
		if len(bs) > 0 && l != "" {
			bs[len(bs)-1] += l + "\n"
		}
	}
	return bs
}

// grfCompare: the messages and the code, and the status message when the
// status came from the server (Go's code is the case's grpc-status and Go
// reports the message the server sent).
// grfPartialHeader reports whether c's body (whole frames from the start)
// ends with 1 to 4 bytes of a frame header, and the stream has trailers.
func grfPartialHeader(c grfCase) bool {
	d, err := hex.DecodeString(c.Body)
	if err != nil || c.T == nil {
		return false
	}
	at := 0
	for at+5 <= len(d) {
		n := int(binary.BigEndian.Uint32(d[at+1 : at+5]))
		if n > len(d)-at-5 {
			return false
		}
		at += 5 + n
	}
	return at < len(d)
}

func grfCompare(c grfCase, got, want string) bool {
	// a grpc-status number past the 17 codes: grpc-go keeps it as its code,
	// std/grpc's Code has no case for it and says Unknown (documented)
	var ws []string
	for _, l := range strings.Split(want, "\n") {
		if n, err := strconv.ParseUint(strings.TrimPrefix(l, "s "), 10, 32); err == nil && strings.HasPrefix(l, "s ") && n > 16 {
			l = "s 2"
		}
		ws = append(ws, l)
	}
	want = strings.Join(ws, "\n")
	if got == want {
		return true
	}
	// a body that ends inside a message's 5 byte header: grpc-go's
	// Stream.ReadMessageHeader loses the bytes it read before the end and
	// reports a clean end, so the call takes the trailers' status; std/grpc
	// says Internal "unexpected EOF" (documented in std/changes/grpc.md)
	if grfPartialHeader(c) {
		eof := "s 13\nx " + hex.EncodeToString([]byte("unexpected EOF"))
		gl, wl := strings.TrimSuffix(got, "\n"), strings.TrimSuffix(want, "\n")
		if strings.HasSuffix(gl, eof) {
			gm := strings.TrimSuffix(gl, eof)
			if i := strings.LastIndex(wl, "s "); i >= 0 && gm == wl[:i] {
				return true
			}
		}
	}
	strip := func(s string) (string, string) {
		var keep, x []string
		for _, l := range strings.Split(strings.TrimSuffix(s, "\n"), "\n") {
			if strings.HasPrefix(l, "x ") || l == "x" {
				x = append(x, l)
			} else {
				keep = append(keep, l)
			}
		}
		return strings.Join(keep, "\n"), strings.Join(x, "\n")
	}
	g, gx := strip(got)
	w, wx := strip(want)
	if g != w {
		return false
	}
	fromServer := c.status > 0 && strings.Contains(want, fmt.Sprintf("s %d\n", c.status)) && strings.TrimSpace(strings.TrimPrefix(wx, "x")) == hex.EncodeToString([]byte(c.msg))
	if fromServer {
		return gx == wx
	}
	return true
}

func TestGrpcFuzz(t *testing.T) {
	if testing.Short() {
		t.Skip("builds an Xo program and the grpc-go oracle")
	}
	seed := int64(envInt(t, "XO_GRPCFUZZ_SEED", 2))
	n := envInt(t, "XO_GRPCFUZZ_N", 2000)
	backends := envBackends("XO_GRPCFUZZ_BACKENDS", []string{"arm64", "llvm"})
	r := rand.New(rand.NewSource(seed))
	xo, err := xotool.Path()
	if err != nil {
		t.Fatal(err)
	}
	root, err := xotool.Root()
	if err != nil {
		t.Fatal(err)
	}
	dir, err := testtmp.Dir("xo-grpcfuzz-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)
	cases := make([]grfCase, n)
	var file bytes.Buffer
	for i := range cases {
		cases[i] = grfGen(r)
		j, _ := json.Marshal(cases[i])
		file.Write(j)
		file.WriteByte('\n')
	}
	cf := filepath.Join(dir, "cases.jsonl")
	if err := os.WriteFile(cf, file.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	if d := os.Getenv("XO_GRPCFUZZ_KEEP"); d != "" {
		// the cases, one JSON line each (grpcref cannedserve's input), to replay a call alone
		if err := os.WriteFile(filepath.Join(d, "cases.jsonl"), file.Bytes(), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	ref := filepath.Join(dir, "grpcref")
	b := exec.Command("go", "build", "-o", ref, ".")
	b.Dir = filepath.Join(root, "internal", "stdtest", "grpcref")
	b.Env = append(os.Environ(), "GOFLAGS=-mod=mod", "GOPROXY=off")
	if out, err := b.CombinedOutput(); err != nil {
		t.Skipf("the grpc-go oracle cannot be built here (as TestGrpcInterop): %v\n%s", err, out)
	}
	srv := testtmp.Guard(exec.Command(ref, "cannedserve", cf))
	so, err := srv.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := srv.Start(); err != nil {
		t.Fatal(err)
	}
	defer srv.Process.Kill()
	addr, err := bufio.NewReader(so).ReadString('\n')
	if err != nil {
		t.Fatal(err)
	}
	addr = strings.TrimSpace(addr)
	run := func(name string, cmd *exec.Cmd) []string {
		var out, errb bytes.Buffer
		cmd.Stdout = &out
		cmd.Stderr = &errb
		done := make(chan error, 1)
		if err := cmd.Start(); err != nil {
			t.Fatal(err)
		}
		go func() { done <- cmd.Wait() }()
		select {
		case err := <-done:
			if err != nil {
				t.Fatalf("%s: %v after %d of %d calls\n%s\nstdout:\n%s", name, err, len(grfBlocks(out.String())), n, myfTail(errb.String(), 20), myfTail(out.String(), 10))
			}
		case <-time.After(10 * time.Minute):
			cmd.Process.Kill()
			t.Fatalf("%s: no end after 10 minutes; its last case: %d", name, len(grfBlocks(out.String())))
		}
		bs := grfBlocks(out.String())
		if len(bs) != n {
			t.Fatalf("%s: %d results for %d calls", name, len(bs), n)
		}
		return bs
	}
	want := run("grpc-go", testtmp.Guard(exec.Command(ref, "cannedclient", addr, strconv.Itoa(n))))
	codes := map[string]int{}
	for _, w := range want {
		for _, l := range strings.Split(w, "\n") {
			if strings.HasPrefix(l, "s ") {
				codes[l]++
			}
		}
	}
	t.Logf("seed %d: %d calls; grpc-go's codes %v", seed, n, codes)
	for _, be := range backends {
		bin := filepath.Join(dir, "xo-grpc-"+be)
		xb := testtmp.Guard(exec.Command(xo, "build", "--backend="+be, "-o", bin, filepath.Join(root, "scripts", "grpcdiff", "xo")))
		xb.Env = append(os.Environ(), "XO_STD="+filepath.Join(root, "std"))
		if out, err := xb.CombinedOutput(); err != nil {
			t.Fatalf("xo build --backend=%s: %v\n%s", be, err, out)
		}
		got := run(be, testtmp.Guard(exec.Command(bin, addr, strconv.Itoa(n))))
		bad := 0
		for i := range want {
			if grfCompare(cases[i], got[i], want[i]) {
				continue
			}
			if bad < envInt(t, "XO_GRPCFUZZ_SHOW", 25) {
				j, _ := json.Marshal(cases[i])
				t.Errorf("%s case %d (seed %d): %s\ngot\n%swant\n%s", be, i, seed, myfTail(string(j), 1)[:min(len(j), 600)], got[i], want[i])
			}
			bad++
		}
		if bad > 0 {
			t.Fatalf("%s: %d of %d calls differ (seed %d)", be, bad, n, seed)
		}
		t.Logf("%s: %d calls equal grpc-go's (seed %d)", be, n, seed)
	}
}
