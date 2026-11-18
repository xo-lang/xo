package stdtest

// A differential of std/dns against Go's net.Resolver (PreferGo, its own
// Dial) on seeded answers from a server it does not trust (task #90, std
// Proven evidence for dns: decision 0461 asks a harness of every parser
// of untrusted input).
//
// A fake server in this test (UDP, and TCP on the same port for a
// truncated answer) answers case N's question: MX or TXT of cN.example.,
// SRV of _sip._tcp.cN.example., or PTR of 10.0.N/256.N%256 (the case's
// lookup kind is N % 4). The answer has 0 to 5 records of the type asked,
// with A records between them now and then, owner names written out or
// compressed, MX, SRV and PTR names that are domain names or not (a
// leading hyphen, an underscore, the root, 63 byte labels, compressed
// suffixes), TXT strings that are empty, long or not UTF-8; an rcode of
// NXDOMAIN, SERVFAIL, NOTIMP or REFUSED now and then, and no recursion
// available (a lame referral when there is no answer); a truncated UDP
// answer now and then, answered whole over TCP. A quarter of the cases with records
// are changed after they are made: an RDLENGTH past the data or short of
// it, the answer section cut, a byte changed past the question, a
// compression pointer to itself or past the message, or an ANCOUNT above
// the records sent. The header and the question are never changed: both
// resolvers ignore a reply whose header or question does not parse and
// wait out their timeout (Go's dnsPacketRoundTrip), which would cost 10 s
// a case and test nothing else.
//
// Both sides print the records sorted (Go shuffles equal preferences and
// weights) or the error, with "lookup NAME: " and " on SERVER" taken out
// of the error text.
//
// XO_DNSFUZZ_SEED (default 2), XO_DNSFUZZ_N (default 300) and
// XO_DNSFUZZ_BACKENDS (default arm64,llvm). The reference is Go's own
// net package: nothing to build.

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"math/rand"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/xo-lang/xo/internal/testtmp"
	"github.com/xo-lang/xo/internal/xotool"
)

const (
	dnfA   = 1
	dnfPTR = 12
	dnfMX  = 15
	dnfTXT = 16
	dnfSRV = 33
)

type dnfCase struct {
	qtype   int
	rcode   int
	answers [][]byte // whole resource records, names already encoded
	trunc   bool     // UDP answers with TC and no records; TCP answers whole
	lame    bool     // no recursion available (RA clear): with no answer, a lame referral
	note    string   // the change made, "" for a case left as made
	change  func(m []byte, qend int) []byte
}

// dnfLookup is case i's lookup line, and its question name and type.
func dnfLookup(i int) (line, qname string, qtype int) {
	switch i % 4 {
	case 0:
		return fmt.Sprintf("mx c%d.example.", i), fmt.Sprintf("c%d.example.", i), dnfMX
	case 1:
		return fmt.Sprintf("txt c%d.example.", i), fmt.Sprintf("c%d.example.", i), dnfTXT
	case 2:
		return fmt.Sprintf("srv sip tcp c%d.example.", i), fmt.Sprintf("_sip._tcp.c%d.example.", i), dnfSRV
	}
	return fmt.Sprintf("addr 10.0.%d.%d", i/256, i%256), fmt.Sprintf("%d.%d.0.10.in-addr.arpa.", i%256, i/256), dnfPTR
}

// dnfName encodes a name: "@" is a pointer to the question name (offset
// 12), "LABEL.@" a label and then that pointer, else the labels written
// out ("." the root).
func dnfName(n string) []byte {
	if n == "@" {
		return []byte{0xC0, 12}
	}
	var b []byte
	if strings.HasSuffix(n, ".@") {
		for _, l := range strings.Split(strings.TrimSuffix(n, ".@"), ".") {
			b = append(b, byte(len(l)))
			b = append(b, l...)
		}
		return append(b, 0xC0, 12)
	}
	for _, l := range strings.Split(strings.TrimSuffix(n, "."), ".") {
		if l == "" {
			continue
		}
		b = append(b, byte(len(l)))
		b = append(b, l...)
	}
	return append(b, 0)
}

func dnfRR(owner []byte, typ int, rdata []byte) []byte {
	b := append([]byte(nil), owner...)
	b = binary.BigEndian.AppendUint16(b, uint16(typ))
	b = binary.BigEndian.AppendUint16(b, 1) // IN
	b = binary.BigEndian.AppendUint32(b, 300)
	b = binary.BigEndian.AppendUint16(b, uint16(len(rdata)))
	return append(b, rdata...)
}

var dnfHosts = []string{"mx1.example.", "mail.c.example.", "a-b.example.", "x.@", "@", ".", "-bad.example.", "bad_name.example.", strings.Repeat("l", 63) + ".example.", "mx2.example.", "UPPER.Example."}

var dnfTexts = []string{"", "v=spf1 -all", "hello world", "日本語", "\xff\xfe", strings.Repeat("t", 255), "a=\"q\""}

func dnfGen(r *rand.Rand, i int) dnfCase {
	_, _, qtype := dnfLookup(i)
	c := dnfCase{qtype: qtype}
	switch r.Intn(20) {
	case 0:
		c.rcode = 3
	case 1:
		c.rcode = 2
	case 2:
		c.rcode = 5
	case 3:
		c.rcode = 4
	}
	c.lame = r.Intn(15) == 0
	for k := r.Intn(6); k > 0; k-- {
		owner := dnfName("@")
		if r.Intn(4) == 0 {
			_, q, _ := dnfLookup(i)
			owner = dnfName(q)
		}
		if r.Intn(6) == 0 {
			c.answers = append(c.answers, dnfRR(owner, dnfA, []byte{192, 0, 2, byte(k)}))
			continue
		}
		var rd []byte
		host := dnfHosts[r.Intn(len(dnfHosts))]
		switch qtype {
		case dnfMX:
			rd = binary.BigEndian.AppendUint16(nil, uint16(pgfPick(r, 0, 10, 10, 20, 65535)))
			rd = append(rd, dnfName(host)...)
		case dnfTXT:
			for s := 1 + r.Intn(3); s > 0; s-- {
				t := dnfTexts[r.Intn(len(dnfTexts))]
				rd = append(rd, byte(len(t)))
				rd = append(rd, t...)
			}
		case dnfSRV:
			rd = binary.BigEndian.AppendUint16(nil, uint16(pgfPick(r, 0, 1, 1, 2)))
			rd = binary.BigEndian.AppendUint16(rd, uint16(pgfPick(r, 0, 5, 10, 60)))
			rd = binary.BigEndian.AppendUint16(rd, uint16(pgfPick(r, 0, 443, 5060, 65535)))
			rd = append(rd, dnfName(host)...)
		default:
			rd = dnfName(host)
		}
		c.answers = append(c.answers, dnfRR(owner, qtype, rd))
	}
	c.trunc = r.Intn(12) == 0
	if len(c.answers) > 0 && r.Intn(4) == 0 {
		dnfChange(r, &c)
	}
	return c
}

// dnfChange changes a made case past its question, as a hostile server
// could; the change applies to the whole message (qend: where the
// answers start).
func dnfChange(r *rand.Rand, c *dnfCase) {
	k := r.Intn(7)
	at := r.Intn(len(c.answers)) // the record changed
	// every draw is made now: the server answers a query more than once
	// (retries, TCP after TC, both clients), always the same
	delta, pick, bit := uint16(1+r.Intn(40)), r.Intn(1<<30), byte(1<<r.Intn(8))
	// where record at starts, from qend
	off := func(qend int) int {
		o := qend
		for j := 0; j < at; j++ {
			o += len(c.answers[j])
		}
		return o
	}
	// where its RDLENGTH is: after its owner name, type, class and TTL
	rdlen := func(m []byte, o int) int {
		p := o
		for p < len(m) {
			if m[p] == 0 {
				p++
				break
			}
			if m[p]&0xC0 == 0xC0 {
				p += 2
				break
			}
			p += 1 + int(m[p])
		}
		return p + 8
	}
	switch k {
	case 0:
		c.note = "rdlength past"
		c.change = func(m []byte, q int) []byte {
			p := rdlen(m, off(q))
			binary.BigEndian.PutUint16(m[p:], binary.BigEndian.Uint16(m[p:])+delta)
			return m
		}
	case 1:
		c.note = "rdlength short"
		c.change = func(m []byte, q int) []byte {
			p := rdlen(m, off(q))
			if v := binary.BigEndian.Uint16(m[p:]); v > 0 {
				binary.BigEndian.PutUint16(m[p:], v-1)
			}
			return m
		}
	case 2:
		c.note = "cut"
		c.change = func(m []byte, q int) []byte { return m[:q+pick%(len(m)-q)] }
	case 3:
		c.note = "byte"
		c.change = func(m []byte, q int) []byte {
			m[q+pick%(len(m)-q)] ^= bit
			return m
		}
	case 4:
		c.note = "pointer loop"
		c.change = func(m []byte, q int) []byte {
			o := off(q)
			binary.BigEndian.PutUint16(m[o:], 0xC000|uint16(o))
			return m
		}
	case 5:
		c.note = "pointer past"
		c.change = func(m []byte, q int) []byte {
			binary.BigEndian.PutUint16(m[off(q):], 0xC000|uint16(len(m)+10))
			return m
		}
	default:
		c.note = "ancount above"
		c.change = func(m []byte, q int) []byte {
			binary.BigEndian.PutUint16(m[6:], binary.BigEndian.Uint16(m[6:])+1+delta%3)
			return m
		}
	}
}

// dnfAnswer is the reply to query q (its ID and question kept) for the
// cases, over TCP or not.
func dnfAnswer(q []byte, cases []dnfCase, tcp bool) []byte {
	if len(q) < 12 {
		return nil
	}
	// the question: the name's labels, then QTYPE and QCLASS
	p := 12
	var labels []string
	for p < len(q) && q[p] != 0 {
		n := int(q[p])
		if p+1+n > len(q) {
			return nil
		}
		labels = append(labels, string(q[p+1:p+1+n]))
		p += 1 + n
	}
	qend := p + 5
	if qend > len(q) {
		return nil
	}
	qname := strings.ToLower(strings.Join(labels, ".") + ".")
	i := -1
	switch {
	case strings.HasSuffix(qname, ".in-addr.arpa."):
		var a, b int
		if _, err := fmt.Sscanf(qname, "%d.%d.0.10.in-addr.arpa.", &a, &b); err == nil {
			i = b*256 + a
		}
	default:
		s := strings.TrimPrefix(qname, "_sip._tcp.")
		if v, err := strconv.Atoi(strings.TrimSuffix(strings.TrimPrefix(s, "c"), ".example.")); err == nil {
			i = v
		}
	}
	m := append([]byte(nil), q[:qend]...)
	// QR, the query's RD, RA; QDCOUNT 1, no authority or additional
	m[2] = 0x80 | q[2]&0x01
	m[3] = 0x80
	binary.BigEndian.PutUint16(m[4:], 1)
	binary.BigEndian.PutUint16(m[8:], 0)
	binary.BigEndian.PutUint16(m[10:], 0)
	if i < 0 || i >= len(cases) || int(binary.BigEndian.Uint16(q[qend-4:])) != cases[i].qtype {
		m[3] |= 3 // NXDOMAIN: not a case's question
		binary.BigEndian.PutUint16(m[6:], 0)
		return m
	}
	c := cases[i]
	if c.lame {
		m[3] &^= 0x80
	}
	m[3] |= byte(c.rcode)
	if c.trunc && !tcp {
		m[2] |= 0x02
		binary.BigEndian.PutUint16(m[6:], 0)
		return m
	}
	binary.BigEndian.PutUint16(m[6:], uint16(len(c.answers)))
	for _, a := range c.answers {
		m = append(m, a...)
	}
	if c.change != nil {
		orig := append([]byte(nil), m...)
		m = c.change(m, qend)
		if dnfReadsID(m, qend) {
			// a name that may run into the ID would parse by the ID each
			// resolver picks at random (seed 133 case 121, 2026-11-17: a
			// changed byte made a pointer to offset 44 whose name ended
			// at a pointer to offset 0; Go and Xo agreed on 3 of 8 runs)
			m = orig
		}
	}
	return m
}

// dnfReadsID reports whether a reply's records past the question at q hold
// two bytes that read as a compression pointer to offset 0 or 1, the ID:
// a name walk reaches the ID only through one (labels go forward), so
// without one a reply means the same whatever ID the query had.
func dnfReadsID(m []byte, q int) bool {
	for i := q; i+1 < len(m); i++ {
		if m[i] == 0xC0 && m[i+1] < 2 {
			return true
		}
	}
	return false
}

func dnfServe(t *testing.T, cases []dnfCase) (string, func()) {
	var pc net.PacketConn
	var l net.Listener
	for try := 0; ; try++ {
		var err error
		pc, err = net.ListenPacket("udp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		l, err = net.Listen("tcp", pc.LocalAddr().String())
		if err == nil {
			break
		}
		pc.Close()
		if try > 20 {
			t.Fatalf("no port free for both UDP and TCP: %v", err)
		}
	}
	go func() {
		buf := make([]byte, 4096)
		for {
			n, from, err := pc.ReadFrom(buf)
			if err != nil {
				return
			}
			if m := dnfAnswer(buf[:n], cases, false); m != nil {
				pc.WriteTo(m, from)
			}
		}
	}()
	go func() {
		for {
			c, err := l.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				c.SetDeadline(time.Now().Add(10 * time.Second))
				for {
					var lb [2]byte
					if _, err := ioReadFull(c, lb[:]); err != nil {
						return
					}
					q := make([]byte, binary.BigEndian.Uint16(lb[:]))
					if _, err := ioReadFull(c, q); err != nil {
						return
					}
					m := dnfAnswer(q, cases, true)
					if m == nil {
						return
					}
					c.Write(append(binary.BigEndian.AppendUint16(nil, uint16(len(m))), m...))
				}
			}(c)
		}
	}()
	return pc.LocalAddr().String(), func() { pc.Close(); l.Close() }
}

func ioReadFull(c net.Conn, b []byte) (int, error) {
	n := 0
	for n < len(b) {
		k, err := c.Read(b[n:])
		n += k
		if err != nil {
			return n, err
		}
	}
	return n, nil
}

// dnfGo answers the lookups with Go's resolver, as scripts/dnsdiff/xo
// prints std/dns's.
func dnfGo(server string, lines []string) string {
	r := &net.Resolver{PreferGo: true, Dial: func(ctx context.Context, network, _ string) (net.Conn, error) {
		var d net.Dialer
		return d.DialContext(ctx, network, server)
	}}
	ctx := context.Background()
	var out strings.Builder
	for _, line := range lines {
		w := strings.Fields(line)
		fmt.Fprintf(&out, "== %s\n", line)
		var recs []string
		var err error
		switch w[0] {
		case "mx":
			var xs []*net.MX
			xs, err = r.LookupMX(ctx, w[1])
			for _, x := range xs {
				recs = append(recs, fmt.Sprintf("mx %d %s", x.Pref, x.Host))
			}
		case "txt":
			var xs []string
			xs, err = r.LookupTXT(ctx, w[1])
			for _, x := range xs {
				recs = append(recs, "txt "+hex.EncodeToString([]byte(x)))
			}
		case "srv":
			var xs []*net.SRV
			_, xs, err = r.LookupSRV(ctx, w[1], w[2], w[3])
			for _, x := range xs {
				recs = append(recs, fmt.Sprintf("srv %d %d %d %s", x.Priority, x.Weight, x.Port, x.Target))
			}
		default:
			recs, err = r.LookupAddr(ctx, w[1])
			for k := range recs {
				recs[k] = "ptr " + recs[k]
			}
		}
		// Go returns the records it kept with a malformed-records error;
		// std/dns returns the records alone (its doc)
		var de *net.DNSError
		if err != nil && !(errors.As(err, &de) && strings.Contains(de.Err, "invalid names") && len(recs) > 0) {
			fmt.Fprintf(&out, "error %s\n", err)
			continue
		}
		if w[0] == "txt" {
			// std/dns leaves out a record that is not UTF-8 (its doc: a
			// Str is UTF-8), Err when all are; Go keeps them
			kept := recs[:0]
			for _, x := range recs {
				if d, _ := hex.DecodeString(strings.TrimPrefix(x, "txt ")); utf8.Valid(d) {
					kept = append(kept, x)
				}
			}
			if len(kept) == 0 && len(recs) > 0 {
				fmt.Fprintf(&out, "error lookup %s: DNS response contained records which are not UTF-8\n", w[1])
				continue
			}
			recs = kept
		}
		sort.Strings(recs)
		for _, x := range recs {
			out.WriteString(x + "\n")
		}
	}
	return out.String()
}

// dnfNorm takes "lookup NAME: " and " on SERVER" out of the error lines.
func dnfNorm(block, server string) string {
	var b strings.Builder
	for _, l := range strings.SplitAfter(block, "\n") {
		if e, ok := strings.CutPrefix(l, "error "); ok {
			e = strings.ReplaceAll(e, " on "+server, "")
			if strings.HasPrefix(e, "lookup ") {
				if k := strings.Index(e, ": "); k >= 0 {
					e = e[k+2:]
				}
			}
			l = "error " + e
		}
		b.WriteString(l)
	}
	return b.String()
}

func TestDnsFuzz(t *testing.T) {
	if testing.Short() {
		t.Skip("the dns fuzz builds the Xo client")
	}
	seed := int64(envInt(t, "XO_DNSFUZZ_SEED", 2))
	n := envInt(t, "XO_DNSFUZZ_N", 300)
	backends := envBackends("XO_DNSFUZZ_BACKENDS", []string{"arm64", "llvm"})
	r := rand.New(rand.NewSource(seed))
	cases := make([]dnfCase, n)
	lines := make([]string, n)
	for i := range cases {
		cases[i] = dnfGen(r, i)
		lines[i], _, _ = dnfLookup(i)
	}
	server, stop := dnfServe(t, cases)
	defer stop()
	if d := os.Getenv("XO_DNSFUZZ_DUMP"); d != "" {
		// one case's reply as hex, to replay it alone
		i, _ := strconv.Atoi(d)
		_, q, qt := dnfLookup(i)
		query := append([]byte{0, 1, 1, 0, 0, 1, 0, 0, 0, 0, 0, 0}, dnfName(q)...)
		query = binary.BigEndian.AppendUint16(query, uint16(qt))
		query = binary.BigEndian.AppendUint16(query, 1)
		fmt.Printf("%x\n", dnfAnswer(query, cases, true))
		t.Logf("case %d: %s rcode %d trunc %v", i, cases[i].note, cases[i].rcode, cases[i].trunc)
		return
	}
	want := myfBlocksBy(dnfNorm(dnfGo(server, lines), server), "== ", lines)
	root, err := xotool.Root()
	if err != nil {
		t.Fatal(err)
	}
	xo, err := xotool.Path()
	if err != nil {
		t.Fatal(err)
	}
	work, err := testtmp.Dir("xo-dnsfuzz-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(work)
	for _, b := range backends {
		bin := filepath.Join(work, "xo-"+b)
		xb := testtmp.Guard(exec.Command(xo, "build", "--backend="+b, "-o", bin, filepath.Join(root, "scripts", "dnsdiff", "xo")))
		xb.Env = append(os.Environ(), "XO_STD="+filepath.Join(root, "std"))
		if out, err := xb.CombinedOutput(); err != nil {
			t.Fatalf("xo build --backend=%s: %v\n%s", b, err, out)
		}
		cmd := testtmp.Guard(exec.Command(bin, server))
		cmd.Stdin = strings.NewReader(strings.Join(lines, "\n") + "\n")
		var out, errb bytes.Buffer
		cmd.Stdout = &out
		cmd.Stderr = &errb
		if err := cmd.Run(); err != nil {
			t.Fatalf("%s: %v\n%s\nstderr:\n%s", b, err, myfTail(out.String(), 20), myfTail(errb.String(), 30))
		}
		got := myfBlocksBy(dnfNorm(out.String(), server), "== ", lines)
		bad := 0
		refused := map[string]int{}
		for i := 0; i < n; i++ {
			if got[i] == want[i] {
				continue
			}
			if cases[i].note != "" && strings.HasPrefix(got[i], "error ") && strings.Count(got[i], "\n") == 1 {
				// a changed case std/dns refuses where Go's parser reads
				// on (an RDLENGTH past the data, for one)
				refused[cases[i].note]++
				continue
			}
			if bad < 10 {
				t.Errorf("%s case %d (seed %d, %s, rcode %d, trunc %v): %s\ngot\n%swant\n%s", b, i, seed, cases[i].note, cases[i].rcode, cases[i].trunc, lines[i], got[i], want[i])
			}
			bad++
		}
		if bad > 0 {
			t.Fatalf("%s: %d of %d cases differ (seed %d)", b, bad, n, seed)
		}
		t.Logf("%s: %d cases equal Go's net.Resolver or refused by std/dns (seed %d); refused changed cases by change: %v", b, n, seed, refused)
	}
}

// myfBlocksBy are the lines after each PREFIX+line header, by the index
// of that line in lines.
func myfBlocksBy(out, prefix string, lines []string) map[int]string {
	idx := map[string]int{}
	for i, l := range lines {
		idx[l] = i
	}
	m := map[int]string{}
	cur := -1
	var b strings.Builder
	flush := func() {
		if cur >= 0 {
			m[cur] = b.String()
		}
		b.Reset()
	}
	for _, l := range strings.Split(out, "\n") {
		if h, ok := strings.CutPrefix(l, prefix); ok {
			flush()
			cur = -1
			if i, ok := idx[h]; ok {
				cur = i
			}
			continue
		}
		if cur >= 0 && l != "" {
			b.WriteString(l + "\n")
		}
	}
	flush()
	return m
}

// TestDnsAnswerErrors: a REFUSED or NOTIMP answer is std/dns's "server
// misbehaving" and a lame referral its "lame referral", as Go's
// checkHeader says (errServerMisbehaving, errLameReferral); both were
// "cannot unmarshal DNS message" (found by TestDnsFuzz seed 2 cases 36
// and 48). A TXT record whose string runs past its RDLENGTH fails the
// lookup, as Go's unpackTXTResource (errCalcLen); std/dns returned the
// other records (seed 4 case 293). A REFUSED answer cut inside its
// first record's data is "server misbehaving" too: Go's checkHeader skips
// the records without minding errors and parses only the first answer's
// header; std/dns parsed every record first and said "cannot unmarshal
// DNS message" (seed 4 case 3977 of the stdproven run, task #115). An SRV
// answer whose owner name is not a domain name (a control byte in a
// label) is "SRV header name is invalid", as Go's LookupSRV checks the
// first SRV answer's name; std/dns returned the record (seed 5 case 3626;
// seeds 6 and 7 with a byte that is not UTF-8 there).
// Go's resolver checks the expectations.
func TestDnsAnswerErrors(t *testing.T) {
	good := dnfRR(dnfName("@"), dnfTXT, append([]byte{5}, "hello"...))
	short := dnfRR(dnfName("@"), dnfTXT, append([]byte{5}, "abcde"...))
	// RDLENGTH (the 2 bytes before the 6 of rdata) 6 to 3: the string
	// runs past it, into bytes the message has
	short[len(short)-8], short[len(short)-7] = 0, 3
	// RDLENGTH 6 to 255: the record runs past the end of the message
	cut := dnfRR(dnfName("@"), dnfTXT, append([]byte{5}, "hello"...))
	cut[len(cut)-8], cut[len(cut)-7] = 0, 255
	cases := []dnfCase{
		{qtype: dnfMX, rcode: 5},
		{qtype: dnfTXT, rcode: 4},
		{qtype: dnfSRV, lame: true},
		{qtype: dnfPTR, rcode: 2},
		{qtype: dnfMX},
		{qtype: dnfTXT, answers: [][]byte{good, short}},
		{qtype: dnfSRV, rcode: 5, answers: [][]byte{cut}},
		// 7 to 9 keep the questions' cycle (MX, TXT, SRV, PTR by index)
		{qtype: dnfPTR, rcode: 5},
		{qtype: dnfMX, rcode: 5},
		{qtype: dnfTXT, rcode: 5},
		{qtype: dnfSRV, answers: [][]byte{dnfRR([]byte("\x08bad\x07name\x00"), dnfSRV, append([]byte{0, 1, 0, 60, 255, 255}, dnfName("a-b.example.")...))}},
		{qtype: dnfPTR, rcode: 5},
		{qtype: dnfMX, rcode: 5},
		{qtype: dnfTXT, rcode: 5},
		// a byte that is not UTF-8 in the owner name (seed 6 case 1998)
		{qtype: dnfSRV, answers: [][]byte{dnfRR([]byte("\x04bad\xec\x00"), dnfSRV, append([]byte{0, 1, 0, 60, 255, 255}, dnfName("a-b.example.")...))}},
		// a "." byte inside a label: Go's dnsmessage refuses the name, and
		// so the message (golang/go#56246; seed 11 case 1440, task #115)
		{qtype: dnfPTR, answers: [][]byte{dnfRR(dnfName("@"), dnfPTR, []byte("\x08bad_.ame\x07example\x00"))}},
		{qtype: dnfMX, answers: [][]byte{dnfRR(dnfName("@"), dnfMX, append([]byte{0, 10}, "\x08bad_.ame\x07example\x00"...))}},
	}
	server, stop := dnfServe(t, cases)
	defer stop()
	root, err := xotool.Root()
	if err != nil {
		t.Fatal(err)
	}
	xo, err := xotool.Path()
	if err != nil {
		t.Fatal(err)
	}
	work, err := testtmp.Dir("xo-dnsrcode-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(work)
	bin := filepath.Join(work, "xo-dns")
	xb := testtmp.Guard(exec.Command(xo, "build", "-o", bin, filepath.Join(root, "scripts", "dnsdiff", "xo")))
	xb.Env = append(os.Environ(), "XO_STD="+filepath.Join(root, "std"))
	if out, err := xb.CombinedOutput(); err != nil {
		t.Fatalf("xo build: %v\n%s", err, out)
	}
	var lines []string
	for i := range cases {
		l, _, _ := dnfLookup(i)
		lines = append(lines, l)
	}
	cmd := testtmp.Guard(exec.Command(bin, server))
	cmd.Stdin = strings.NewReader(strings.Join(lines, "\n") + "\n")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("xo client: %v\n%s", err, out)
	}
	got := myfBlocksBy(dnfNorm(string(out), server), "== ", lines)
	want := myfBlocksBy(dnfNorm(dnfGo(server, lines), server), "== ", lines)
	for i, w := range []string{"error server misbehaving\n", "error server misbehaving\n", "error lame referral\n", "error server misbehaving\n", "error no such host\n", "error cannot unmarshal DNS message\n", "error server misbehaving\n", "error server misbehaving\n", "error server misbehaving\n", "error server misbehaving\n", "error SRV header name is invalid\n", "error server misbehaving\n", "error server misbehaving\n", "error server misbehaving\n", "error SRV header name is invalid\n", "error cannot unmarshal DNS message\n", "error cannot unmarshal DNS message\n"} {
		if want[i] != w {
			t.Fatalf("case %d: Go's net.Resolver says %q, the test expects %q", i, want[i], w)
		}
		if got[i] != w {
			t.Errorf("case %d (%s): got %q, want %q", i, lines[i], got[i], w)
		}
	}
}
