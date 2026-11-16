package stdtest

// A differential of std/textproto against Go's net/textproto (task #115):
// random line protocol text (header blocks with continued lines, bad keys,
// control bytes and values that are not UTF-8, reply lines of one and of
// several lines, dot-encoded blocks, lines ended by \n, by \r\n or by the
// end of the input) read by a random series of ReadLine,
// ReadContinuedLine, ReadMIMEHeader, ReadCodeLine, ReadResponse,
// ReadDotBytes and ReadDotLines, the first error ending the case, then the
// bytes left; and CanonicalMIMEHeaderKey, TrimString, a DotWriter, and
// MIMEHeader's Add, Set, Del, Get and Values. Each result is compared,
// as hex, with what scripts/textprotodiff/xo prints, built with each
// backend.
//
// Where Xo differs by design: a Str is UTF-8, so Go's strings are read as
// Xo's to_str_lossy reads bytes (string([]rune(s))); a header's keys are
// compared sorted (MIMEHeader is a map); ReadMIMEHeader's header with
// io.EOF is Xo's Ok with header_truncated, and its io.EOF with no fields
// Xo's Eof; Get of a key with no values is Xo's None.
//
//   XO_TEXTPROTOFUZZ_SEED (2), _N (20000), _BACKENDS (arm64,llvm)

import (
	"bufio"
	"bytes"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"net/textproto"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/xo-lang/xo/internal/testtmp"
	"github.com/xo-lang/xo/internal/xotool"
)

func tpfPick(r *rand.Rand, xs []string) string { return xs[r.Intn(len(xs))] }

func tpfX(s string) string { return "x" + hex.EncodeToString([]byte(s)) }

func tpfUnx(s string) string {
	b, err := hex.DecodeString(strings.TrimPrefix(s, "x"))
	if err != nil {
		panic(err)
	}
	return string(b)
}

// tpfLossy is Xo's to_str_lossy: each byte of an invalid sequence U+FFFD.
func tpfLossy(s string) string { return string([]rune(s)) }

func tpfH(s string) string { return hex.EncodeToString([]byte(tpfLossy(s))) }

var tpfKeys = []string{"Content-Type", "content-type", "X-Id", "x-id", "Accept-Encoding", "ACCEPT-ENCODING", "a", "A", "Foo_Bar", "foo-", "-foo", "1-2", "WWW-Authenticate", "www-authenticate", "X-Ünï", "bad key", "bad\tkey", "k@y", "k{", "", "Subject", "subject", "Received", "dkim-signature"}

var tpfValues = []string{"text/plain", "", " leading", "trailing ", "a, b", "x\x01y", "x\x7fy", "x\ty", "caf\xc3\xa9", "\xff\xfe", "\xc3", "=?utf-8?q?x?=", "a:b:c", "1", "  ", "\t", "éé"}

func tpfEnd(r *rand.Rand) string {
	switch r.Intn(6) {
	case 0:
		return "\n"
	case 1:
		if r.Intn(3) == 0 {
			return "\r"
		}
	}
	return "\r\n"
}

func tpfLine(r *rand.Rand) string {
	switch r.Intn(14) {
	case 0, 1, 2, 3:
		sep := tpfPick(r, []string{": ", ":", ":  ", " : ", ":\t"})
		return tpfPick(r, tpfKeys) + sep + tpfPick(r, tpfValues)
	case 4:
		return tpfPick(r, []string{" ", "\t", "  ", " \t"}) + tpfPick(r, tpfValues)
	case 5:
		return ""
	case 6:
		return tpfPick(r, []string{"NoColon", "no colon here", ":empty key", "\x00", "a\x00: b", "Key\x80: v"})
	case 7, 8:
		code := tpfPick(r, []string{"250", "220", "354", "500", "550", "099", "25", "2500", "abc", "1x0", "999", "100", "251"})
		return code + tpfPick(r, []string{" ", "-", "", "x", "  "}) + tpfPick(r, []string{"ok", "hello there", "", "PIPELINING", "multi\xffbyte"})
	case 9:
		return "."
	case 10:
		return tpfPick(r, []string{"..", ".x", "..x", ". ", "x.", ".\xff"})
	default:
		n := r.Intn(30)
		var b strings.Builder
		for i := 0; i < n; i++ {
			b.WriteByte(tpfPick(r, []string{"a", "b", " ", "\t", ".", ":", "-", "\xc3\xa9", "\xff", "Z", "0"})[0])
		}
		return b.String()
	}
}

var tpfOps = []string{"line", "cont", "hdr", "hdr", "code", "resp", "resp", "dotb", "dotl"}

func tpfGen(r *rand.Rand) string {
	switch r.Intn(20) {
	case 0:
		return "canon " + tpfX(tpfLossy(tpfPick(r, tpfKeys)+tpfPick(r, []string{"", "-x", "-X", " ", "é"})))
	case 1:
		return "trim " + tpfX(tpfLossy(tpfPick(r, []string{" ", "\t", "\r\n", "", "\v", " "})+tpfPick(r, tpfValues)+tpfPick(r, []string{" ", "\n", "", "\f", "\u0085"})))
	case 2:
		var b strings.Builder
		for i := r.Intn(6); i >= 0; i-- {
			b.WriteString(tpfLine(r))
			if i > 0 || r.Intn(2) == 0 {
				b.WriteString(tpfPick(r, []string{"\n", "\r\n", "\r", "\n\n"}))
			}
		}
		return "dotenc " + tpfX(b.String())
	case 3, 4:
		var ops []string
		for i := 1 + r.Intn(8); i > 0; i-- {
			k := tpfX(tpfLossy(tpfPick(r, tpfKeys)))
			v := tpfX(tpfLossy(tpfPick(r, tpfValues)))
			switch r.Intn(5) {
			case 0, 1:
				ops = append(ops, "a:"+k+":"+v)
			case 2:
				ops = append(ops, "s:"+k+":"+v)
			case 3:
				ops = append(ops, "d:"+k)
			default:
				ops = append(ops, "g:"+k)
			}
		}
		return "hops " + strings.Join(ops, " ")
	}
	if r.Intn(5) < 2 {
		return tpfShaped(r)
	}
	var in strings.Builder
	n := r.Intn(10)
	for i := 0; i < n; i++ {
		in.WriteString(tpfLine(r))
		if i < n-1 || r.Intn(4) != 0 {
			in.WriteString(tpfEnd(r))
		}
	}
	ops := make([]string, 1+r.Intn(4))
	for i := range ops {
		ops[i] = tpfPick(r, tpfOps)
		if ops[i] == "code" || ops[i] == "resp" {
			ops[i] += ":" + tpfPick(r, []string{"0", "2", "25", "250", "220", "5", "550", "-1", "1000"})
		}
	}
	return "s " + tpfX(in.String()) + " " + strings.Join(ops, " ")
}

// tpfShaped is mostly well formed text, read in its order: a header block
// (folded values, repeated keys, a bad byte now and then), a reply of one
// or several lines, a dot-encoded block, each ended as a protocol ends it.
func tpfShaped(r *rand.Rand) string {
	var in strings.Builder
	end := func() string {
		if r.Intn(8) == 0 {
			return "\n"
		}
		return "\r\n"
	}
	var ops []string
	for part := 0; part < 3; part++ {
		switch (part + r.Intn(3)) % 3 {
		case 0:
			for i := r.Intn(6); i >= 0; i-- {
				k := tpfPick(r, tpfKeys[:14])
				v := tpfPick(r, tpfValues)
				if r.Intn(12) != 0 {
					v = strings.Map(func(c rune) rune {
						if c < 0x20 && c != '\t' || c == 0x7f {
							return 'x'
						}
						return c
					}, v)
				}
				in.WriteString(k + ":" + tpfPick(r, []string{" ", "", "\t"}) + v + end())
				for j := r.Intn(4) - 2; j > 0; j-- {
					in.WriteString(tpfPick(r, []string{" ", "\t", "   "}) + tpfPick(r, tpfValues[:5]) + end())
				}
			}
			if r.Intn(10) != 0 {
				in.WriteString(end())
			}
			ops = append(ops, "hdr")
		case 1:
			code := tpfPick(r, []string{"250", "220", "550", "354"})
			for i := r.Intn(4); i > 0; i-- {
				in.WriteString(code + "-" + tpfPick(r, []string{"PIPELINING", "SIZE 1000", "", "8BITMIME", "caf\xc3\xa9"}) + end())
				if r.Intn(10) == 0 {
					in.WriteString(tpfPick(r, []string{"251 other", "x", "25"}) + end())
				}
			}
			in.WriteString(code + " " + tpfPick(r, []string{"ok", "done", ""}) + end())
			ops = append(ops, tpfPick(r, []string{"resp:0", "resp:2", "resp:" + code, "resp:5", "code:" + code, "code:0"}))
		default:
			for i := r.Intn(5); i > 0; i-- {
				in.WriteString(tpfPick(r, []string{"..", ".x", "..x", "body", "", " ", "a.b"}) + end())
			}
			if r.Intn(12) != 0 {
				in.WriteString("." + end())
			}
			ops = append(ops, tpfPick(r, []string{"dotb", "dotl"}))
		}
	}
	if r.Intn(3) == 0 {
		ops = append(ops, tpfPick(r, tpfOps))
	}
	return "s " + tpfX(in.String()) + " " + strings.Join(ops, " ")
}

func tpfVals(xs []string) string {
	out := make([]string, len(xs))
	for i, x := range xs {
		out[i] = tpfH(x)
	}
	return "[" + strings.Join(out, ",") + "]"
}

func tpfShow(h textproto.MIMEHeader) string {
	keys := make([]string, 0, len(h))
	for k := range h {
		keys = append(keys, k)
	}
	// Xo sorts its (lossy) keys as Str: by bytes
	sort.Slice(keys, func(i, j int) bool { return tpfLossy(keys[i]) < tpfLossy(keys[j]) })
	parts := make([]string, len(keys))
	for i, k := range keys {
		parts[i] = tpfH(k) + "=" + tpfVals(h[k])
	}
	return strings.Join(parts, ";")
}

func tpfErr(err error) string {
	var pe textproto.ProtocolError
	var ce *textproto.Error
	switch {
	case errors.As(err, &ce):
		return fmt.Sprintf("err code %d %s", ce.Code, tpfH(ce.Msg))
	case errors.As(err, &pe):
		return "err proto " + tpfH(string(pe))
	case errors.Is(err, io.ErrUnexpectedEOF):
		return "err ueof"
	case err == io.EOF:
		return "err eof"
	}
	return "err other " + tpfH(err.Error())
}

// tpfGo is what the Xo client prints for line, from Go's net/textproto.
func tpfGo(line string) string {
	w := strings.Split(line, " ")
	var b strings.Builder
	switch w[0] {
	case "canon":
		b.WriteString("v " + tpfH(textproto.CanonicalMIMEHeaderKey(tpfUnx(w[1]))) + "\n")
	case "trim":
		b.WriteString("v " + tpfH(textproto.TrimString(tpfUnx(w[1]))) + "\n")
	case "dotenc":
		var out bytes.Buffer
		bw := bufio.NewWriter(&out)
		dw := textproto.NewWriter(bw).DotWriter()
		dw.Write([]byte(tpfUnx(w[1])))
		dw.Close()
		bw.Flush()
		b.WriteString("v " + hex.EncodeToString(out.Bytes()) + "\n")
	case "hops":
		h := textproto.MIMEHeader{}
		for _, op := range w[1:] {
			f := strings.Split(op, ":")
			switch f[0] {
			case "a":
				h.Add(tpfUnx(f[1]), tpfUnx(f[2]))
			case "s":
				h.Set(tpfUnx(f[1]), tpfUnx(f[2]))
			case "d":
				h.Del(tpfUnx(f[1]))
			default:
				k := tpfUnx(f[1])
				vs := h.Values(k)
				get := "none"
				if len(vs) > 0 {
					get = tpfH(h.Get(k))
				}
				fmt.Fprintf(&b, "get %s %s %s\n", tpfH(k), get, tpfVals(vs))
			}
		}
		b.WriteString("all " + tpfShow(h) + "\n")
	default:
		br := bufio.NewReader(bytes.NewReader([]byte(tpfUnx(w[1]))))
		tr := textproto.NewReader(br)
		for _, op := range w[2:] {
			name, arg, _ := strings.Cut(op, ":")
			n, _ := strconv.Atoi(arg)
			var res string
			var err error
			switch name {
			case "line":
				var s string
				s, err = tr.ReadLine()
				res = "ok " + tpfH(s)
			case "cont":
				var s string
				s, err = tr.ReadContinuedLine()
				res = "ok " + tpfH(s)
			case "hdr":
				var h textproto.MIMEHeader
				h, err = tr.ReadMIMEHeader()
				if err == io.EOF && len(h) > 0 {
					err = nil
					res = "ok trunc " + tpfShow(h)
				} else {
					res = "ok end " + tpfShow(h)
				}
			case "code":
				var c int
				var m string
				c, m, err = tr.ReadCodeLine(n)
				res = fmt.Sprintf("ok %d %s", c, tpfH(m))
			case "resp":
				var c int
				var m string
				c, m, err = tr.ReadResponse(n)
				res = fmt.Sprintf("ok %d %s", c, tpfH(m))
			case "dotb":
				var d []byte
				d, err = tr.ReadDotBytes()
				res = "ok " + hex.EncodeToString(d)
			default:
				var ls []string
				ls, err = tr.ReadDotLines()
				res = "ok " + tpfVals(ls)
			}
			if err != nil {
				b.WriteString(name + " " + tpfErr(err) + "\n")
				break
			}
			b.WriteString(name + " " + res + "\n")
		}
		rest, _ := io.ReadAll(br)
		b.WriteString("rest " + hex.EncodeToString(rest) + "\n")
	}
	return b.String()
}

func tpfBuild(t *testing.T, work, backend string) string {
	root, err := xotool.Root()
	if err != nil {
		t.Fatal(err)
	}
	xo, err := xotool.Path()
	if err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(work, "xo-textproto-"+backend)
	args := []string{"build"}
	if backend != "" {
		args = append(args, "--backend="+backend)
	}
	args = append(args, "-o", bin, filepath.Join(root, "scripts", "textprotodiff", "xo"))
	xb := testtmp.Guard(exec.Command(xo, args...))
	xb.Env = append(os.Environ(), "XO_STD="+filepath.Join(root, "std"))
	if out, err := xb.CombinedOutput(); err != nil {
		t.Fatalf("xo build --backend=%s: %v\n%s", backend, err, out)
	}
	return bin
}

func tpfShowLine(line string) string {
	w := strings.Split(line, " ")
	for i := 1; i < len(w); i++ {
		f := strings.Split(w[i], ":")
		for j := range f {
			if strings.HasPrefix(f[j], "x") {
				f[j] = strconv.Quote(tpfUnx(f[j]))
			}
		}
		w[i] = strings.Join(f, ":")
	}
	return strings.Join(w, " ")
}

func TestTextprotoFuzz(t *testing.T) {
	if testing.Short() {
		t.Skip("the textproto fuzz builds the Xo client")
	}
	seed := int64(envInt(t, "XO_TEXTPROTOFUZZ_SEED", 2))
	n := envInt(t, "XO_TEXTPROTOFUZZ_N", 20000)
	backends := envBackends("XO_TEXTPROTOFUZZ_BACKENDS", []string{"arm64", "llvm"})
	r := rand.New(rand.NewSource(seed))
	lines := make([]string, n)
	want := make([]string, n)
	errs := 0
	for i := range lines {
		lines[i] = tpfGen(r)
		want[i] = tpfGo(lines[i])
		if strings.Contains(want[i], " err ") {
			errs++
		}
	}
	t.Logf("seed %d: %d cases, %d of them end in an error in Go", seed, n, errs)
	work, err := testtmp.Dir("xo-textprotofuzz-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(work)
	for _, b := range backends {
		got := tpfRunXo(t, tpfBuild(t, work, b), lines)
		if len(got) != n {
			t.Fatalf("%s: %d results for %d cases", b, len(got), n)
		}
		bad := 0
		for i := 0; i < n; i++ {
			if got[i] == want[i] {
				continue
			}
			if bad < 10 {
				t.Errorf("%s case %d (seed %d): %s\ngot\n%swant\n%s", b, i, seed, tpfShowLine(lines[i]), got[i], want[i])
			}
			bad++
		}
		if bad > 0 {
			t.Fatalf("%s: %d of %d cases differ (seed %d)", b, bad, n, seed)
		}
		t.Logf("%s: %d cases equal Go's net/textproto (seed %d)", b, n, seed)
	}
}

// tpfRunXo runs the client on lines: each case's output, the lines after
// its "== " line.
func tpfRunXo(t *testing.T, bin string, lines []string) []string {
	cmd := testtmp.Guard(exec.Command(bin))
	cmd.Stdin = strings.NewReader(strings.Join(lines, "\n") + "\n")
	var out, errb bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errb
	if err := cmd.Run(); err != nil {
		t.Fatalf("%s: %v\n%s\nstderr:\n%s", bin, err, myfTail(out.String(), 20), myfTail(errb.String(), 30))
	}
	var bs []string
	for _, l := range strings.Split(out.String(), "\n") {
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
