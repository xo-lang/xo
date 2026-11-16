//go:debug cryptocustomrand=1

// Command stdcorpus generates the std test corpora of
// internal/stdtest/testdata/corpus: Go's results on its own test inputs
// and on seeded random ones, as Xo tests of a std module's public API,
// which TestStdCorpora runs on every backend outside -short. The Go
// oracle stays as data whatever implements the module on the Go backend.
//
//	go run ./scripts/stdcorpus MODULE...   # rewrite the corpora of the modules
//	go run ./scripts/stdcorpus -check      # exit 1 when a corpus differs
//
// It also writes the fault corpus programs of
// internal/stdtest/testdata/faults (faults.go), which are programs rather
// than xo test files: their recorded behavior is TestSliceFaults's.
//
// Each corpus is deterministic for a Go version (the seed is fixed);
// regenerate with the Go version its header names.
package main

import (
	"bufio"
	"bytes"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync/atomic"
	"time"
	"unicode/utf8"
)

// generators are the corpora by module.
var generators = map[string]func() string{
	"archive":      archiveCorpus,
	"base32":       base32Corpus,
	"base64":       base64Corpus,
	"bufio":        bufioCorpus,
	"big":          bigCorpus,
	"bigfloat":     bfCorpus,
	"bigrat":       brCorpus,
	"binary":       binaryCorpus,
	"bytes":        bytesCorpus,
	"case":         caseCorpus,
	"charset":      charsetCorpus,
	"cli":          cliCorpus,
	"compress":     compressCorpus,
	"aead":         aeadCorpus,
	"adler32":      adler32Corpus,
	"crc64":        crc64Corpus,
	"x509":         x509Corpus,
	"x509create":   x509CreateCorpus,
	"x509keys":     x509KeysCorpus,
	"x509oid":      x509OIDCorpus,
	"x509policy":   x509PolicyCorpus,
	"crypto":       cryptoCorpus,
	"shake":        shakeCorpus,
	"pbkdf2":       pbkdf2Corpus,
	"gcmsizes":     gcmSizesCorpus,
	"ed25519opts":  ed25519OptsCorpus,
	"hashstate":    hashStateCorpus,
	"ecdsaraw":     ecdsaRawCorpus,
	"rsaparts":     rsaPartsCorpus,
	"rsaopts":      rsaOptsCorpus,
	"randbig":      randBigCorpus,
	"subtle":       subtleCorpus,
	"ciphermodes":  cipherModesCorpus,
	"hpke":         hpkeCorpus,
	"csv":          csvCorpus,
	"cut":          cutCorpus,
	"equalfold":    equalfoldCorpus,
	"fakeclock":    fakeclockCorpus,
	"http":         httpCorpus,
	"httpurl":      httpurlCorpus,
	"json":         jsonCorpus,
	"jsondecode":   jsonDecodeCorpus,
	"jsonencode":   jsonEncodeCorpus,
	"jsonstring":   jsonStringCorpus,
	"jsontoken":    jsonTokenCorpus,
	"lzw":          lzwCorpus,
	"gif":          gifCorpus,
	"mail":         mailCorpus,
	"metrics":      metricsCorpus,
	"mime":         mimeCorpus,
	"qp":           qpCorpus,
	"multipart":    multipartCorpus,
	"template":     templateCorpus,
	"strconv":      strconvCorpus,
	"time":         timeCorpus,
	"url":          urlCorpus,
	"textproto":    textprotoCorpus,
	"asn1":         asn1Corpus,
	"asn1fields":   asn1fieldsCorpus,
	"aesmodes":     aesModesCorpus,
	"ecdh":         ecdhCorpus,
	"mlkem":        mlkemCorpus,
	"random":       randomCorpus,
	"uuid":         uuidCorpus,
	"words":        wordsCorpus,
	"xml":          xmlCorpus,
	"xmlcodec":     xmlcodecCorpus,
	"netip":        netipCorpus,
	"path":         pathCorpus,
	"cbor":         cborCorpus,
	"protobuf":     protobufCorpus,
	"protoparse":   protoparseCorpus,
	"image":        imageCorpus,
	"jpeg":         jpegCorpus,
	"jpegenc":      jpegencCorpus,
	"png":          pngCorpus,
	"prelude":      preludeCorpus,
	"regexp":       regexpCorpus,
	"runes":        runesCorpus,
	"smtp":         smtpCorpus,
	"sql":          sqlCorpus,
	"strsearch":    strsearchCorpus,
	"strwhere":     whereCorpus,
	"trimchars":    trimcharsCorpus,
	"strings":      stringsCorpus,
	"tabwriter":    tabwriterCorpus,
	"backoff":      backoffCorpus,
	"deque":        dequeCorpus,
	"heap":         heapCorpus,
	"brotli":       brotliCorpus,
	"cron":         cronCorpus,
	"cronfuzz":     cronfuzzCorpus,
	"rate":         rateCorpus,
	"ratefuzz":     ratefuzzCorpus,
	"heapops":      heapopsCorpus,
	"otp":          otpCorpus,
	"cookiejar":    cookiejarCorpus,
	"norm":         normCorpus,
	"idna":         idnaCorpus,
	"bcrypt":       bcryptCorpus,
	"xcrypto":      xcryptoCorpus,
	"htmltok":      htmltokCorpus,
	"htmlparse":    htmlparseCorpus,
	"protodesc":    protodescCorpus,
	"protowkt":     protowktCorpus,
	"protojson":    protojsonCorpus,
	"yaml":         yamlCorpus,
	"yamlfuzz":     yamlfuzzCorpus,
	"jwt":          jwtCorpus,
	"jwe":          jweCorpus,
	"pkcs12":       pkcs12Corpus,
	"amqp":         amqpCorpus,
	"toml":         tomlCorpus,
	"term":         termCorpus,
	"sqlite":       sqliteCorpus,
	"grpc":         grpcCorpus,
	"otlp":         otlpCorpus,
	"timego":       timegoCorpus,
	"sse":          sseCorpus,
	"jsonrpc":      jsonrpcCorpus,
	"httpproxy":    httpproxyCorpus,
	"publicsuffix": publicsuffixCorpus,
}

// checkOnly is -check: generators write no files of their own (hpke's
// pinned outputs).
var checkOnly bool

// noFMA runs this program again with Go's fused multiply-add off
// (-gcflags=all=-d=fmahash=n) and returns its exit status, or -1 when it
// is that run (XO_STDCORPUS_NOFMA set). Go may fuse x*y+z on arm64, in
// the generators and in Go's own math and math/rand/v2, and not on amd64;
// Xo never fuses (decision 0287), so every Go answer here is computed
// unfused, the same on every platform (#116, 2026-11-16: random fuzz
// seeds 17 and 87 drew one ulp apart from Xo on arm64 Go; every committed
// corpus was unchanged under the flag). gotchas/go-oracle-fma-arm64.md.
// noFMAEnv is env for the run with fusion off: XO_STDCORPUS_NOFMA set and
// the flag in GOFLAGS, so it reaches every `go run` of an oracle module
// the generators start too (#147: those are separate go invocations, and
// a -gcflags on this one's command line stopped at this program).
func noFMAEnv(env []string) []string {
	out := []string{"XO_STDCORPUS_NOFMA=1"}
	gf := ""
	for _, e := range env {
		if v, ok := strings.CutPrefix(e, "GOFLAGS="); ok {
			gf = v
			continue
		}
		out = append(out, e)
	}
	return append(out, "GOFLAGS="+strings.TrimSpace(gf+" "+noFMAFlag))
}

// noFMAFlag turns Go's fused multiply-add off in every package built.
const noFMAFlag = "-gcflags=all=-d=fmahash=n"

// mulAdd is x*y+z as the compiler builds it: one fused multiply-add on
// arm64 unless fusion is off.
//
//go:noinline
func mulAdd(x, y, z float64) float64 { return x*y + z }

// fmaVals are inputs whose product rounds away 2^-60: unfused, x*y+z is
// 0; fused, 2^-60. Variables, so nothing folds them.
var fmaVals = [3]float64{1 + 1.0/(1<<30), 1 + 1.0/(1<<30), -(1 + 1.0/(1<<29))}

// fused reports whether this binary was built with fused multiply-add.
func fused() bool { return mulAdd(fmaVals[0], fmaVals[1], fmaVals[2]) != 0 }

func noFMA() int {
	if os.Getenv("XO_STDCORPUS_NOFMA") != "" {
		// the guard of the rule (agent-rules.md, gotchas/go-oracle-fma-arm64.md):
		// a corpus is never written by a build that fuses
		if fused() {
			fmt.Fprintln(os.Stderr, "stdcorpus: built with fused multiply-add although XO_STDCORPUS_NOFMA is set (GOFLAGS lost "+noFMAFlag+"?): no corpus written")
			return 2
		}
		return -1
	}
	cmd := exec.Command("go", append([]string{"run", "./scripts/stdcorpus"}, os.Args[1:]...)...)
	cmd.Env = noFMAEnv(os.Environ())
	cmd.Stdin, cmd.Stdout = os.Stdin, os.Stdout
	// the compiler reports each fusion it declined, and the package of
	// each report: those lines are dropped, the rest of stderr kept
	pr, pw, err := os.Pipe()
	if err != nil {
		fmt.Fprintln(os.Stderr, "stdcorpus:", err)
		return 2
	}
	cmd.Stderr = pw
	done := make(chan struct{})
	go func() {
		sc := bufio.NewScanner(pr)
		sc.Buffer(make([]byte, 1<<20), 1<<20)
		for sc.Scan() {
			l := sc.Text()
			if strings.Contains(l, "fmahash triggered") || strings.Contains(l, "[bisect-match ") || (strings.HasPrefix(l, "# ") && !strings.Contains(l[2:], " ")) {
				continue
			}
			fmt.Fprintln(os.Stderr, l)
		}
		close(done)
	}()
	err = cmd.Run()
	pw.Close()
	<-done
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			return ee.ExitCode()
		}
		fmt.Fprintln(os.Stderr, "stdcorpus:", err)
		return 2
	}
	return 0
}

// beat writes a heartbeat line to w every `every` until stop is closed:
// how many of total corpora are done and which one runs (#147, lead
// 2026-11-16, alert #4808: a -check of every corpus prints nothing for 10
// to 16 minutes, and xw's hang check flagged it as a quiet job; a line
// naming a heartbeat in the job's log keeps it off).
func beat(w io.Writer, every time.Duration, total int, done *atomic.Int64, cur *atomic.Value, stop <-chan struct{}) {
	start := time.Now()
	t := time.NewTicker(every)
	defer t.Stop()
	for {
		select {
		case <-stop:
			return
		case <-t.C:
			m, _ := cur.Load().(string)
			fmt.Fprintf(w, "stdcorpus: heartbeat: %d of %d corpora done, %s running, %s\n", done.Load(), total, m, time.Since(start).Round(time.Second))
		}
	}
}

func main() {
	if rc := noFMA(); rc >= 0 {
		os.Exit(rc)
	}
	check := flag.Bool("check", false, "exit 1 when a corpus differs from its generator's output")
	flag.Parse()
	checkOnly = *check
	// net/mail and time read the local zone: every corpus is made in UTC
	time.Local = time.UTC
	mods := flag.Args()
	if len(mods) == 0 {
		for m := range generators {
			mods = append(mods, m)
		}
	}
	sort.Strings(mods)
	// XO_STDCORPUS_BEAT_S (60; 0 none): the heartbeat's interval
	var done atomic.Int64
	var cur atomic.Value
	cur.Store("")
	every := 60 * time.Second
	if v, err := strconv.Atoi(os.Getenv("XO_STDCORPUS_BEAT_S")); err == nil {
		every = time.Duration(v) * time.Second
	}
	if every > 0 && len(mods) > 1 {
		stop := make(chan struct{})
		defer close(stop)
		go beat(os.Stderr, every, len(mods), &done, &cur, stop)
	}
	bad := false
	// the fault corpora are programs rather than xo test files (faults.go)
	if writeFaultCorpora(*check) {
		bad = true
	}
	for _, m := range mods {
		cur.Store(m)
		gen, ok := generators[m]
		if !ok {
			fmt.Fprintf(os.Stderr, "stdcorpus: no corpus for %q\n", m)
			os.Exit(2)
		}
		path := filepath.Join("internal", "stdtest", "testdata", "corpus", m, m+"_test.xo")
		if out := os.Getenv("XO_STDCORPUS_OUT"); out != "" {
			// a scratch copy (fresh fuzz seeds), never the committed corpus
			path = filepath.Join(out, m+"_test.xo")
		}
		text := []byte(gen())
		old, oldErr := os.ReadFile(path)
		// the header names the Go that generated the corpus; a different
		// patch release alone is not staleness (81 corpora said go1.26.1 and
		// 10 go1.26.11 on 2026-11-15, so -check failed nearly every corpus
		// on either), and regenerating keeps the old header
		same := oldErr == nil && sameButGoVersion(old, text)
		done.Add(1)
		if *check {
			if !same {
				fmt.Fprintf(os.Stderr, "stdcorpus: %s is stale: run go run ./scripts/stdcorpus %s\n", path, m)
				bad = true
			}
			continue
		}
		if same {
			continue
		}
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			fmt.Fprintln(os.Stderr, "stdcorpus:", err)
			os.Exit(1)
		}
		if err := os.WriteFile(path, text, 0o644); err != nil {
			fmt.Fprintln(os.Stderr, "stdcorpus:", err)
			os.Exit(1)
		}
	}
	if bad {
		os.Exit(1)
	}
}

// goVersionRE is a Go release as a header names it: `(go1.26.11)`.
var goVersionRE = regexp.MustCompile(`^\(go[0-9][^)]*\)`)

// sameButGoVersion reports whether corpus text old equals the generated
// text gen but for the Go release gen's headers name (runtime.Version()
// where header put it, once a header; old may name another release in
// each place).
func sameButGoVersion(old, gen []byte) bool {
	v := []byte("(" + runtime.Version() + ")")
	for {
		i := bytes.Index(gen, v)
		if i < 0 {
			return bytes.Equal(old, gen)
		}
		if len(old) < i || !bytes.Equal(old[:i], gen[:i]) {
			return false
		}
		loc := goVersionRE.FindIndex(old[i:])
		if loc == nil {
			return false
		}
		old, gen = old[i+loc[1]:], gen[i+len(v):]
	}
}

// header is a corpus file's first comment lines.
func header(what string) string {
	return "// Generated by scripts/stdcorpus from " + what + " (" + runtime.Version() + ").\n// Do not edit: regenerate with go run ./scripts/stdcorpus.\n"
}

// xq is s as an Xo string literal.
func xq(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range s {
		switch {
		case r == '"':
			b.WriteString(`\"`)
		case r == '\\':
			b.WriteString(`\\`)
		case r == '$':
			b.WriteString(`\$`)
		case r == '\n':
			b.WriteString(`\n`)
		case r == '\r':
			b.WriteString(`\r`)
		case r == '\t':
			b.WriteString(`\t`)
		case r < 0x20 || r == 0x7f:
			fmt.Fprintf(&b, `\u{%x}`, r)
		default:
			b.WriteRune(r)
		}
	}
	b.WriteByte('"')
	return b.String()
}

// xbytes is b as an Xo Bytes literal.
func xbytes(b []byte) string {
	var s strings.Builder
	s.WriteString(`b"`)
	for _, c := range b {
		if c >= 0x20 && c < 0x7f && c != '"' && c != '\\' && c != '$' {
			s.WriteByte(c)
		} else {
			fmt.Fprintf(&s, `\x%02x`, c)
		}
	}
	s.WriteByte('"')
	return s.String()
}

// textOf is std/rt's textOf: U+FFFD for each byte of an invalid sequence
// (decision 0259).
func textOf(s string) string {
	if utf8.ValidString(s) {
		return s
	}
	var b strings.Builder
	for i := 0; i < len(s); {
		r, n := utf8.DecodeRuneInString(s[i:])
		if r == utf8.RuneError && n == 1 {
			b.WriteString("�")
		} else {
			b.WriteString(s[i : i+n])
		}
		i += n
	}
	return b.String()
}
