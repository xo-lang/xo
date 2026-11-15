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
	"bytes"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
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
	"multipart":    multipartCorpus,
	"template":     templateCorpus,
	"strconv":      strconvCorpus,
	"time":         timeCorpus,
	"url":          urlCorpus,
	"textproto":    textprotoCorpus,
	"asn1":         asn1Corpus,
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
	"cron":         cronCorpus,
	"rate":         rateCorpus,
	"heapops":      heapopsCorpus,
	"otp":          otpCorpus,
	"cookiejar":    cookiejarCorpus,
	"norm":         normCorpus,
	"idna":         idnaCorpus,
	"bcrypt":       bcryptCorpus,
	"htmltok":      htmltokCorpus,
	"htmlparse":    htmlparseCorpus,
	"protodesc":    protodescCorpus,
	"protowkt":     protowktCorpus,
	"protojson":    protojsonCorpus,
	"yaml":         yamlCorpus,
	"yamlfuzz":     yamlfuzzCorpus,
	"jwt":          jwtCorpus,
	"pkcs12":       pkcs12Corpus,
	"amqp":         amqpCorpus,
	"grpc":         grpcCorpus,
	"otlp":         otlpCorpus,
	"timego":       timegoCorpus,
	"sse":          sseCorpus,
	"jsonrpc":      jsonrpcCorpus,
	"httpproxy":    httpproxyCorpus,
	"publicsuffix": publicsuffixCorpus,
}

func main() {
	check := flag.Bool("check", false, "exit 1 when a corpus differs from its generator's output")
	flag.Parse()
	// net/mail and time read the local zone: every corpus is made in UTC
	time.Local = time.UTC
	mods := flag.Args()
	if len(mods) == 0 {
		for m := range generators {
			mods = append(mods, m)
		}
	}
	sort.Strings(mods)
	bad := false
	// the fault corpora are programs rather than xo test files (faults.go)
	if writeFaultCorpora(*check) {
		bad = true
	}
	for _, m := range mods {
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
