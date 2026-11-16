package stdtest

// A differential of std/websocket's client against gorilla/websocket
// v1.5.3's on seeded frames from a server it does not trust (task #90,
// std Proven evidence for websocket: decision 0461 asks a harness of
// every parser of untrusted input).
//
// A fake server in this test (plain TCP, the opening handshake by hand)
// answers a dial of /cN with case N's frames: 0 to 4 messages, text (valid
// UTF-8, or now and then not) or binary, of 0 bytes to a little past 64
// KiB, now and then just past std/websocket's max_message (1 MiB), each in
// 1 to 4 frames with pings and pongs between them; then a close frame
// (RFC 6455's valid and invalid codes, a reason, one byte of payload, or
// none) or the end of the stream. A third of the cases
// are changed after they are made: a reserved bit set, an unknown opcode,
// a masked frame, a continuation first, a new message inside a fragmented
// one, a control frame past 125 bytes or fragmented, a length in a longer
// form than it needs or with its top bit set, a frame cut short, or a byte
// changed.
//
// Both clients are scripts/wsdiff's. They read messages until an error
// and must print the same lines (std/websocket displays its errors as
// gorilla's), but for wsfUTF8Rule. XO_WSFUZZ_KEEP=DIR keeps both outputs.
//
// XO_WSFUZZ_SEED (default 2), XO_WSFUZZ_N (default 300) and
// XO_WSFUZZ_BACKENDS (default arm64,llvm). The reference builds from the
// Go module cache (GOFLAGS=-mod=mod, GOPROXY=off): gorilla/websocket
// v1.5.3 must be in it; the test fails (not skips) without it.

import (
	"bufio"
	"bytes"
	"crypto/sha1"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"io"
	"math/rand"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/xo-lang/xo/internal/testtmp"
	"github.com/xo-lang/xo/internal/xotool"
)

type wsfCase struct {
	frames  [][]byte
	note    string // the change made, "" for a case left as made
	badText bool   // a text message's payload is not UTF-8
}

// wsfFrame is one frame: FIN, the reserved bits (0 to 7), the opcode, a
// mask or none, and the length in its minimal form unless form says 16 or
// 64 bits.
func wsfFrame(fin bool, rsv, op byte, mask []byte, payload []byte, form int) []byte {
	b0 := op | rsv<<4
	if fin {
		b0 |= 0x80
	}
	f := []byte{b0}
	var mbit byte
	if mask != nil {
		mbit = 0x80
	}
	n := len(payload)
	switch {
	case form == 64 || (form == 0 && n > 65535):
		f = append(f, mbit|127)
		f = binary.BigEndian.AppendUint64(f, uint64(n))
	case form == 16 || (form == 0 && n > 125):
		f = append(f, mbit|126)
		f = binary.BigEndian.AppendUint16(f, uint16(n))
	default:
		f = append(f, mbit|byte(n))
	}
	if mask != nil {
		f = append(f, mask...)
		p := make([]byte, n)
		for i := range payload {
			p[i] = payload[i] ^ mask[i%4]
		}
		return append(f, p...)
	}
	return append(f, payload...)
}

var wsfText = []string{"", "hello", "héllo wörld", "日本語のテキスト", "emoji 😀 end", "tab\tand\nnewline", "{\"a\":1}"}

func wsfPayload(r *rand.Rand, text bool) []byte {
	var n int
	switch r.Intn(10) {
	case 0:
		n = 0
	case 1:
		n = 125
	case 2:
		n = 126
	case 3:
		n = 65535 + r.Intn(3)
	case 4:
		if r.Intn(8) == 0 {
			n = 1048576 + r.Intn(2) // at and just past max_message
		} else {
			n = r.Intn(4000)
		}
	default:
		n = r.Intn(300)
	}
	if text {
		var b []byte
		for len(b) < n {
			b = append(b, wsfText[r.Intn(len(wsfText))]...)
			b = append(b, ' ')
		}
		b = b[:n]
		// the cut at n can split a rune: mostly the split rune goes, now
		// and then it stays (text that is not UTF-8)
		if r.Intn(10) != 0 {
			for !utf8.Valid(b) {
				b = b[:len(b)-1]
			}
		}
		if r.Intn(10) == 0 && len(b) > 0 {
			b[r.Intn(len(b))] = 0xFF // not UTF-8
		}
		return b
	}
	b := make([]byte, n)
	r.Read(b)
	return b
}

var wsfCodes = []int{1000, 1001, 1002, 1003, 1007, 1008, 1009, 1011, 3000, 4999, 999, 1004, 1005, 1006, 1015, 1016, 2999, 5000}

func wsfClose(r *rand.Rand) []byte {
	switch r.Intn(6) {
	case 0:
		return nil
	case 1:
		return []byte{3} // one byte: no code
	}
	p := binary.BigEndian.AppendUint16(nil, uint16(wsfCodes[r.Intn(len(wsfCodes))]))
	switch r.Intn(4) {
	case 0:
		p = append(p, "bye"...)
	case 1:
		p = append(p, "bad \xff reason"...)
	case 2:
		p = append(p, strings.Repeat("r", 123)...)
	}
	return p
}

func wsfGen(r *rand.Rand) wsfCase {
	var c wsfCase
	for k := r.Intn(5); k > 0; k-- {
		text := r.Intn(2) == 0
		p := wsfPayload(r, text)
		if text && !utf8.Valid(p) {
			c.badText = true
		}
		parts := 1 + r.Intn(4)
		if len(p) < parts {
			parts = 1
		}
		op := byte(2)
		if text {
			op = 1
		}
		at := 0
		for i := 0; i < parts; i++ {
			end := len(p)
			if i < parts-1 {
				end = at + r.Intn(len(p)-at+1)
			}
			if i > 0 {
				op = 0
			}
			c.frames = append(c.frames, wsfFrame(i == parts-1, 0, op, nil, p[at:end], 0))
			at = end
			if r.Intn(6) == 0 {
				ping := make([]byte, r.Intn(126))
				r.Read(ping)
				c.frames = append(c.frames, wsfFrame(true, 0, pgfPick(r, byte(9), byte(10)), nil, ping, 0))
			}
		}
	}
	if r.Intn(5) != 0 {
		c.frames = append(c.frames, wsfFrame(true, 0, 8, nil, wsfClose(r), 0))
	}
	if r.Intn(3) == 0 && len(c.frames) > 0 {
		wsfChange(r, &c)
	}
	return c
}

// wsfChange changes a made case as a hostile server could.
func wsfChange(r *rand.Rand, c *wsfCase) {
	i := r.Intn(len(c.frames))
	f := append([]byte(nil), c.frames[i]...)
	switch r.Intn(11) {
	case 0:
		f[0] |= byte(1+r.Intn(7)) << 4
		c.note = "rsv"
	case 1:
		f[0] = f[0]&0xF0 | pgfPick(r, byte(3), byte(7), byte(11), byte(15))
		c.note = "opcode"
	case 2:
		f = wsfFrame(true, 0, 2, []byte{1, 2, 3, 4}, []byte("masked"), 0)
		c.note = "masked"
	case 3:
		c.frames = append([][]byte{wsfFrame(true, 0, 0, nil, []byte("cont"), 0)}, c.frames...)
		c.note = "continuation first"
		return
	case 4:
		// a text start without FIN, then a new binary message
		c.frames = append(c.frames[:i:i], append([][]byte{wsfFrame(false, 0, 1, nil, []byte("part"), 0), wsfFrame(true, 0, 2, nil, []byte("new"), 0)}, c.frames[i:]...)...)
		c.note = "new message inside"
		return
	case 5:
		f = wsfFrame(true, 0, 9, nil, bytes.Repeat([]byte("p"), 126), 0)
		c.note = "long control"
	case 6:
		f = wsfFrame(false, 0, 9, nil, []byte("ping"), 0)
		c.note = "fragmented control"
	case 7:
		f = wsfFrame(true, 0, 2, nil, []byte("short"), pgfPick(r, 16, 64))
		c.note = "long length form"
	case 8:
		f = []byte{0x82, 127, 0x80, 0, 0, 0, 0, 0, 0, 5, 'x', 'x', 'x', 'x', 'x'}
		c.note = "length top bit"
	case 9:
		c.frames = append(c.frames[:i:i], f[:r.Intn(len(f))])
		c.note = "cut"
		return
	default:
		f[r.Intn(len(f))] ^= byte(1 << r.Intn(8))
		c.note = "byte"
	}
	c.frames[i] = f
}

const wsfGUID = "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"

func wsfServer(l net.Listener, cases []wsfCase) {
	for {
		c, err := l.Accept()
		if err != nil {
			return
		}
		go wsfConn(c, cases)
	}
}

func wsfConn(c net.Conn, cases []wsfCase) {
	defer c.Close()
	c.SetDeadline(time.Now().Add(60 * time.Second))
	br := bufio.NewReader(c)
	line, err := br.ReadString('\n')
	if err != nil {
		return
	}
	parts := strings.Fields(line)
	if len(parts) < 2 {
		return
	}
	key := ""
	for {
		h, err := br.ReadString('\n')
		if err != nil {
			return
		}
		h = strings.TrimRight(h, "\r\n")
		if h == "" {
			break
		}
		if k, v, ok := strings.Cut(h, ":"); ok && strings.EqualFold(strings.TrimSpace(k), "Sec-WebSocket-Key") {
			key = strings.TrimSpace(v)
		}
	}
	i, err := strconv.Atoi(strings.TrimPrefix(parts[1], "/c"))
	if err != nil || i < 0 || i >= len(cases) {
		c.Write([]byte("HTTP/1.1 404 Not Found\r\nContent-Length: 0\r\n\r\n"))
		return
	}
	sum := sha1.Sum([]byte(key + wsfGUID))
	var w bytes.Buffer
	fmt.Fprintf(&w, "HTTP/1.1 101 Switching Protocols\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Accept: %s\r\n\r\n", base64.StdEncoding.EncodeToString(sum[:]))
	for _, f := range cases[i].frames {
		w.Write(f)
	}
	// the client's replies (pongs, its close) are drained so that a reset
	// never cuts what was sent
	go io.Copy(io.Discard, br)
	if _, err := c.Write(w.Bytes()); err != nil {
		return
	}
	if tc, ok := c.(*net.TCPConn); ok {
		tc.CloseWrite()
	}
	time.Sleep(2 * time.Second)
}

func TestWsFuzz(t *testing.T) {
	if testing.Short() {
		t.Skip("the websocket fuzz builds gorilla and the Xo client")
	}
	seed := int64(envInt(t, "XO_WSFUZZ_SEED", 2))
	n := envInt(t, "XO_WSFUZZ_N", 300)
	backends := envBackends("XO_WSFUZZ_BACKENDS", []string{"arm64", "llvm"})
	r := rand.New(rand.NewSource(seed))
	cases := make([]wsfCase, n)
	for i := range cases {
		cases[i] = wsfGen(r)
	}
	if d := os.Getenv("XO_WSFUZZ_DUMP"); d != "" {
		// one case's frames as hex, to replay it alone
		i, _ := strconv.Atoi(d)
		for _, f := range cases[i].frames {
			if len(f) > 64 {
				fmt.Printf("%x... (%d bytes)\n", f[:64], len(f))
			} else {
				fmt.Printf("%x\n", f)
			}
		}
		t.Logf("case %d: %s", i, cases[i].note)
		return
	}
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	go wsfServer(l, cases)
	root, err := xotool.Root()
	if err != nil {
		t.Fatal(err)
	}
	xo, err := xotool.Path()
	if err != nil {
		t.Fatal(err)
	}
	work, err := testtmp.Dir("xo-wsfuzz-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(work)
	ref := filepath.Join(work, "gorilla")
	gb := exec.Command("go", "build", "-o", ref, ".")
	gb.Dir = filepath.Join(root, "scripts", "wsdiff", "gorilla")
	gb.Env = append(os.Environ(), "GOFLAGS=-mod=mod", "GOPROXY=off", "GOWORK=off")
	if out, err := gb.CombinedOutput(); err != nil {
		t.Fatalf("building the reference (github.com/gorilla/websocket v1.5.3 must be in the module cache): %v\n%s", err, out)
	}
	var in strings.Builder
	for i := range cases {
		fmt.Fprintf(&in, "c%d\n", i)
	}
	base := "ws://" + l.Addr().String()
	run := func(bin string) string {
		cmd := testtmp.Guard(exec.Command(bin, base))
		cmd.Stdin = strings.NewReader(in.String())
		var out, errb bytes.Buffer
		cmd.Stdout = &out
		cmd.Stderr = &errb
		if err := cmd.Run(); err != nil {
			t.Fatalf("%s: %v\n%s\nstderr:\n%s", filepath.Base(bin), err, myfTail(out.String(), 20), myfTail(errb.String(), 30))
		}
		if d := os.Getenv("XO_WSFUZZ_KEEP"); d != "" {
			os.WriteFile(filepath.Join(d, filepath.Base(bin)+".out"), out.Bytes(), 0o644)
		}
		return out.String()
	}
	want := wsfBlocks(run(ref))
	for i, w := range want {
		want[i] = wsfUTF8Rule(w)
	}
	if len(want) != n {
		t.Fatalf("the reference answered %d of %d cases", len(want), n)
	}
	for _, b := range backends {
		bin := filepath.Join(work, "xo-"+b)
		xb := testtmp.Guard(exec.Command(xo, "build", "--backend="+b, "-o", bin, filepath.Join(root, "scripts", "wsdiff", "xo")))
		xb.Env = append(os.Environ(), "XO_STD="+filepath.Join(root, "std"))
		if out, err := xb.CombinedOutput(); err != nil {
			t.Fatalf("xo build --backend=%s: %v\n%s", b, err, out)
		}
		got := wsfBlocks(run(bin))
		bad, early := 0, 0
		for i := 0; i < n; i++ {
			if got[i] == want[i] {
				continue
			}
			if cases[i].badText && wsfEarlyUTF8(got[i], want[i]) {
				early++
				continue
			}
			if bad < 10 {
				t.Errorf("%s case %d (seed %d, %s): got\n%swant\n%s", b, i, seed, cases[i].note, wsfShort(got[i]), wsfShort(want[i]))
			}
			bad++
		}
		if bad > 0 {
			t.Fatalf("%s: %d of %d cases differ (seed %d)", b, bad, n, seed)
		}
		t.Logf("%s: %d cases equal gorilla's (seed %d), %d of them with std/websocket's UTF-8 error before gorilla's", b, n, seed, early)
	}
}

// wsfUTF8Rule: gorilla/websocket returns a text message that is not
// UTF-8, where RFC 6455 section 8.1 fails the connection (close 1007) and
// std/websocket does (its Message doc); so the reference's lines stop at
// such a message with std/websocket's error.
func wsfUTF8Rule(block string) string {
	var b strings.Builder
	for _, l := range strings.SplitAfter(block, "\n") {
		if h, ok := strings.CutPrefix(strings.TrimSuffix(l, "\n"), "msg text "); ok {
			if d, err := hex.DecodeString(h); err == nil && !utf8.Valid(d) {
				b.WriteString(wsfUTF8Err)
				return b.String()
			}
		}
		b.WriteString(l)
	}
	return b.String()
}

const wsfUTF8Err = "error websocket: invalid UTF-8 in text frame\n"

// wsfEarlyUTF8: std/websocket checks a text message's UTF-8 in the pieces
// that arrive (Autobahn 6.4.3, 6.4.4), so in a case with text that is not
// UTF-8 its error can come before the error gorilla reaches; the messages
// before it must be gorilla's.
func wsfEarlyUTF8(got, want string) bool {
	head, ok := strings.CutSuffix(got, wsfUTF8Err)
	return ok && strings.HasPrefix(want, head) && !strings.Contains(head, "error ")
}

// wsfBlocks are the lines after each "== cN" line, by case.
func wsfBlocks(out string) map[int]string {
	m := map[int]string{}
	cur := -1
	var b strings.Builder
	for _, l := range strings.Split(out, "\n") {
		if name, ok := strings.CutPrefix(l, "== c"); ok {
			if cur >= 0 {
				m[cur] = b.String()
			}
			cur, _ = strconv.Atoi(name)
			b.Reset()
			continue
		}
		if cur >= 0 && l != "" {
			b.WriteString(l + "\n")
		}
	}
	if cur >= 0 {
		m[cur] = b.String()
	}
	return m
}

// wsfShort shortens long message lines for a failure's text.
func wsfShort(block string) string {
	var b strings.Builder
	for _, l := range strings.Split(strings.TrimSuffix(block, "\n"), "\n") {
		if len(l) > 160 {
			l = fmt.Sprintf("%s... (%d chars)", l[:160], len(l))
		}
		b.WriteString(l + "\n")
	}
	return b.String()
}

// TestWsLengthTopBit: a frame whose 64-bit length has its top bit set is
// std/websocket's "read limit exceeded", as gorilla's (a negative length
// to its setReadRemaining). It faulted (an Int overflow in the length's
// shift, found by TestWsFuzz seed 2 case 105). Needs no reference build.
func TestWsLengthTopBit(t *testing.T) {
	cases := []wsfCase{
		{frames: [][]byte{{0x82, 127, 0x80, 0, 0, 0, 0, 0, 0, 5, 'x', 'x', 'x', 'x', 'x'}}},
		{frames: [][]byte{{0x82, 127, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF}}},
		{frames: [][]byte{wsfFrame(true, 0, 2, nil, []byte("ok"), 64)}},
	}
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	go wsfServer(l, cases)
	root, err := xotool.Root()
	if err != nil {
		t.Fatal(err)
	}
	xo, err := xotool.Path()
	if err != nil {
		t.Fatal(err)
	}
	work, err := testtmp.Dir("xo-wstop-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(work)
	bin := filepath.Join(work, "xo-ws")
	xb := testtmp.Guard(exec.Command(xo, "build", "-o", bin, filepath.Join(root, "scripts", "wsdiff", "xo")))
	xb.Env = append(os.Environ(), "XO_STD="+filepath.Join(root, "std"))
	if out, err := xb.CombinedOutput(); err != nil {
		t.Fatalf("xo build: %v\n%s", err, out)
	}
	cmd := testtmp.Guard(exec.Command(bin, "ws://"+l.Addr().String()))
	cmd.Stdin = strings.NewReader("c0\nc1\nc2\n")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("xo client: %v\n%s", err, out)
	}
	got := wsfBlocks(string(out))
	for i := 0; i < 2; i++ {
		if got[i] != "error websocket: read limit exceeded\n" {
			t.Errorf("case %d: got %q, want read limit exceeded", i, got[i])
		}
	}
	if !strings.HasPrefix(got[2], "msg binary 6f6b\n") {
		t.Errorf("a short payload in the 64-bit form: got %q", got[2])
	}
}
