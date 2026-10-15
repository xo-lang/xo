package llvm

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hkdf"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"testing"

	"github.com/xo-lang/xo/internal/xotool"
)

// tlsSrvDiffProgram runs each line of its input (an op of
// internal.crypto_op and its a, b, c and d, in hex) and prints the
// result in hex: on the Go backend std/rt's references answer
// (internal_tls13srv.go, the TLS 1.3 server written out in Go;
// internal_tls12.go, crypto/tls's own server), on the native ones
// tls13srv.xo and tls12srv.xo.
const tlsSrvDiffProgram = `use std/internal

fn h(s: Str) -> Bytes { Bytes.from_hex(s) ?? b"" }

fn main(os: Os) {
  let all = os.stdio.read_all().or_fault("stdin")
  for line in all.lines() {
    if line != "" {
      let f = line.split(" ")
      let r = internal.crypto_op(f[0], h(f[1]), h(f[2]), h(f[3]), h(f[4]))
      os.stdio.println(r.to_hex())
    }
  }
}
`

// tlsSrvTrace is a recorded client of a golden (tls13_server,
// tls12_server): its flights (TLS 1.3: fed one after another; TLS 1.2:
// the handshake records and the records after it), the server's
// settings and hooks as the golden passes them, and for the RFC 8448
// trace the client's handshake traffic keys (to alter its Finished under
// a valid protection).
type tlsSrvTrace struct {
	name    string
	op      string
	flights [][]byte
	sv      [][]byte
	hooks   [][]byte
	d       []string
	tls12   bool
	key     []byte // tls12.server's a
	cKey    []byte // the client's handshake key and IV (RFC 8448 only)
	cIV     []byte
}

func tlsSrvTraces(t *testing.T) []tlsSrvTrace {
	g := tls13Consts(t, "tls13_server")
	var out []tlsSrvTrace
	// RFC 8448 section 3 with its encoding, EncryptedExtensions and ticket
	rfc := tlsSrvTrace{name: "rfc8448", op: "tls13srv.replay",
		flights: [][]byte{g["HELLO_RECORD"], g["CLIENT_FINISHED"], tlsSrvCat(g["CLIENT_DATA_RECORD"], g["CLIENT_ALERT"])},
		sv:      [][]byte{g["CERT"], g["RSA_KEY"], g["EE"], g["TICKET_MSG"], g["SERVER_DATA"]},
		hooks:   [][]byte{g["SERVER_RANDOM"], g["SERVER_KEY"], g["SALT"]},
		d:       []string{"-", "-", "-", "1", "0", "1"}}
	r8448 := tls13Consts(t, "tls13_rfc8448")
	rfc.cKey, rfc.cIV = tlsSrvClientKeys(g["HELLO_RECORD"][5:], g["SERVER_HELLO"][5:], r8448["SHARED"])
	out = append(out, rfc)
	goHooks := func(key []byte) [][]byte { return [][]byte{g["GO_RANDOM"], key, g["GO_SALT"]} }
	for _, x := range []struct {
		name, prefix  string
		n             int
		cert, key     string
		suites, grps  string
		alpn, ecdhKey string
	}{
		{"go rsa alpn", "RSA_F", 3, "CERT", "RSA_KEY", "-", "29,23,24", "h2,http/1.1", "GO_KEY"},
		{"go hrr p-256", "HRR_F", 5, "CERT", "RSA_KEY", "-", "23", "-", "GO_KEY"},
		{"go ed25519 chacha", "ED_F", 3, "ED_CERT", "ED_KEY", "1303", "24,29", "-", "GO_KEY_P384"},
	} {
		var fs [][]byte
		for i := 0; i < x.n; i++ {
			fs = append(fs, g[x.prefix+strconv.Itoa(i)])
		}
		out = append(out, tlsSrvTrace{name: x.name, op: "tls13srv.replay", flights: fs,
			sv: [][]byte{g[x.cert], g[x.key], []byte("-"), nil, []byte("hello from xo")}, hooks: goHooks(g[x.ecdhKey]),
			d: []string{x.suites, x.grps, x.alpn, "0", "0", "1"}})
	}
	// session resumption: the ticket, then the resumed handshake
	tk := append(goHooks(g["GO_KEY"]), g["TICKET_SEED"], []byte("1800000000"), g["TICKET_IV"], g["AGE_ADD"])
	for _, r := range []string{"R0", "R1"} {
		out = append(out, tlsSrvTrace{name: "go resume " + r, op: "tls13srv.replay",
			flights: [][]byte{g[r+"_F0"], g[r+"_F1"], g[r+"_F2"]},
			sv:      [][]byte{g["ED_CERT"], g["ED_KEY"], []byte("-"), nil, []byte("hello from xo")}, hooks: tk,
			d: []string{"-", "29,23,24", "-", "0", "0", "1"}})
	}
	// TLS 1.2: crypto/tls's clients of the tls12_server golden
	t2 := tls13Consts(t, "tls12_server")
	// a: the X25519, P-256 and P-384 keys, the one of the group chosen
	// (a mutation may change it); mask 1: the ServerKeyExchange shown as
	// its signature's check (ECDSA's signatures are not deterministic)
	keys := tlsSrvCat(t2["SERVER_X25519"], t2["SERVER_P256"], t2["SERVER_P384"])
	for _, x := range []struct {
		p, cert, skey, kt, alpn string
		sig                     bool
	}{
		{"A", "CERT_P256", "KEY_P256_D", "p256", "-", true},
		{"B", "CERT_P384", "KEY_P384_D", "p384", "-", true},
		{"C", "CERT_RSA", "KEY_RSA", "rsa", "-", false},
		{"D", "CERT_RSA", "KEY_RSA", "rsa", "http/1.1", false},
		{"F", "CERT_P256", "KEY_P256_D", "p256", "-", true},
		{"H", "CERT_P256", "KEY_P256_D", "p256", "-", true},
	} {
		var sig []byte
		if x.sig {
			sig = t2[x.p+"_SIG"]
		}
		out = append(out, tlsSrvTrace{name: "tls12 " + x.p, op: "tls12.server", tls12: true, key: keys,
			flights: [][]byte{t2[x.p+"_C_HS"], t2[x.p+"_C_APP"]},
			sv:      [][]byte{t2[x.cert], t2[x.skey], sig},
			d:       []string{"c02b,c02f,c02c,c030,cca9,cca8,c009,c013,c00a,c014", "29,23,24", x.alpn, x.kt, "0", "1"}})
	}
	return out
}

func tlsSrvCat(xs ...[]byte) []byte { return bytes.Join(xs, nil) }

// tlsSrvClientKeys is the client's handshake traffic key and IV
// (AES-128-GCM, SHA-256) of a ClientHello and ServerHello and the ECDHE
// secret.
func tlsSrvClientKeys(ch, sh, shared []byte) ([]byte, []byte) {
	label := func(secret []byte, lb string, ctx []byte, n int) []byte {
		info := []byte{byte(n >> 8), byte(n), byte(6 + len(lb))}
		info = append(info, "tls13 "+lb...)
		info = append(info, byte(len(ctx)))
		info = append(info, ctx...)
		out, _ := hkdf.Expand(sha256.New, secret, string(info), n)
		return out
	}
	empty := sha256.Sum256(nil)
	early, _ := hkdf.Extract(sha256.New, make([]byte, 32), nil)
	hs, _ := hkdf.Extract(sha256.New, shared, label(early, "derived", empty[:], 32))
	th := sha256.Sum256(tlsSrvCat(ch, sh))
	cHS := label(hs, "c hs traffic", th[:], 32)
	return label(cHS, "key", nil, 16), label(cHS, "iv", nil, 12)
}

func tlsSrvPack(xs [][]byte) []byte {
	var out []byte
	for _, p := range xs {
		out = append(out, byte(len(p)>>16), byte(len(p)>>8), byte(len(p)))
		out = append(out, p...)
	}
	return out
}

// tlsSrvMutate makes a case from trace tr: bytes of a client flight
// flipped, cut, inserted or rewritten, records dropped, repeated or
// swapped (also across flights), flights dropped or repeated, the RFC
// trace's Finished altered under the client's valid protection, and
// the client's bytes fed in pieces.
func tlsSrvMutate(rng *rand.Rand, tr tlsSrvTrace) (a, b, c, d []byte) {
	fs := make([][]byte, len(tr.flights))
	for i, f := range tr.flights {
		fs[i] = append([]byte(nil), f...)
	}
	dd := append([]string(nil), tr.d...)
	mutateBytes := func(x []byte) []byte {
		if len(x) == 0 {
			return x
		}
		switch rng.Intn(6) {
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
			x[rng.Intn(len(x))] = byte(rng.Intn(256))
		case 5:
			// a byte near the start: record and message headers
			i := rng.Intn(min(len(x), 12))
			x[i] = byte(rng.Intn(256))
		}
		return x
	}
	i := rng.Intn(len(fs))
	switch k := rng.Intn(10); {
	case k < 4:
		fs[i] = mutateBytes(fs[i])
	case k < 6:
		recs := tls13Records(fs[i])
		if len(recs) > 0 {
			x, y := rng.Intn(len(recs)), rng.Intn(len(recs))
			switch rng.Intn(3) {
			case 0:
				recs = append(recs[:x:x], recs[x+1:]...)
			case 1:
				recs = append(recs[:x+1:x+1], recs[x:]...)
			case 2:
				recs[x], recs[y] = recs[y], recs[x]
			}
			fs[i] = bytes.Join(recs, nil)
		}
	case k < 7:
		// a record moved to another flight, or a flight dropped or repeated
		j := rng.Intn(len(fs))
		switch rng.Intn(3) {
		case 0:
			recs := tls13Records(fs[i])
			if len(recs) > 0 {
				x := rng.Intn(len(recs))
				fs[j] = append(fs[j], recs[x]...)
				fs[i] = bytes.Join(append(recs[:x:x], recs[x+1:]...), nil)
			}
		case 1:
			fs = append(fs[:i:i], fs[i+1:]...)
		case 2:
			fs = append(fs[:i+1:i+1], fs[i:]...)
		}
	default:
		if tr.cKey != nil && len(fs) > 1 {
			fs[1] = tlsSrvMutateFinished(rng, tr, mutateBytes)
		} else {
			fs[i] = mutateBytes(fs[i])
		}
	}
	if len(fs) == 0 {
		fs = [][]byte{nil}
	}
	if rng.Intn(4) == 0 {
		dd[4] = strconv.Itoa(1 + rng.Intn(40))
	}
	if tr.tls12 {
		// a: the ECDHE key; b: handshake, records after, cert, key, sig
		parts := [][]byte{fs[0], nil, tr.sv[0], tr.sv[1], tr.sv[2]}
		if len(fs) > 1 {
			parts[1] = bytes.Join(fs[1:], nil)
		}
		return tr.key, tlsSrvPack(parts), nil, []byte(strings.Join(dd, " "))
	}
	return tlsSrvPack(fs), tlsSrvPack(tr.sv), tlsSrvPack(tr.hooks), []byte(strings.Join(dd, " "))
}

// tlsSrvMutateFinished decrypts the RFC trace's client Finished record,
// changes its plaintext (and now and then its inner type or padding),
// and seals it again, in one or two records.
func tlsSrvMutateFinished(rng *rand.Rand, tr tlsSrvTrace, mutate func([]byte) []byte) []byte {
	blk, _ := aes.NewCipher(tr.cKey)
	aead, _ := cipher.NewGCM(blk)
	rec := tr.flights[1]
	pt, err := aead.Open(nil, tls13Nonce(tr.cIV, 0), rec[5:], rec[:5])
	if err != nil {
		panic(err)
	}
	plain := mutate(append([]byte(nil), pt[:len(pt)-1]...))
	var out []byte
	parts := [][]byte{plain}
	if len(plain) > 1 && rng.Intn(3) == 0 {
		x := 1 + rng.Intn(len(plain)-1)
		parts = [][]byte{plain[:x], plain[x:]}
	}
	for seq, p := range parts {
		inner := append(append([]byte(nil), p...), 22)
		if rng.Intn(10) == 0 {
			inner[len(inner)-1] = byte(rng.Intn(256))
		}
		if rng.Intn(10) == 0 {
			inner = append(inner, make([]byte, rng.Intn(20))...)
		}
		hdr := []byte{23, 3, 3, 0, 0}
		binary.BigEndian.PutUint16(hdr[3:], uint16(len(inner)+16))
		out = append(out, aead.Seal(hdr, tls13Nonce(tr.cIV, uint64(seq)), inner, hdr)...)
	}
	return out
}

// tlsSrvErrText matches an error's text in a server's output: what a
// text only difference may change.
var tlsSrvErrText = regexp.MustCompile(`((local|remote) error: )?(tls|x509|crypto/[a-z]+): [^\n]*`)

// TestTLSServerReplayDiff feeds the TLS servers mutated client records
// (decision 0202's differential fuzzing, the server half: the recorded
// clients of the tls13_server and tls12_server goldens with bytes
// flipped, cut, inserted, records dropped, repeated, swapped or moved,
// flights dropped or repeated, the client's Finished altered under a
// valid protection, the bytes fed in pieces) and diffs what tls13srv.xo
// and tls12srv.xo do (LLVM and arm64) against the Go references (the Go
// backend): the bytes they send, their states, what they read, their
// errors. A difference in behaviour fails; one only in an error's text
// is counted. XO_TLSSRV_DIFF_N cases (default 3000) from
// XO_TLSSRV_DIFF_SEED (default 11); XO_TLSSRV_DIFF_ONLY=tls13srv or
// tls12 keeps one server's traces; XO_TLSSRV_DIFF_DUMP names a file the
// differing cases are appended to.
func TestTLSServerReplayDiff(t *testing.T) {
	if testing.Short() {
		t.Skip("builds a program with three backends")
	}
	if _, err := xotool.Clang(); err != nil {
		t.Skip("clang not found")
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "main.xo"), []byte(tlsSrvDiffProgram), 0o644); err != nil {
		t.Fatal(err)
	}
	mod := "module github.com/xo-lang/xo/test/tlssrvdiff\nedition preview\n"
	if err := os.WriteFile(filepath.Join(dir, "xo.mod"), []byte(mod), 0o644); err != nil {
		t.Fatal(err)
	}
	n := 3000
	if v, err := strconv.Atoi(os.Getenv("XO_TLSSRV_DIFF_N")); err == nil {
		n = v
	}
	seed := int64(11)
	if v, err := strconv.ParseInt(os.Getenv("XO_TLSSRV_DIFF_SEED"), 10, 64); err == nil {
		seed = v
	}
	traces := tlsSrvTraces(t)
	if only := os.Getenv("XO_TLSSRV_DIFF_ONLY"); only != "" {
		// the traces of one server: "tls13srv" or "tls12"
		var keep []tlsSrvTrace
		for _, tr := range traces {
			if strings.HasPrefix(tr.op, only) {
				keep = append(keep, tr)
			}
		}
		traces = keep
	}
	rng := rand.New(rand.NewSource(seed))
	var in strings.Builder
	var cases []string
	for i := 0; i < n; i++ {
		tr := traces[rng.Intn(len(traces))]
		a, b, c, d := tlsSrvMutate(rng, tr)
		in.WriteString(strings.Join([]string{tr.op, hex.EncodeToString(a), hex.EncodeToString(b), hex.EncodeToString(c), hex.EncodeToString(d)}, " ") + "\n")
		cases = append(cases, tr.name)
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
		cmd := exec.Command(res.Binaries[xotool.Host()])
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
	bad, texts := 0, 0
	states := map[string]int{}
	byTrace := map[string]int{}
	for i := 0; i < n; i++ {
		ref, _ := hex.DecodeString(outs[0][i])
		states[tlsSrvDiffState(string(ref))]++
		for b := 1; b < len(backends); b++ {
			if outs[b][i] == outs[0][i] {
				continue
			}
			got, _ := hex.DecodeString(outs[b][i])
			if tlsSrvErrText.ReplaceAllString(string(got), "ERR") == tlsSrvErrText.ReplaceAllString(string(ref), "ERR") {
				texts++
				if testing.Verbose() && texts <= 20 {
					t.Logf("case %d (%s): %s text differs\ngo: %s\n%s: %s", i, cases[i], backends[b], tlsSrvDiffState(string(ref)), backends[b], tlsSrvDiffState(string(got)))
				}
				continue
			}
			if bad < 5 {
				t.Errorf("case %d (%s): %s differs from go\ngo:\n%s\n%s:\n%s", i, cases[i], backends[b], ref, backends[b], got)
			}
			if dump := os.Getenv("XO_TLSSRV_DIFF_DUMP"); dump != "" {
				f, _ := os.OpenFile(dump, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
				f.WriteString(cases[i] + "\t" + strings.Split(in.String(), "\n")[i] + "\t" + string(ref) + "\t" + string(got) + "\n")
				f.Close()
			}
			byTrace[cases[i]+" "+backends[b]]++
			bad++
		}
	}
	t.Logf("%d cases (seed %d), %d text only differences, outcomes: %v", n, seed, texts, states)
	if bad > 0 {
		t.Errorf("%d differences: %v", bad, byTrace)
	}
}

// tlsSrvDiffState is a case's outcome: the last error's text, or done.
func tlsSrvDiffState(out string) string {
	last := "done"
	for _, m := range tlsSrvErrText.FindAllString(out, -1) {
		last = m
	}
	if len(last) > 90 {
		last = last[:90]
	}
	return last
}
