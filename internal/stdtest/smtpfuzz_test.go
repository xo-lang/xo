package stdtest

// TestSmtpFuzz (task #90): std/smtp's send against Go's net/smtp SendMail
// on seeded sessions with a fake SMTP server. Each case is a sender,
// recipients, a message (dots, bare CR and LF, no final newline, bytes
// that are not UTF-8) and an authentication (none, PLAIN or CRAM-MD5),
// and the server's replies to every command: the greeting, EHLO with
// extensions (8BITMIME, SMTPUTF8, AUTH and others, as servers write
// them) or refused (then HELO), the AUTH exchange (challenges in base64,
// well formed or not), MAIL, each RCPT, DATA and the end of the data,
// QUIT, each with codes that pass or fail the command, multi-line now
// and then. A quarter of the cases change one reply (LF alone, a short
// line, a code that is not one, a continuation with another code, bytes
// that are not UTF-8, quotes and controls in the text, a long line) or
// cut the connection partway through the replies. Both clients send in
// plain text to the loopback server (std/smtp's Security.None); the
// server keeps what each sends, and a case's output is that transcript
// and the send's result, which must be the same for both.
//
//   XO_SMTPFUZZ_SEED (default 2), XO_SMTPFUZZ_N (300),
//   XO_SMTPFUZZ_BACKENDS (arm64,llvm), XO_SMTPFUZZ_DUMP=i (one case's
//   replies and client lines, to replay it alone)

import (
	"bufio"
	"bytes"
	"encoding/base64"
	"fmt"
	"io"
	"math/rand"
	"net"
	"net/smtp"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/xo-lang/xo/internal/hostile"
	"github.com/xo-lang/xo/internal/testtmp"
	"github.com/xo-lang/xo/internal/xotool"
)

type smfCase struct {
	from       string
	to         []string
	auth       string // none, plain, cram
	user, pass string
	msg        []byte
	greet      []byte
	ehlo, helo []byte
	auths      [][]byte // the replies to AUTH and each response after it
	mail       []byte
	rcpt       [][]byte
	data, end  []byte
	quit       []byte
	cut        int // the bytes of replies the server writes before it closes; -1: all
	note       string
}

var smfWords = []string{"ok", "mail.test", "ESMTP", "go", "on", "2.1.0", "5.7.8", "queued", "as", "AB12", "bye", "welcome"}

func smfText(r *rand.Rand) string {
	n := 1 + r.Intn(4)
	w := make([]string, n)
	for i := range w {
		w[i] = smfWords[r.Intn(len(smfWords))]
	}
	switch r.Intn(12) {
	case 0:
		w = append(w, `"quoted"`)
	case 1:
		w = append(w, `back\slash`)
	case 2:
		w = append(w, "tab\there")
	case 3:
		w = append(w, "café ☃")
	case 4:
		return ""
	case 5:
		// the hostile dictionary (task #352): digit runs past Int and
		// Int's edges, as a reply's text (EHLO SIZE reads a number)
		w = append(w, hostile.Pick(r, hostile.Digits()), strconv.FormatInt(hostile.Pick(r, hostile.Ints()), 10))
	}
	return strings.Join(w, " ")
}

// smfReply: a reply of code with lines of text (one at least), CRLF.
func smfReply(code int, lines ...string) []byte {
	var b bytes.Buffer
	for i, l := range lines {
		sep := "-"
		if i == len(lines)-1 {
			sep = " "
		}
		fmt.Fprintf(&b, "%03d%s%s\r\n", code, sep, l)
	}
	return b.Bytes()
}

func smfSimple(r *rand.Rand, code int) []byte {
	if r.Intn(6) == 0 {
		lines := make([]string, 2+r.Intn(3))
		for i := range lines {
			lines[i] = smfText(r)
		}
		return smfReply(code, lines...)
	}
	return smfReply(code, smfText(r))
}

// smfPick: ok most of the time, else one of the others.
func smfPick(r *rand.Rand, ok int, others ...int) int {
	if r.Intn(8) != 0 || len(others) == 0 {
		return ok
	}
	return others[r.Intn(len(others))]
}

var smfExts = []string{"8BITMIME", "SMTPUTF8", "AUTH PLAIN CRAM-MD5", "AUTH CRAM-MD5", "AUTH PLAIN LOGIN", "AUTH", "SIZE 35882577", "PIPELINING", "8bitmime", "AUTH=PLAIN", "ENHANCEDSTATUSCODES", "8BITMIME extra words", "DSN"}

func smfAddr(r *rand.Rand, i int) string {
	switch r.Intn(40) {
	case 0:
		return fmt.Sprintf("u%d@x.test\r\nRCPT TO:<evil@x.test>", i)
	case 1:
		return fmt.Sprintf("u%d\n@x.test", i)
	case 2:
		return fmt.Sprintf("u%d@x.test\r", i)
	case 3, 4:
		return fmt.Sprintf("ü%d@x.test", i)
	case 5:
		return ""
	}
	return fmt.Sprintf("u%d.%d@x.test", i, r.Intn(100))
}

func smfMsg(r *rand.Rand) []byte {
	if r.Intn(20) == 0 {
		return nil
	}
	pieces := []string{"Subject: x", "", ".", "..", ".x", "body line", "\xff\xfe", "a\rb", "café", ". ", "From: a@x.test"}
	ends := []string{"\n", "\r\n", "\r\n", "\r"}
	var b bytes.Buffer
	n := 1 + r.Intn(8)
	for k := 0; k < n; k++ {
		b.WriteString(pieces[r.Intn(len(pieces))])
		if k < n-1 || r.Intn(4) != 0 {
			b.WriteString(ends[r.Intn(len(ends))])
		}
	}
	return b.Bytes()
}

func smfWord(r *rand.Rand) string {
	s := []string{"user", "a", "üser", "p@ss w", "secret", "x"}
	return s[r.Intn(len(s))] + strconv.Itoa(r.Intn(10))
}

// smfChallenge: a 334 reply to CRAM-MD5, its challenge as servers write
// it or not.
func smfChallenge(r *rand.Rand) []byte {
	ch := fmt.Sprintf("<%d.%d@mail.test>", r.Intn(100000), r.Intn(1000))
	e := base64.StdEncoding.EncodeToString([]byte(ch))
	switch r.Intn(10) {
	case 0:
		return smfReply(334, strings.TrimRight(e, "="))
	case 1:
		return smfReply(334, e[:len(e)/2]+"*"+e[len(e)/2:])
	case 2:
		return smfReply(334, e[:8], e[8:])
	case 3:
		return smfReply(334, " "+e)
	case 4:
		return smfReply(334, "")
	}
	return smfReply(334, e)
}

func smfGen(r *rand.Rand, i int) smfCase {
	c := smfCase{cut: -1}
	c.from = smfAddr(r, i)
	for k := r.Intn(4); k > 0; k-- {
		c.to = append(c.to, smfAddr(r, i*10+k))
	}
	switch r.Intn(5) {
	case 0:
		c.auth = "plain"
	case 1:
		c.auth = "cram"
	default:
		c.auth = "none"
	}
	c.user, c.pass = smfWord(r), smfWord(r)
	c.msg = smfMsg(r)
	c.greet = smfSimple(r, smfPick(r, 220, 554, 421, 250))
	if r.Intn(10) == 0 {
		c.ehlo = smfSimple(r, smfPick(r, 502, 500, 421))
	} else {
		lines := []string{"mail.test greets localhost"}
		for k := r.Intn(6); k > 0; k-- {
			if r.Intn(8) == 0 {
				// SIZE with a hostile number (task #352)
				lines = append(lines, "SIZE "+hostile.Pick(r, hostile.Digits()))
			} else {
				lines = append(lines, smfExts[r.Intn(len(smfExts))])
			}
		}
		if c.auth != "none" && r.Intn(4) != 0 {
			lines = append(lines, "AUTH PLAIN CRAM-MD5")
		}
		c.ehlo = smfReply(smfPick(r, 250, 550), lines...)
	}
	c.helo = smfSimple(r, smfPick(r, 250, 501))
	switch c.auth {
	case "plain":
		switch r.Intn(6) {
		case 0:
			c.auths = [][]byte{smfSimple(r, 535)}
		case 1:
			c.auths = [][]byte{smfReply(334, "")}
		case 2:
			c.auths = [][]byte{smfReply(334, "not base64!")}
		default:
			c.auths = [][]byte{smfSimple(r, 235)}
		}
	case "cram":
		c.auths = [][]byte{smfChallenge(r)}
		switch r.Intn(6) {
		case 0:
			c.auths = append(c.auths, smfSimple(r, 535))
		case 1:
			c.auths = append(c.auths, smfChallenge(r), smfSimple(r, 235))
		case 2:
			c.auths = [][]byte{smfSimple(r, smfPick(r, 504, 535, 235))}
		default:
			c.auths = append(c.auths, smfSimple(r, 235))
		}
	}
	c.mail = smfSimple(r, smfPick(r, 250, 550, 421, 251))
	for range c.to {
		c.rcpt = append(c.rcpt, smfSimple(r, smfPick(r, 250, 251, 252, 550, 452, 260)))
	}
	c.data = smfSimple(r, smfPick(r, 354, 554, 250, 451))
	c.end = smfSimple(r, smfPick(r, 250, 552, 554, 251))
	c.quit = smfSimple(r, smfPick(r, 221, 250, 500))
	if r.Intn(4) == 0 {
		smfChange(r, &c)
	}
	return c
}

// smfChange: one hostile edit of one reply of c, or a cut.
func smfChange(r *rand.Rand, c *smfCase) {
	refs := []*[]byte{&c.greet, &c.ehlo, &c.helo, &c.mail, &c.data, &c.end, &c.quit}
	for k := range c.auths {
		refs = append(refs, &c.auths[k])
	}
	for k := range c.rcpt {
		refs = append(refs, &c.rcpt[k])
	}
	// the replies a session reaches first are the ones most cases read
	p := refs[r.Intn(len(refs))]
	if r.Intn(2) == 0 {
		p = refs[[]int{0, 1, 3}[r.Intn(3)]]
	}
	b := *p
	first := string(b[:3])
	switch k := r.Intn(13); k {
	case 0:
		c.note = "lf"
		*p = bytes.ReplaceAll(b, []byte("\r\n"), []byte("\n"))
	case 1:
		c.note = "short"
		*p = []byte(first + "\r\n")
	case 2:
		c.note = "not a code"
		*p = append([]byte("x"+first[1:]+" text\r\n"), b...)
	case 3:
		c.note = "low code"
		*p = []byte("099 low\r\n")
	case 4:
		c.note = "other code"
		*p = append([]byte(first+"-first\r\n999-other\r\nnot a code line\r\n"), b...)
	case 5:
		c.note = "not utf8"
		*p = bytes.Replace(b, []byte("\r\n"), []byte(" \xff\xc3(\r\n"), 1)
	case 6:
		c.note = "controls"
		*p = bytes.Replace(b, []byte("\r\n"), []byte(" \x00\x07\x1b \"\\ é\r\n"), 1)
	case 7:
		c.note = "long"
		*p = bytes.Replace(b, []byte("\r\n"), []byte(" "+strings.Repeat("y", 3000)+"\r\n"), 1)
	case 8:
		// a continuation, then the server closes (the clients would wait
		// for the rest until the server's deadline)
		c.note = "cont end"
		*p = []byte(first + "-goes on\r\n" + smfClose)
	case 9:
		c.note = "lone cr"
		*p = bytes.Replace(b, []byte("\r\n"), []byte(" a\rb\r\n"), 1)
	case 10:
		c.note = "four digits"
		*p = []byte(first + "0 four\r\n")
	case 11:
		c.note = "blank"
		*p = append([]byte("\r\n"), b...)
	default:
		c.note = "cut"
		total := len(c.greet) + len(c.ehlo) + len(c.mail) + len(c.data) + len(c.end) + len(c.quit)
		for _, x := range c.rcpt {
			total += len(x)
		}
		c.cut = r.Intn(total)
	}
}

// smfLine: case i as a line for the Xo client (scripts/smtpdiff/xo).
func smfLine(addr string, c smfCase) string {
	h := func(s string) string { return fmt.Sprintf("x%x", s) }
	to := "-"
	if len(c.to) > 0 {
		var xs []string
		for _, x := range c.to {
			xs = append(xs, h(x))
		}
		to = strings.Join(xs, ",")
	}
	a := "none"
	if c.auth != "none" {
		a = fmt.Sprintf("%s:%s:%s", c.auth, h(c.user), h(c.pass))
	}
	return fmt.Sprintf("%s %s %s %s %s", addr, h(c.from), to, a, h(string(c.msg)))
}

// smfServer: a listener per case, each answering with its case's
// replies and keeping what the client sent.
type smfServer struct {
	addrs []string
	mu    sync.Mutex
	tr    map[int][]string
	wg    sync.WaitGroup
	ls    []net.Listener
}

func smfServe(t *testing.T, cases []smfCase) *smfServer {
	s := &smfServer{tr: map[int][]string{}}
	for i := range cases {
		l, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		s.ls = append(s.ls, l)
		s.addrs = append(s.addrs, l.Addr().String())
		go func(i int, l net.Listener) {
			for {
				conn, err := l.Accept()
				if err != nil {
					return
				}
				s.wg.Add(1)
				go func() {
					defer s.wg.Done()
					tr := smfSession(conn, cases[i])
					s.mu.Lock()
					s.tr[i] = append(s.tr[i], tr...)
					s.mu.Unlock()
				}()
			}
		}(i, l)
	}
	return s
}

func (s *smfServer) stop() {
	for _, l := range s.ls {
		l.Close()
	}
}

// take: the transcripts of the clients' sessions so far (once every
// session has ended), and a fresh start.
func (s *smfServer) take() map[int][]string {
	s.wg.Wait()
	s.mu.Lock()
	defer s.mu.Unlock()
	m := s.tr
	s.tr = map[int][]string{}
	return m
}

// smfHangUp closes conn as a server that is done: a FIN first, then what
// the client still sends is read and dropped until it closes. A plain
// Close with the client's next command unread sends a RST instead, and the
// client's read then fails with "connection reset" or reads EOF depending on
// whether that command arrived before the close (seed 2 case 1333 on llvm,
// a cut at the end of the EHLO reply: the client sent AUTH in time).
func smfHangUp(conn net.Conn) {
	if tc, ok := conn.(*net.TCPConn); ok {
		tc.CloseWrite()
		conn.SetReadDeadline(time.Now().Add(5 * time.Second))
		io.Copy(io.Discard, conn)
	}
	conn.Close()
}

// smfClose ends a reply after which the server closes the connection.
const smfClose = "\x00close"

func smfSession(conn net.Conn, c smfCase) []string {
	defer smfHangUp(conn)
	conn.SetDeadline(time.Now().Add(10 * time.Second))
	var tr []string
	left := c.cut
	send := func(b []byte) bool {
		if x, ok := bytes.CutSuffix(b, []byte(smfClose)); ok {
			conn.Write(x)
			tr = append(tr, "s closed")
			return false
		}
		if left >= 0 && len(b) >= left {
			conn.Write(b[:left])
			tr = append(tr, "s cut")
			return false
		}
		if left >= 0 {
			left -= len(b)
		}
		conn.Write(b)
		return true
	}
	if !send(c.greet) {
		return tr
	}
	br := bufio.NewReader(conn)
	ai, ri := 0, 0
	for {
		line, err := br.ReadString('\n')
		if line != "" {
			tr = append(tr, "c "+strconv.Quote(line))
		}
		if err != nil {
			return tr
		}
		verb := strings.ToUpper(strings.SplitN(strings.TrimRight(line, "\r\n"), " ", 2)[0])
		var reply []byte
		switch {
		case verb == "EHLO":
			reply = c.ehlo
		case verb == "HELO":
			reply = c.helo
		case verb == "*":
			reply = []byte("501 cancelled\r\n")
		case verb == "MAIL":
			reply = c.mail
		case verb == "RCPT":
			reply = []byte("250 ok\r\n")
			if ri < len(c.rcpt) {
				reply = c.rcpt[ri]
			}
			ri++
		case verb == "DATA":
			if !send(c.data) {
				return tr
			}
			if !bytes.HasPrefix(c.data, []byte("354")) {
				continue
			}
			var d bytes.Buffer
			for {
				l, err := br.ReadString('\n')
				d.WriteString(l)
				if err != nil || l == ".\r\n" {
					break
				}
			}
			tr = append(tr, "d "+strconv.Quote(d.String()))
			reply = c.end
		case verb == "QUIT":
			reply = c.quit
		case ai < len(c.auths):
			// AUTH, then each response to a challenge
			reply = c.auths[ai]
			ai++
		default:
			reply = []byte("500 unrecognized\r\n")
		}
		if !send(reply) {
			return tr
		}
	}
}

// smfGo: Go's net/smtp SendMail of case c, as the Xo client prints it.
func smfGo(addr string, c smfCase) string {
	var a smtp.Auth
	switch c.auth {
	case "plain":
		a = smtp.PlainAuth("", c.user, c.pass, "127.0.0.1")
	case "cram":
		a = smtp.CRAMMD5Auth(c.user, c.pass)
	}
	if err := smtp.SendMail(addr, a, c.from, c.to, c.msg); err != nil {
		return "error " + err.Error()
	}
	return "ok"
}

var smfNet = regexp.MustCompile(`(?i)(connection reset by peer|broken pipe|connection reset|reset by peer)`)

// smfNorm: a result, a connection's failure as the same text (the
// two report the system's error with other words around it).
func smfNorm(s string) string {
	if strings.HasPrefix(s, "error ") && smfNet.MatchString(s) {
		return "error connection closed by the server"
	}
	return s
}

func smfBlocks(out string, addrs []string) map[int]string {
	idx := map[string]int{}
	for i, a := range addrs {
		idx[a] = i
	}
	m := map[int]string{}
	cur := -1
	for _, l := range strings.Split(out, "\n") {
		if h, ok := strings.CutPrefix(l, "== "); ok {
			cur = -1
			if i, ok := idx[h]; ok {
				cur = i
			}
			continue
		}
		if cur >= 0 && l != "" {
			m[cur] = smfNorm(l)
		}
	}
	return m
}

func smfCompose(tr []string, result string) string {
	return strings.Join(tr, "\n") + "\n" + result + "\n"
}

func smfBuild(t *testing.T, work, backend string) string {
	root, err := xotool.Root()
	if err != nil {
		t.Fatal(err)
	}
	xo, err := xotool.Path()
	if err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(work, "xo-smtp-"+backend)
	args := []string{"build", "-o", bin, filepath.Join(root, "scripts", "smtpdiff", "xo")}
	if backend != "" {
		args = append(args[:1], append([]string{"--backend=" + backend}, args[1:]...)...)
	}
	xb := testtmp.Guard(exec.Command(xo, args...))
	xb.Env = append(os.Environ(), "XO_STD="+filepath.Join(root, "std"))
	if out, err := xb.CombinedOutput(); err != nil {
		t.Fatalf("xo build --backend=%s: %v\n%s", backend, err, out)
	}
	return bin
}

// smfRunXo: the Xo client's results of every case, and the server's
// transcripts of them.
func smfRunXo(t *testing.T, bin string, s *smfServer, cases []smfCase) (map[int]string, map[int][]string) {
	var lines []string
	for i, c := range cases {
		lines = append(lines, smfLine(s.addrs[i], c))
	}
	cmd := testtmp.Guard(exec.Command(bin))
	cmd.Stdin = strings.NewReader(strings.Join(lines, "\n") + "\n")
	var out, errb bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errb
	if err := cmd.Run(); err != nil {
		t.Fatalf("%s: %v\n%s\nstderr:\n%s", bin, err, myfTail(out.String(), 20), myfTail(errb.String(), 30))
	}
	return smfBlocks(out.String(), s.addrs), s.take()
}

func TestSmtpFuzz(t *testing.T) {
	if testing.Short() {
		t.Skip("the smtp fuzz builds the Xo client")
	}
	seed := int64(envInt(t, "XO_SMTPFUZZ_SEED", 2))
	n := envInt(t, "XO_SMTPFUZZ_N", 300)
	backends := envBackends("XO_SMTPFUZZ_BACKENDS", []string{"arm64", "llvm"})
	r := rand.New(rand.NewSource(seed))
	cases := make([]smfCase, n)
	for i := range cases {
		cases[i] = smfGen(r, i)
	}
	if d := os.Getenv("XO_SMTPFUZZ_DUMP"); d != "" {
		i, _ := strconv.Atoi(d)
		c := cases[i]
		fmt.Printf("note %q auth %s user %q pass %q cut %d\nfrom %q\nto %q\nmsg %q\n", c.note, c.auth, c.user, c.pass, c.cut, c.from, c.to, c.msg)
		for _, x := range []struct {
			n string
			b []byte
		}{{"greet", c.greet}, {"ehlo", c.ehlo}, {"helo", c.helo}, {"mail", c.mail}, {"data", c.data}, {"end", c.end}, {"quit", c.quit}} {
			fmt.Printf("%s %q\n", x.n, x.b)
		}
		fmt.Printf("auths %q\nrcpt %q\n", c.auths, c.rcpt)
		return
	}
	s := smfServe(t, cases)
	defer s.stop()
	want := map[int]string{}
	for i, c := range cases {
		res := smfNorm(smfGo(s.addrs[i], c))
		want[i] = res
	}
	wantTr := s.take()
	for i := range cases {
		want[i] = smfCompose(wantTr[i], want[i])
	}
	work, err := testtmp.Dir("xo-smtpfuzz-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(work)
	for _, b := range backends {
		bin := smfBuild(t, work, b)
		res, tr := smfRunXo(t, bin, s, cases)
		bad := 0
		for i := 0; i < n; i++ {
			got := smfCompose(tr[i], res[i])
			if got == want[i] {
				continue
			}
			if bad < 10 {
				t.Errorf("%s case %d (seed %d, %s, auth %s, cut %d):\ngot\n%swant\n%s", b, i, seed, cases[i].note, cases[i].auth, cases[i].cut, got, want[i])
			}
			bad++
		}
		if bad > 0 {
			t.Fatalf("%s: %d of %d cases differ (seed %d)", b, bad, n, seed)
		}
		t.Logf("%s: %d sessions equal Go's net/smtp SendMail (seed %d)", b, n, seed)
	}
}

// TestSmtpReplyBytes: std/smtp reads a server's reply as Go's
// textproto does. A line that is not UTF-8 is read as sent (it was "", a
// short response: TestSmtpFuzz seed 2 case 9), a reply's error quotes
// the bytes as sent, also from AUTH (it had U+FFFD: seed 5 case 1418),
// and a last line the server ends without CRLF is a line (it was EOF:
// seed 5 case 1258). Go's SendMail checks each expectation.
func TestSmtpReplyBytes(t *testing.T) {
	ok := func() smfCase {
		return smfCase{from: "a@x.test", to: []string{"b@x.test"}, auth: "none", msg: []byte("hi\n"), cut: -1,
			greet: smfReply(220, "mail.test"), ehlo: smfReply(250, "mail.test", "AUTH PLAIN"), helo: smfReply(250, "x"),
			mail: smfReply(250, "ok"), rcpt: [][]byte{smfReply(250, "ok")}, data: smfReply(354, "go"), end: smfReply(250, "ok"), quit: smfReply(221, "bye")}
	}
	cases := []smfCase{ok(), ok(), ok(), ok()}
	cases[0].greet = []byte("220 mail.test \xff\xfe\r\n")
	cases[1].auth, cases[1].user, cases[1].pass = "plain", "u", "p"
	cases[1].auths = [][]byte{[]byte("535 no \xff(\r\n")}
	cases[2].mail = []byte("550 gone" + smfClose)
	cases[3].ehlo = []byte("250-mail.test \xc3(\r\n250 8BITMIME\r\n")
	cases[3].end = []byte("552 too \xffbig\r\n")
	wants := []string{"ok", `error 535 "no \xff("`, `error 550 "gone"`, `error 552 "too \xffbig"`}
	s := smfServe(t, cases)
	defer s.stop()
	for i, c := range cases {
		if g := smfGo(s.addrs[i], c); g != wants[i] {
			t.Fatalf("case %d: Go's SendMail says %q, the test expects %q", i, g, wants[i])
		}
	}
	goTr := s.take()
	work, err := testtmp.Dir("xo-smtpbytes-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(work)
	res, tr := smfRunXo(t, smfBuild(t, work, ""), s, cases)
	for i := range cases {
		got, want := smfCompose(tr[i], res[i]), smfCompose(goTr[i], wants[i])
		if got != want {
			t.Errorf("case %d:\ngot\n%swant\n%s", i, got, want)
		}
	}
}

// TestSmfHangUpUnreadIsEOF: the client's command is in the server's
// buffer, unread, when the server hangs up; the client still reads EOF,
// as when the hang up comes first (TestSmtpFuzz seed 2 case 1333, llvm).
func TestSmfHangUpUnreadIsEOF(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	sent := make(chan struct{})
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		conn.Write([]byte("250 ok\r\n"))
		<-sent
		time.Sleep(50 * time.Millisecond) // the command is in conn's buffer
		smfHangUp(conn)
	}()
	c, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	c.SetDeadline(time.Now().Add(10 * time.Second))
	br := bufio.NewReader(c)
	if _, err := br.ReadString('\n'); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Write([]byte("AUTH PLAIN AGEAYg==\r\n")); err != nil {
		t.Fatal(err)
	}
	close(sent)
	if _, err := br.ReadByte(); err != io.EOF {
		t.Fatalf("read after the hang up: %v, want EOF", err)
	}
}
