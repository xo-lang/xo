package stdtest

// TestMultipartFuzz (task #90): std/multipart's parse and parse_form
// against Go's mime/multipart Reader (NextPart with each part read whole,
// and ReadForm with http's 32 MB) on seeded bodies. A body is an optional
// preamble (lines that look like the boundary and are not), parts and an
// optional final boundary and epilogue. A delimiter ends with CRLF or LF,
// after spaces and tabs now and then. A part has headers
// (Content-Disposition form-data or not, names quoted and bare, file
// names with directories, RFC 2231 file names, Content-Type,
// quoted-printable transfer encoding, other headers, folded lines, lines
// without a colon, bytes that are not UTF-8, repeated headers, none) and
// data (text, binary, the boundary inside a line or after CR alone, lines
// that start like the boundary, quoted-printable soft breaks and bad
// escapes, sometimes past the 4,096 byte buffer, a boundary straddling it).
// A quarter of the bodies are then cut short or have a byte changed, and
// one in 200 has more than 1,000 parts (ReadForm's limit). Each case is
// read both ways; the parts, form values and files (texts as std's Str
// holds them: each byte of an invalid sequence U+FFFD), or the error,
// must be the same.
//
//   XO_MULTIPARTFUZZ_SEED (default 2), XO_MULTIPARTFUZZ_N (500),
//   XO_MULTIPARTFUZZ_BACKENDS (arm64,llvm), XO_MULTIPARTFUZZ_DUMP=i

import (
	"bytes"
	"fmt"
	"io"
	"math/rand"
	"mime"
	"mime/multipart"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/xo-lang/xo/internal/testtmp"
	"github.com/xo-lang/xo/internal/xotool"
)

type mpfCase struct {
	form     bool
	boundary string
	body     []byte
	note     string
}

var mpfBoundaries = []string{"b", "XyZ", "----WebKitFormBoundary7MA4YWxkTrZu0gW", "a b", "=_=", "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef", "--", "q.r"}

func mpfPick(r *rand.Rand, xs []string) string { return xs[r.Intn(len(xs))] }

func mpfNL(r *rand.Rand) string {
	if r.Intn(6) == 0 {
		return "\n"
	}
	return "\r\n"
}

func mpfDisposition(r *rand.Rand) string {
	names := []string{`"f"`, `"field 2"`, `f3`, `""`, `"caf\u00e9"`, `"a\"b"`, `"x`}
	files := []string{`"a.txt"`, `"dir/sub/b.bin"`, `"C:\\win\\c.txt"`, `""`, `"/"`, `"."`, `"..\\..\\x"`, `"\u00fc.txt"`}
	d := mpfPick(r, []string{"form-data", "form-data", "form-data", "attachment", "FORM-DATA", "inline"})
	if r.Intn(8) != 0 {
		d += "; name=" + mpfPick(r, names)
	}
	switch r.Intn(5) {
	case 0, 1:
		d += "; filename=" + mpfPick(r, files)
	case 2:
		d += "; filename*=UTF-8''%e2%82%ac%20rates.txt"
	}
	if r.Intn(20) == 0 {
		d += "; name=dup"
	}
	return d
}

func mpfQP(r *rand.Rand) []byte {
	pieces := []string{"plain text", "=41=42", "=\r\n", "=\n", "=ZZ", "=4", "caf=C3=A9", "trailing   ", "=3D", "a=\r\nb", "\r\n", "line\n"}
	var b bytes.Buffer
	for k := 1 + r.Intn(8); k > 0; k-- {
		b.WriteString(mpfPick(r, pieces))
	}
	return b.Bytes()
}

func mpfData(r *rand.Rand, boundary string) []byte {
	var b bytes.Buffer
	switch r.Intn(10) {
	case 0:
		// past the 4,096 byte buffer, the boundary near it
		n := 4000 + r.Intn(300)
		for b.Len() < n {
			b.WriteString("0123456789abcdefghijklmnopqrstuvwxyz\r\n")
		}
		b.Truncate(n)
		return b.Bytes()
	case 1:
		for k := 0; k < 64; k++ {
			b.WriteByte(byte(r.Intn(256)))
		}
		return b.Bytes()
	}
	pieces := []string{"hello", "", "\r", "\n", "\r\n", "--" + boundary + "x", "x--" + boundary, "\r--" + boundary, "\n--" + boundary + "--x", "\xff\xfe", "caf\u00e9", "-", "--", " --" + boundary}
	for k := r.Intn(7); k > 0; k-- {
		b.WriteString(mpfPick(r, pieces))
	}
	return b.Bytes()
}

func mpfHeaders(r *rand.Rand, nl string) (string, bool) {
	var hs []string
	qp := false
	if r.Intn(10) != 0 {
		hs = append(hs, "Content-Disposition: "+mpfDisposition(r))
	}
	for k := r.Intn(4); k > 0; k-- {
		switch r.Intn(10) {
		case 0:
			hs = append(hs, "Content-Type: "+mpfPick(r, []string{"text/plain", "application/octet-stream", "text/plain; charset=utf-8", "image/png"}))
		case 1:
			hs = append(hs, mpfPick(r, []string{"Content-Transfer-Encoding: quoted-printable", "content-transfer-encoding: QUOTED-PRINTABLE", "Content-Transfer-Encoding: base64"}))
			qp = true
		case 2:
			hs = append(hs, "X-Folded: first"+nl+" second"+nl+"\tthird")
		case 3:
			hs = append(hs, "nocolon")
		case 4:
			hs = append(hs, "X-Bin: \xff\xfe v")
		case 5:
			hs = append(hs, "x-lower: v", "X-LOWER: w")
		case 6:
			hs = append(hs, "Bad Key: v")
		case 7:
			hs = append(hs, "X-Empty:")
		case 8:
			hs = append(hs, " leading: space")
		default:
			hs = append(hs, "X-A: "+strconv.Itoa(r.Intn(100)))
		}
	}
	var b strings.Builder
	for _, h := range hs {
		b.WriteString(h + nl)
	}
	return b.String(), qp
}

func mpfGen(r *rand.Rand) mpfCase {
	c := mpfCase{form: r.Intn(2) == 0, boundary: mpfPick(r, mpfBoundaries)}
	B := c.boundary
	var b bytes.Buffer
	if r.Intn(4) == 0 {
		b.WriteString(mpfPick(r, []string{"preamble" + "\r\n", "--" + B + "x\r\n", "-" + B + "\r\n", "\r\n", "--" + B + " junk\r\n"}))
	}
	parts := 1 + r.Intn(4)
	if r.Intn(10) == 0 {
		parts = 0
	}
	if r.Intn(200) == 0 {
		parts = 1001 + r.Intn(3)
		c.note = "many parts"
	}
	for k := 0; k < parts; k++ {
		nl := mpfNL(r)
		if k > 0 {
			b.WriteString(nl)
		}
		b.WriteString("--" + B)
		if r.Intn(10) == 0 {
			b.WriteString(mpfPick(r, []string{" ", "\t", "  \t"}))
		}
		b.WriteString(nl)
		if parts > 1000 {
			b.WriteString(fmt.Sprintf("Content-Disposition: form-data; name=\"p%d\"%s%s%d", k, nl, nl, k))
			continue
		}
		hs, qp := mpfHeaders(r, nl)
		b.WriteString(hs)
		b.WriteString(nl)
		if qp && r.Intn(3) != 0 {
			b.Write(mpfQP(r))
		} else {
			b.Write(mpfData(r, B))
		}
	}
	if r.Intn(8) != 0 {
		b.WriteString(mpfNL(r) + "--" + B + "--")
		switch r.Intn(4) {
		case 0:
			b.WriteString("\r\nepilogue\r\n")
		case 1:
			b.WriteString("\r\n")
		case 2:
			b.WriteString(" \t\r\n--" + B + "\r\nX: y\r\n\r\nafter")
		}
	}
	c.body = b.Bytes()
	if r.Intn(4) == 0 && len(c.body) > 0 {
		if r.Intn(2) == 0 {
			c.note = strings.TrimSpace(c.note + " cut")
			c.body = c.body[:r.Intn(len(c.body))]
		} else {
			c.note = strings.TrimSpace(c.note + " byte")
			i := r.Intn(len(c.body))
			c.body[i] = mpfPick(r, []string{"\r", "\n", "-", ":", " ", "\xff", "=", "x"})[0]
		}
	}
	return c
}

// mpfText: b as std's Str holds outside bytes, each byte of an invalid
// sequence U+FFFD (decision 0259).
func mpfText(s string) string {
	if utf8.ValidString(s) {
		return s
	}
	var b strings.Builder
	for i := 0; i < len(s); {
		r, n := utf8.DecodeRuneInString(s[i:])
		b.WriteRune(r)
		i += n
	}
	return b.String()
}

func mpfQuote(s string) string { return strconv.Quote(mpfText(s)) }

func mpfPart(b *strings.Builder, tag, name, filename string, h map[string][]string, data []byte) {
	f := "-"
	if filename != "" {
		f = mpfQuote(filename)
	}
	fmt.Fprintf(b, "%s %s %s\n", tag, mpfQuote(name), f)
	keys := make([]string, 0, len(h))
	for k := range h {
		keys = append(keys, mpfText(k))
	}
	sort.Strings(keys)
	for _, k := range keys {
		fmt.Fprintf(b, "h %s: %s\n", k, mpfQuote(h[k][0]))
	}
	fmt.Fprintf(b, "data %x\n", data)
}

// mpfNames: the part's form name and file name as std/multipart has
// them. Its headers are text (each byte of an invalid sequence U+FFFD,
// decision 0259), and std/mime leaves out a parameter value that is not
// UTF-8 (one RFC 2231 decodes so; TestMimeFuzz), so a name or file name
// is Go's of the header as text, "" when the value is not UTF-8; ok is
// false when that is not Go's.
func mpfNames(p *multipart.Part) (string, string, bool) {
	gname, gfile := p.FormName(), p.FileName()
	disp, params, err := mime.ParseMediaType(mpfText(p.Header.Get("Content-Disposition")))
	if err != nil {
		return "", "", gname == "" && gfile == ""
	}
	name, file := "", ""
	if v := params["name"]; disp == "form-data" && utf8.ValidString(v) {
		name = v
	}
	if v := params["filename"]; v != "" && utf8.ValidString(v) {
		file = filepath.Base(v)
	}
	return name, file, name == mpfText(gname) && file == mpfText(gfile)
}

// mpfGo: Go's reading of case c, as the Xo client prints it; "skip" for
// a form with a name or file name that is not UTF-8 (std/mime's rule
// above changes which parts ReadForm keeps and how).
func mpfGo(c mpfCase) string {
	var b strings.Builder
	r := multipart.NewReader(bytes.NewReader(c.body), c.boundary)
	if !c.form {
		var parts strings.Builder
		n := 0
		for {
			p, err := r.NextPart()
			if err == io.EOF {
				break
			}
			if err != nil {
				return "error " + err.Error() + "\n"
			}
			data, err := io.ReadAll(p)
			if err != nil {
				return "error " + err.Error() + "\n"
			}
			name, file, _ := mpfNames(p)
			mpfPart(&parts, "part", name, file, p.Header, data)
			n++
		}
		fmt.Fprintf(&b, "parts %d\n", n)
		b.WriteString(parts.String())
		return b.String()
	}
	scan := multipart.NewReader(bytes.NewReader(c.body), c.boundary)
	for {
		p, err := scan.NextPart()
		if err != nil {
			break
		}
		if _, _, ok := mpfNames(p); !ok {
			return "skip\n"
		}
	}
	f, err := r.ReadForm(32 << 20)
	if err != nil {
		return "error " + err.Error() + "\n"
	}
	defer f.RemoveAll()
	var keys []string
	for k := range f.Value {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool { return mpfText(keys[i]) < mpfText(keys[j]) })
	for _, k := range keys {
		for _, v := range f.Value[k] {
			fmt.Fprintf(&b, "v %s %s\n", mpfQuote(k), mpfQuote(v))
		}
	}
	keys = keys[:0]
	for k := range f.File {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool { return mpfText(keys[i]) < mpfText(keys[j]) })
	for _, k := range keys {
		for _, fh := range f.File[k] {
			rd, err := fh.Open()
			if err != nil {
				return "error open " + err.Error() + "\n"
			}
			data, _ := io.ReadAll(rd)
			rd.Close()
			mpfPart(&b, "f "+mpfQuote(k), k, fh.Filename, fh.Header, data)
		}
	}
	return b.String()
}

func mpfLine(c mpfCase) string {
	op := "parse"
	if c.form {
		op = "form"
	}
	return fmt.Sprintf("%s x%x x%x", op, c.boundary, c.body)
}

func mpfBlocks(out string) map[int]string {
	m := map[int]string{}
	cur := -1
	for _, l := range strings.Split(out, "\n") {
		if h, ok := strings.CutPrefix(l, "== "); ok {
			cur, _ = strconv.Atoi(h)
			m[cur] = ""
			continue
		}
		if cur >= 0 && l != "" {
			m[cur] += l + "\n"
		}
	}
	return m
}

func mpfBuild(t *testing.T, work, backend string) string {
	root, err := xotool.Root()
	if err != nil {
		t.Fatal(err)
	}
	xo, err := xotool.Path()
	if err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(work, "xo-multipart-"+backend)
	args := []string{"build"}
	if backend != "" {
		args = append(args, "--backend="+backend)
	}
	args = append(args, "-o", bin, filepath.Join(root, "scripts", "multipartdiff", "xo"))
	xb := testtmp.Guard(exec.Command(xo, args...))
	xb.Env = append(os.Environ(), "XO_STD="+filepath.Join(root, "std"))
	if out, err := xb.CombinedOutput(); err != nil {
		t.Fatalf("xo build --backend=%s: %v\n%s", backend, err, out)
	}
	return bin
}

func mpfRunXo(t *testing.T, bin string, cases []mpfCase) map[int]string {
	var lines []string
	for _, c := range cases {
		lines = append(lines, mpfLine(c))
	}
	cmd := testtmp.Guard(exec.Command(bin))
	cmd.Stdin = strings.NewReader(strings.Join(lines, "\n") + "\n")
	var out, errb bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errb
	if err := cmd.Run(); err != nil {
		t.Fatalf("%s: %v\n%s\nstderr:\n%s", bin, err, myfTail(out.String(), 20), myfTail(errb.String(), 30))
	}
	return mpfBlocks(out.String())
}

func TestMultipartFuzz(t *testing.T) {
	if testing.Short() {
		t.Skip("the multipart fuzz builds the Xo client")
	}
	seed := int64(envInt(t, "XO_MULTIPARTFUZZ_SEED", 2))
	n := envInt(t, "XO_MULTIPARTFUZZ_N", 500)
	backends := envBackends("XO_MULTIPARTFUZZ_BACKENDS", []string{"arm64", "llvm"})
	r := rand.New(rand.NewSource(seed))
	cases := make([]mpfCase, n)
	for i := range cases {
		cases[i] = mpfGen(r)
	}
	if d := os.Getenv("XO_MULTIPARTFUZZ_DUMP"); d != "" {
		i, _ := strconv.Atoi(d)
		c := cases[i]
		fmt.Printf("form %v boundary %q note %q\nbody %q\n%sline %s\n", c.form, c.boundary, c.note, c.body, mpfGo(c), mpfLine(c))
		return
	}
	want := make([]string, n)
	outcomes := map[string]int{}
	for i, c := range cases {
		want[i] = mpfGo(c)
		o := strings.SplitN(want[i], "\n", 2)[0]
		if !strings.HasPrefix(o, "error ") {
			o = "read"
			if strings.Contains(want[i], "\nh ") || strings.HasPrefix(want[i], "f ") {
				o = "read, headers"
			}
		} else if k := strings.Index(o, ": \""); k > 0 {
			o = o[:k]
		}
		outcomes[o]++
	}
	t.Logf("Go's outcomes (seed %d): %v", seed, outcomes)
	work, err := testtmp.Dir("xo-multipartfuzz-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(work)
	for _, b := range backends {
		got := mpfRunXo(t, mpfBuild(t, work, b), cases)
		bad := 0
		skipped := 0
		for i := 0; i < n; i++ {
			if got[i] == want[i] {
				continue
			}
			if want[i] == "skip\n" {
				skipped++
				continue
			}
			if bad < 10 {
				t.Errorf("%s case %d (seed %d, form %v, %s): boundary %q body %q\ngot\n%swant\n%s", b, i, seed, cases[i].form, cases[i].note, cases[i].boundary, myfTail(string(cases[i].body), 30), got[i], want[i])
			}
			bad++
		}
		if bad > 0 {
			t.Fatalf("%s: %d of %d cases differ (seed %d)", b, bad, n, seed)
		}
		t.Logf("%s: %d bodies read as Go's mime/multipart reads them (seed %d; %d forms with a name or file name that is not UTF-8 not compared)", b, n-skipped, seed, skipped)
	}
}

// TestMultipartSkipQP: ReadForm skips a part without a name by draining
// it through its decoder (Part.Close); a quoted-printable decoder stops
// at a bad byte, and NextPart goes on from what the decoder's bufio had
// read of the part. Go's partReader scans only what its own 4096 byte
// buffer holds, so the decoder's reads end inside the part's first 4096
// bytes; std/multipart went on from the end of the part's data, read in
// one scan (TestMultipartFuzz seed 4 case 15785). Go's ReadForm checks
// the expectations.
func TestMultipartSkipQP(t *testing.T) {
	var data strings.Builder
	data.WriteString("ok\r\n\x01bad\r\n")
	for data.Len() < 6000 {
		data.WriteString("0123456789abcdefghijklmnopqrstuvwxyz\r\n")
	}
	body := "--b\r\nContent-Disposition: form-data; name=\"\"\r\nContent-Transfer-Encoding: quoted-printable\r\n\r\n" + data.String() +
		"\r\n--b\r\nContent-Disposition: form-data; name=\"after\"\r\n\r\nv\r\n--b--\r\n"
	cases := []mpfCase{
		{form: true, boundary: "b", body: []byte(body)},
		// the same part read whole: the decoder's error
		{form: false, boundary: "b", body: []byte(body)},
	}
	want := []string{mpfGo(cases[0]), mpfGo(cases[1])}
	if !strings.HasPrefix(want[0], "error multipart: unexpected line in Next(): ") {
		t.Fatalf("Go's ReadForm: %q, the test expects an unexpected line inside the part", want[0])
	}
	work, err := testtmp.Dir("xo-multipartqp-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(work)
	got := mpfRunXo(t, mpfBuild(t, work, ""), cases)
	for i := range cases {
		if got[i] != want[i] {
			t.Errorf("case %d:\ngot\n%swant\n%s", i, got[i], want[i])
		}
	}
}
