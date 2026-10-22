package llvm

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/ecdh"
	"crypto/hkdf"
	"crypto/mlkem"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"testing"

	"github.com/xo-lang/xo/internal/testtmp"
	"github.com/xo-lang/xo/internal/xotool"
)

// tls13DiffProgram replays each line of its input (a, b, c and d of
// internal.crypto_op's tls13.replay, in hex) and prints the result in
// hex: on the Go backend std/rt's internal_tls13.go answers, on the
// native ones std/native/llrt/tls13.xo.
const tls13DiffProgram = `use std/internal

fn h(s: Str) -> Bytes { Bytes.from_hex(s) ?? b"" }

fn main(os: Os) {
  let all = os.stdio.read_all().or_fault("stdin")
  for line in all.lines() {
    if line != "" {
      let f = line.split(" ")
      let r = internal.crypto_op("tls13.replay", h(f[0]), h(f[1]), h(f[2]), h(f[3]))
      os.stdio.println(r.to_hex())
    }
  }
}
`

// tls13Trace is a recorded trace of a golden (tls13_rfc8448, tls13_go):
// the replay's inputs, the server's records split in two parts, and what
// the plaintext mutations need (the ClientHello message and the ECDHE
// secret, nil for a trace they skip).
type tls13Trace struct {
	name           string
	a, c           []byte
	hs, post, data []byte
	root           []byte
	d              string
	clientHello    []byte
	shared         []byte
}

// tls13Consts reads the `const NAME: Str = "hex"` lines of a golden.
func tls13Consts(t *testing.T, golden string) map[string][]byte {
	src, err := os.ReadFile(filepath.Join("testdata", "golden", golden, "main.xo"))
	if err != nil {
		t.Fatal(err)
	}
	out := map[string][]byte{}
	re := regexp.MustCompile(`(?m)^const ([A-Z0-9_]+): Str = "([0-9a-f]*)"$`)
	for _, m := range re.FindAllStringSubmatch(string(src), -1) {
		b, err := hex.DecodeString(m[2])
		if err != nil {
			t.Fatalf("%s %s: %v", golden, m[1], err)
		}
		out[m[1]] = b
	}
	return out
}

func tls13Traces(t *testing.T) []tls13Trace {
	r := tls13Consts(t, "tls13_rfc8448")
	cat := func(xs ...[]byte) []byte { return bytes.Join(xs, nil) }
	traces := []tls13Trace{{
		name: "rfc8448", a: r["HELLO"], c: r["SHARED"],
		hs: cat(r["SERVER_HELLO"], r["SERVER_FLIGHT"]), post: cat(r["TICKET"], r["SERVER_DATA_RECORD"], r["SERVER_ALERT"]),
		data: r["CLIENT_DATA"], root: r["CERT"], d: "1600000000 - 1301 0 shared 29 -",
		clientHello: r["HELLO"], shared: r["SHARED"],
	}}
	g := tls13Consts(t, "tls13_go")
	for _, x := range []struct{ p, suites, groups, alpn string }{
		{"HRR_CHACHA", "1303", "29,23", "h2,http/1.1"},
		{"CERTREQ_AES256", "1302", "29", "-"},
		{"ED25519_P256", "1301", "23", "-"},
		{"HYBRID", "1301", "4588,29", "-"},
		{"P521", "1302", "25", "-"},
		{"HYBRID_P256", "1301", "4587,23", "-"},
		{"HYBRID_P384", "1302", "4589,24", "-"},
	} {
		tr := tls13Trace{name: x.p, a: g[x.p+"_START"], c: g[x.p+"_KEY"], hs: g[x.p+"_SERVER_HS"], post: g[x.p+"_SERVER_POST"],
			data: []byte("GET / trace"), root: g["ROOT"],
			d: fmt.Sprintf("1780000000 tls.example %s 0 build %s %s", x.suites, x.groups, x.alpn)}
		if x.p != "HRR_CHACHA" {
			// the ECDHE secret from the client's key and the server's share
			ch := g[x.p+"_CH"][5:]
			sh := tls13Records(tr.hs)[0][5:]
			group, share := tls13KeyShare(sh)
			key := tr.c
			if group == 0x11ec {
				// X25519MLKEM768: ML-KEM's key, then X25519's secret
				dk, err := mlkem.NewDecapsulationKey768(tr.c[:64])
				if err != nil {
					t.Fatal(err)
				}
				k, err := dk.Decapsulate(share[:1088])
				if err != nil {
					t.Fatal(err)
				}
				tr.shared = k
				group, share, key = 29, share[1088:], tr.c[64:96]
			}
			var tail []byte
			if group == 0x11eb || group == 0x11ed {
				// the ECDH part first, then ML-KEM-768's or -1024's ciphertext
				es, eg := 65, 23
				var k []byte
				var err error
				if group == 0x11eb {
					dk, e := mlkem.NewDecapsulationKey768(tr.c[:64])
					if e != nil {
						t.Fatal(e)
					}
					k, err = dk.Decapsulate(share[es:])
				} else {
					es, eg = 97, 24
					dk, e := mlkem.NewDecapsulationKey1024(tr.c[:64])
					if e != nil {
						t.Fatal(e)
					}
					k, err = dk.Decapsulate(share[es:])
				}
				if err != nil {
					t.Fatal(err)
				}
				tail = k
				group, share, key = eg, share[:es], tr.c[64:]
			}
			curve := map[int]ecdh.Curve{29: ecdh.X25519(), 23: ecdh.P256(), 24: ecdh.P384(), 25: ecdh.P521()}[group]
			priv, err := curve.NewPrivateKey(key)
			if err != nil {
				t.Fatal(err)
			}
			pub, err := curve.NewPublicKey(share)
			if err != nil {
				t.Fatal(err)
			}
			x, _ := priv.ECDH(pub)
			tr.shared = append(append(tr.shared, x...), tail...)
			tr.clientHello = ch
		}
		traces = append(traces, tr)
	}
	return traces
}

func tls13Records(b []byte) [][]byte {
	var out [][]byte
	for len(b) >= 5 {
		n := 5 + int(binary.BigEndian.Uint16(b[3:5]))
		if n > len(b) {
			n = len(b)
		}
		out = append(out, b[:n])
		b = b[n:]
	}
	if len(b) > 0 {
		out = append(out, b)
	}
	return out
}

// tls13KeyShare is the group and key of a ServerHello message's
// key_share.
func tls13KeyShare(sh []byte) (int, []byte) {
	sn := int(sh[38])
	for e := sh[44+sn:]; len(e) >= 4; {
		typ, n := int(binary.BigEndian.Uint16(e)), int(binary.BigEndian.Uint16(e[2:]))
		if typ == 51 {
			return int(binary.BigEndian.Uint16(e[4:])), e[8 : 4+n]
		}
		e = e[4+n:]
	}
	return 0, nil
}

// tls13ServerKeys is the AEAD and IV of the server's handshake traffic
// for a trace: the suite from the ServerHello.
func tls13ServerKeys(tr tls13Trace) (cipher.AEAD, []byte) {
	sh := tls13Records(tr.hs)[0][5:]
	sn := int(sh[38])
	suite := int(binary.BigEndian.Uint16(sh[39+sn:]))
	hf, l, kl := sha256.New, 32, 16
	if suite == 0x1302 {
		hf, l, kl = sha512.New384, 48, 32
	}
	sum := func(b []byte) []byte { h := hf(); h.Write(b); return h.Sum(nil) }
	label := func(secret []byte, lb string, ctx []byte, n int) []byte {
		info := []byte{byte(n >> 8), byte(n), byte(6 + len(lb))}
		info = append(info, "tls13 "+lb...)
		info = append(info, byte(len(ctx)))
		info = append(info, ctx...)
		out, _ := hkdf.Expand(hf, secret, string(info), n)
		return out
	}
	early, _ := hkdf.Extract(hf, make([]byte, l), nil)
	derived := label(early, "derived", sum(nil), l)
	hs, _ := hkdf.Extract(hf, tr.shared, derived)
	sHS := label(hs, "s hs traffic", sum(append(append([]byte(nil), tr.clientHello...), sh...)), l)
	blk, _ := aes.NewCipher(label(sHS, "key", nil, kl))
	g, _ := cipher.NewGCM(blk)
	return g, label(sHS, "iv", nil, 12)
}

func tls13Nonce(iv []byte, seq uint64) []byte {
	n := append([]byte(nil), iv...)
	for i := 0; i < 8; i++ {
		n[4+i] ^= byte(seq >> (56 - 8*i))
	}
	return n
}

// tls13Mutate makes a case from trace tr: a mutation of the server's
// records, or of the handshake messages under them (decrypted, changed,
// sealed again in records of random sizes).
func tls13Mutate(rng *rand.Rand, tr tls13Trace) (a, b, c, d []byte) {
	hs, post := append([]byte(nil), tr.hs...), append([]byte(nil), tr.post...)
	dd := strings.Split(tr.d, " ")
	mutateBytes := func(x []byte) []byte {
		if len(x) == 0 {
			return x
		}
		switch rng.Intn(5) {
		case 0, 1:
			x[rng.Intn(len(x))] ^= byte(1 + rng.Intn(255))
		case 2:
			x = x[:rng.Intn(len(x))]
		case 3:
			i := rng.Intn(len(x))
			ins := make([]byte, 1+rng.Intn(8))
			rng.Read(ins)
			x = append(x[:i:i], append(ins, x[i:]...)...)
		case 4:
			// a length or type byte near a message start
			i := rng.Intn(len(x))
			x[i] = byte(rng.Intn(256))
		}
		return x
	}
	switch k := rng.Intn(10); {
	case k < 2:
		hs = mutateBytes(hs)
	case k < 3:
		post = mutateBytes(post)
	case k < 4:
		// records dropped, repeated, or swapped
		recs := tls13Records(hs)
		i, j := rng.Intn(len(recs)), rng.Intn(len(recs))
		switch rng.Intn(3) {
		case 0:
			recs = append(recs[:i:i], recs[i+1:]...)
		case 1:
			recs = append(recs[:i+1:i+1], recs[i:]...)
		case 2:
			recs[i], recs[j] = recs[j], recs[i]
		}
		hs = bytes.Join(recs, nil)
	default:
		if tr.shared != nil {
			hs = tls13MutatePlain(rng, tr, mutateBytes)
		} else {
			hs = mutateBytes(hs)
		}
	}
	if rng.Intn(4) == 0 {
		dd[3] = strconv.Itoa(1 + rng.Intn(40))
	}
	var parts []byte
	for _, p := range [][]byte{hs, post, tr.data, tr.root} {
		parts = append(parts, byte(len(p)>>16), byte(len(p)>>8), byte(len(p)))
		parts = append(parts, p...)
	}
	return tr.a, parts, tr.c, []byte(strings.Join(dd, " "))
}

// tls13MutatePlain decrypts the server's encrypted handshake records,
// mutates the messages, and seals them again (random record sizes, now
// and then padding or another inner type).
func tls13MutatePlain(rng *rand.Rand, tr tls13Trace, mutate func([]byte) []byte) []byte {
	aead, iv := tls13ServerKeys(tr)
	recs := tls13Records(tr.hs)
	var head, plain []byte
	seq := uint64(0)
	for _, r := range recs {
		if r[0] != 23 {
			head = append(head, r...)
			continue
		}
		pt, err := aead.Open(nil, tls13Nonce(iv, seq), r[5:], r[:5])
		if err != nil {
			panic(err)
		}
		seq++
		plain = append(plain, pt[:len(pt)-1]...)
	}
	plain = mutate(plain)
	out := head
	seq = 0
	for off := 0; off < len(plain) || off == 0; {
		n := 1 + rng.Intn(len(plain)+1)
		if off+n > len(plain) {
			n = len(plain) - off
		}
		inner := append(append([]byte(nil), plain[off:off+n]...), 22)
		if rng.Intn(10) == 0 {
			inner[len(inner)-1] = byte(rng.Intn(256))
		}
		if rng.Intn(10) == 0 {
			inner = append(inner, make([]byte, rng.Intn(20))...)
		}
		hdr := []byte{23, 3, 3, 0, 0}
		binary.BigEndian.PutUint16(hdr[3:], uint16(len(inner)+16))
		out = append(out, aead.Seal(hdr, tls13Nonce(iv, seq), inner, hdr)...)
		seq++
		off += n
		if n == 0 {
			break
		}
	}
	return out
}

// TestTLS13ReplayDiff feeds the TLS 1.3 client mutated server records
// (decision 0202's differential fuzzing: the recorded traces of the
// tls13_rfc8448 and tls13_go goldens with bytes flipped, cut, inserted,
// records dropped, repeated or swapped, handshake messages changed under
// a valid record protection, fed in pieces) and diffs what the Xo client
// (tls13.xo, the LLVM and arm64 backends) does against the Go reference
// port (std/rt's internal_tls13.go, the Go backend): the bytes it sends,
// its state, its error text. XO_TLS13_DIFF_N cases (default 3000) from
// XO_TLS13_DIFF_SEED (default 7).
func TestTLS13ReplayDiff(t *testing.T) {
	if testing.Short() {
		t.Skip("builds a program with three backends")
	}
	if _, err := xotool.Clang(); err != nil {
		t.Skip("clang not found")
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "main.xo"), []byte(tls13DiffProgram), 0o644); err != nil {
		t.Fatal(err)
	}
	// std/internal is for the repository's modules only
	mod := "module github.com/xo-lang/xo/test/tls13diff\nedition preview\n"
	if err := os.WriteFile(filepath.Join(dir, "xo.mod"), []byte(mod), 0o644); err != nil {
		t.Fatal(err)
	}
	n := 3000
	if v, err := strconv.Atoi(os.Getenv("XO_TLS13_DIFF_N")); err == nil {
		n = v
	}
	seed := int64(7)
	if v, err := strconv.ParseInt(os.Getenv("XO_TLS13_DIFF_SEED"), 10, 64); err == nil {
		seed = v
	}
	traces := tls13Traces(t)
	rng := rand.New(rand.NewSource(seed))
	var in strings.Builder
	var cases []string
	for i := 0; i < n; i++ {
		tr := traces[rng.Intn(len(traces))]
		a, b, c, d := tls13Mutate(rng, tr)
		line := strings.Join([]string{hex.EncodeToString(a), hex.EncodeToString(b), hex.EncodeToString(c), hex.EncodeToString(d)}, " ")
		cases = append(cases, tr.name)
		in.WriteString(line + "\n")
	}
	backends := []string{xotool.BackendGo, xotool.BackendLLVM}
	if runtime.GOARCH == "arm64" {
		backends = append(backends, xotool.BackendArm64)
	}
	outs := make([][]string, len(backends))
	for i, be := range backends {
		res, err := xotool.Build([]string{dir}, xotool.Options{Backend: be})
		if err != nil {
			t.Fatalf("build (%s): %v", be, err)
		}
		if res.Failed {
			t.Fatalf("build (%s) failed: %v", be, res.Output)
		}
		cmd := testtmp.Guard(exec.Command(res.Binaries[xotool.Host()]))
		cmd.Stdin = strings.NewReader(in.String())
		var stderr bytes.Buffer
		cmd.Stderr = &stderr
		out, err := cmd.Output()
		if err != nil {
			t.Fatalf("%s: %v\n%s", be, err, stderr.String())
		}
		outs[i] = strings.Split(strings.TrimSuffix(string(out), "\n"), "\n")
		if len(outs[i]) != n {
			t.Fatalf("%s: %d results for %d cases", be, len(outs[i]), n)
		}
	}
	// a difference in what the client does (finishes or not, the bytes
	// it sends while it does, the class of its error) and one only in a
	// refused certificate's text are counted apart, both failures
	bad, texts := 0, 0
	states := map[string]int{}
	for i := 0; i < n; i++ {
		ref, _ := hex.DecodeString(outs[0][i])
		states[tls13DiffState(string(ref))]++
		for b := 1; b < len(backends); b++ {
			if outs[b][i] == outs[0][i] {
				continue
			}
			got, _ := hex.DecodeString(outs[b][i])
			if tls13DiffClass(string(got)) == tls13DiffClass(string(ref)) {
				texts++
				if testing.Verbose() && texts <= 20 {
					t.Logf("case %d (%s): %s text differs\ngo: %s\n%s: %s", i, cases[i], backends[b], tls13DiffErr(string(ref)), backends[b], tls13DiffErr(string(got)))
				}
				continue
			}
			if bad < 5 {
				t.Errorf("case %d (%s): %s differs from go\ngo:\n%s\n%s:\n%s", i, cases[i], backends[b], ref, backends[b], got)
			}
			bad++
		}
	}
	t.Logf("%d cases, %d text only differences, outcomes: %v", n, texts, states)
	if bad > 0 {
		t.Errorf("%d differences", bad)
	}
	// x509.xo parses as Go's parser and verifies with Go's hints since
	// 2026-10-15: a text difference is a regression too
	if texts > 0 {
		t.Errorf("%d text only differences (run with -v to list them)", texts)
	}
}

// tls13DiffClass is what a replay did, without the texts of a refused
// certificate: the lines before the handshake's end, and for a failure
// its class (a certificate refused, by parsing or verifying, is one).
func tls13DiffClass(out string) string {
	lines := strings.Split(out, "\n")
	for i, line := range lines {
		rest, ok := strings.CutPrefix(line, "flight ")
		if !ok {
			continue
		}
		f := strings.SplitN(rest, " ", 3)
		if f[0] == "6" || len(f) < 3 {
			return out
		}
		msg := f[2]
		// x509.xo parses some certificates Go's parser refuses (a malformed
		// OID, an invalid PrintableString); they are refused after it
		if strings.HasPrefix(msg, "tls: failed to parse certificate from server") || strings.HasPrefix(msg, "tls: failed to verify certificate") ||
			msg == "tls: server's certificate contains an unsupported type of public key" {
			msg = "certificate refused"
		}
		return strings.Join(lines[:i], "\n") + "\nfailed " + msg
	}
	return out
}

// tls13DiffState is a replay's outcome: done, or the error's text.
func tls13DiffState(out string) string {
	for _, line := range strings.Split(out, "\n") {
		if rest, ok := strings.CutPrefix(line, "flight "); ok {
			f := strings.SplitN(rest, " ", 3)
			if f[0] == "6" {
				return "done"
			}
			if len(f) == 3 {
				msg := f[2]
				if i := strings.Index(msg, ": x509:"); i >= 0 {
					msg = msg[:i]
				}
				return msg
			}
		}
	}
	return "?"
}

// tls13DiffErr is a replay's error text, "" when it finished.
func tls13DiffErr(out string) string {
	for _, line := range strings.Split(out, "\n") {
		if rest, ok := strings.CutPrefix(line, "flight "); ok {
			if f := strings.SplitN(rest, " ", 3); len(f) == 3 {
				return f[2]
			}
		}
	}
	return ""
}
