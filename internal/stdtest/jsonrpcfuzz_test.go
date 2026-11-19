package stdtest

// A differential of std/jsonrpc against github.com/modelcontextprotocol/
// go-sdk v1.8.0's jsonrpc package (task #115): random message texts, valid
// and not (members in any order and duplicated, whitespace, escapes in
// strings, lone surrogates, ids of every JSON type and of numbers with
// fractions and exponents, methods and versions of the wrong type, both
// result and error, errors missing a code or message, extra members,
// truncated texts, trailing bytes), decoded by both, rendered as the
// corpus renders them (scripts/stdcorpus/jsonrpcoracle's render, the
// corpus test's show); and for texts whose params, result and data are
// compact with sorted keys (the SDK copies them raw), the bytes of the
// SDK's EncodeMessage against jsonrpc.encode. The oracle is
// jsonrpcoracle's lines mode.
//
//   XO_JSONRPCFUZZ_SEED (2), _N (20000), _BACKENDS (arm64,llvm), _DUMP (a
//   file prefix: the differing cases are written to PREFIX.BACKEND)

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/xo-lang/xo/internal/hostile"
	"github.com/xo-lang/xo/internal/testtmp"
	"github.com/xo-lang/xo/internal/xotool"
)

func jrfPick(r *rand.Rand, xs ...string) string { return xs[r.Intn(len(xs))] }

// jrfCanonVal: a compact value with sorted keys, which the SDK copies as
// it is and std/jsonrpc writes the same.
func jrfCanonVal(r *rand.Rand, depth int) string {
	if depth > 2 {
		return jrfPick(r, `1`, `"s"`, `null`, `true`)
	}
	switch r.Intn(10) {
	case 0:
		return jrfPick(r, `0`, `1`, `-2`, `123456789`, `-9007199254740991`)
	case 1:
		return jrfPick(r, `0.5`, `-2.25`, `1.5`)
	case 2:
		return jrfPick(r, `""`, `"s"`, `"é"`, `"<&>"`, `"tab\there"`, `"q\"x"`, `"\\"`, `"日本"`)
	case 3:
		return jrfPick(r, `true`, `false`, `null`)
	case 4, 5:
		var xs []string
		for i := r.Intn(4); i > 0; i-- {
			xs = append(xs, jrfCanonVal(r, depth+1))
		}
		return "[" + strings.Join(xs, ",") + "]"
	default:
		ks := []string{"a", "b", "c", "id", "z"}
		var xs []string
		for _, k := range ks {
			if r.Intn(2) == 0 {
				xs = append(xs, `"`+k+`":`+jrfCanonVal(r, depth+1))
			}
		}
		return "{" + strings.Join(xs, ",") + "}"
	}
}

// jrfAnyVal: any value, also ones the SDK copies with their spelling.
func jrfAnyVal(r *rand.Rand) string {
	switch r.Intn(6) {
	case 0:
		return jrfPick(r, `1e3`, `1E2`, `1.0`, `-0`, `0.1`, `1e-7`, `25e-1`, `1.5e300`)
	case 1:
		return jrfPick(r, `{"b":1,"a":2}`, `{ "a" : [ 1 , 2 ] }`, `{"a":1,"a":2}`, `"é"`, `"😀"`, `"\ud800"`, `" "`, `"\/"`)
	default:
		return jrfCanonVal(r, 0)
	}
}

func jrfID(r *rand.Rand) string {
	return jrfPick(r, `1`, `0`, `-7`, `42`, `"x"`, `""`, `"id-9"`, `null`, `1.5`, `-1.9`, `1e3`, `2.0`, `true`, `{}`, `[]`, `"é"`,
		`999999999999999`, `-0`, `0.0001`)
}

func jrfError(r *rand.Rand) string {
	var f []string
	if r.Intn(8) > 0 {
		f = append(f, `"code":`+jrfPick(r, `-32601`, `-32000`, `5`, `0`, `-32700`, `1.5`, `"x"`, `null`))
	}
	if r.Intn(8) > 0 {
		f = append(f, `"message":`+jrfPick(r, `"m"`, `""`, `"no such method"`, `1`, `null`, `"é\n"`))
	}
	if r.Intn(3) == 0 {
		f = append(f, `"data":`+jrfCanonVal(r, 0))
	}
	if r.Intn(10) == 0 {
		return jrfPick(r, `null`, `"boom"`, `[]`, `5`)
	}
	r.Shuffle(len(f), func(i, j int) { f[i], f[j] = f[j], f[i] })
	return "{" + strings.Join(f, ",") + "}"
}

// jrfGen: a message text and whether its values are canonical (encode is
// compared for those).
func jrfGen(r *rand.Rand) (string, bool) {
	canon := true
	val := func() string {
		if r.Intn(4) == 0 {
			canon = false
			return jrfAnyVal(r)
		}
		return jrfCanonVal(r, 0)
	}
	var f []string
	switch r.Intn(12) {
	case 0:
	case 1:
		f = append(f, `"jsonrpc":`+jrfPick(r, `"1.0"`, `2`, `null`, `"2.0 "`, `"2.00"`))
	default:
		f = append(f, `"jsonrpc":"2.0"`)
	}
	kind := r.Intn(10)
	if kind < 5 {
		// a request
		if r.Intn(10) > 0 {
			f = append(f, `"method":`+jrfPick(r, `"m"`, `"tools/call"`, `"initialize"`, `""`, `"é"`, `"a\nb"`, `"A"`, `1`, `null`, `"rpc.x"`))
		}
		if r.Intn(3) > 0 {
			f = append(f, `"id":`+jrfID(r))
		}
		if r.Intn(3) > 0 {
			f = append(f, `"params":`+val())
		}
	} else if kind < 9 {
		// a response
		if r.Intn(8) > 0 {
			f = append(f, `"id":`+jrfID(r))
		}
		switch r.Intn(5) {
		case 0:
			f = append(f, `"error":`+jrfError(r))
		case 1:
			f = append(f, `"error":`+jrfError(r), `"result":`+val())
		case 2:
			f = append(f, `"error":null`, `"result":`+val())
		default:
			f = append(f, `"result":`+val())
		}
	} else {
		f = append(f, `"id":`+jrfID(r), `"method":"m"`, `"result":1`)
	}
	if r.Intn(10) == 0 {
		f = append(f, `"extra":`+jrfCanonVal(r, 0))
	}
	if r.Intn(15) == 0 && len(f) > 0 {
		f = append(f, f[r.Intn(len(f))])
	}
	r.Shuffle(len(f), func(i, j int) { f[i], f[j] = f[j], f[i] })
	sep := ","
	if r.Intn(6) == 0 {
		sep = " ,\n\t"
		canon = false
	}
	t := "{" + strings.Join(f, sep) + "}"
	switch r.Intn(25) {
	case 0:
		// cut on a rune boundary: std/jsonrpc decodes a Str
		t = t[:r.Intn(len(t))]
		for !utf8.ValidString(t) {
			t = t[:len(t)-1]
		}
	case 1:
		t += jrfPick(r, " ", "\n", "x", "{}", ",")
	case 2:
		t = jrfPick(r, `[]`, `[`+t+`]`, `1`, `"x"`, `null`, ``, ` `)
	case 3:
		t = " " + t + " "
	}
	return t, canon
}

func jrfOracle(t *testing.T, lines []string) []string {
	root, err := xotool.Root()
	if err != nil {
		t.Fatal(err)
	}
	cmd := testtmp.Guard(exec.Command("go", "run", ".", "lines"))
	cmd.Dir = filepath.Join(root, "scripts", "stdcorpus", "jsonrpcoracle")
	cmd.Stdin = strings.NewReader(strings.Join(lines, "\n") + "\n")
	var out, errb bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errb
	if err := cmd.Run(); err != nil {
		t.Fatalf("jsonrpcoracle lines: %v\n%s", err, errb.String())
	}
	return jrfBlocks(out.String())
}

func jrfBlocks(s string) []string {
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

func jrfBuild(t *testing.T, work, backend string) string {
	root, err := xotool.Root()
	if err != nil {
		t.Fatal(err)
	}
	xo, err := xotool.Path()
	if err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(work, "xo-jsonrpc-"+backend)
	args := []string{"build"}
	if backend != "" {
		args = append(args, "--backend="+backend)
	}
	args = append(args, "-o", bin, filepath.Join(root, "scripts", "jsonrpcdiff", "xo"))
	xb := testtmp.Guard(exec.Command(xo, args...))
	xb.Env = append(os.Environ(), "XO_STD="+filepath.Join(root, "std"))
	if out, err := xb.CombinedOutput(); err != nil {
		t.Fatalf("xo build --backend=%s: %v\n%s", backend, err, out)
	}
	return bin
}

// jrfHostile is the k-th hostile message (task #352), chosen by the case
// index so the random stream is unchanged: an id or an error code from the
// hostile Ints and digit runs past Int, params nested to a hostile depth
// (encoding/json stops at 10000), or a method a long run.
func jrfHostile(k int) string {
	var ns []string
	for _, v := range hostile.Ints() {
		ns = append(ns, strconv.FormatInt(v, 10))
	}
	ns = append(ns, hostile.Digits()...)
	n := ns[(k/4)%len(ns)]
	d := hostile.Depths()[(k/4)%len(hostile.Depths())]
	switch k % 4 {
	case 0:
		return `{"jsonrpc":"2.0","id":` + n + `,"method":"m"}`
	case 1:
		return `{"jsonrpc":"2.0","id":1,"error":{"code":` + n + `,"message":"m"}}`
	case 2:
		return `{"jsonrpc":"2.0","id":1,"method":"m","params":` + hostile.Nest("[", "]", d) + `}`
	}
	return `{"jsonrpc":"2.0","method":"` + strings.Repeat("m", d) + `"}`
}

func TestJsonrpcFuzz(t *testing.T) {
	if testing.Short() {
		t.Skip("the jsonrpc fuzz builds the Xo client")
	}
	seed := int64(envInt(t, "XO_JSONRPCFUZZ_SEED", 2))
	n := envInt(t, "XO_JSONRPCFUZZ_N", 20000)
	backends := envBackends("XO_JSONRPCFUZZ_BACKENDS", []string{"arm64", "llvm"})
	r := rand.New(rand.NewSource(seed))
	lines := make([]string, n)
	texts := make([]string, n)
	for i := range lines {
		text, canon := jrfGen(r)
		if i%40 == 39 {
			text, canon = jrfHostile(i/40), false
		}
		mode := "d"
		if canon {
			mode = "e"
		}
		texts[i] = text
		lines[i] = mode + " x" + hex.EncodeToString([]byte(text))
	}
	want := jrfOracle(t, lines)
	if len(want) != n {
		t.Fatalf("the oracle: %d results for %d cases", len(want), n)
	}
	errs := 0
	for _, w := range want {
		if strings.HasPrefix(w, "error") {
			errs++
		}
	}
	t.Logf("seed %d: %d cases, %d of them do not decode in Go", seed, n, errs)
	work, err := testtmp.Dir("xo-jsonrpcfuzz-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(work)
	for _, b := range backends {
		bin := jrfBuild(t, work, b)
		cmd := testtmp.Guard(exec.Command(bin))
		cmd.Stdin = strings.NewReader(strings.Join(lines, "\n") + "\n")
		var out, errb bytes.Buffer
		cmd.Stdout = &out
		cmd.Stderr = &errb
		if err := cmd.Run(); err != nil {
			t.Fatalf("%s: %v\n%s", bin, err, myfTail(errb.String(), 20))
		}
		got := jrfBlocks(out.String())
		if len(got) != n {
			t.Fatalf("%s: %d results for %d cases", b, len(got), n)
		}
		bad := 0
		var dump strings.Builder
		for i := 0; i < n; i++ {
			if got[i] == want[i] {
				continue
			}
			fmt.Fprintf(&dump, "%q\t%q\t%q\n", texts[i], got[i], want[i])
			if bad < 10 {
				t.Errorf("%s case %d (seed %d): %q\ngot\n%swant\n%s", b, i, seed, texts[i], got[i], want[i])
			}
			bad++
		}
		if p := os.Getenv("XO_JSONRPCFUZZ_DUMP"); p != "" && bad > 0 {
			// every differing case, text, got and want quoted, for sorting by hand
			os.WriteFile(p+"."+b, []byte(dump.String()), 0o644)
		}
		if bad > 0 {
			t.Fatalf("%s: %d of %d cases differ (seed %d)", b, bad, n, seed)
		}
		t.Logf("%s: %d cases equal the Go SDK's (seed %d)", b, n, seed)
	}
}
