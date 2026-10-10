package selfhost

import (
	"bytes"
	"fmt"
	"io/fs"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/xo-lang/xo/internal/build"
	"github.com/xo-lang/xo/internal/gen"
	"github.com/xo-lang/xo/internal/lexer"
)

// repoRoot is the repository root, two levels up from this package.
func repoRoot(t testing.TB) string {
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	return root
}

// lexerBinaries holds selfhost/lexer (the Xo lexer's CLI) built once per
// backend.
var (
	lexerMu       sync.Mutex
	lexerBinaries = map[string]string{}
)

// lexerBinary builds selfhost/lexer with backend ("go", "arm64", or
// "llvm"; decision 0081) once.
// TestMain turns on the binary cache: the lexer is rebuilt only when its
// generated code or the toolchain changes.
func TestMain(m *testing.M) {
	build.EnableTestBinCache()
	os.Exit(m.Run())
}

func lexerBinary(backend string) (string, error) {
	lexerMu.Lock()
	defer lexerMu.Unlock()
	if bin, ok := lexerBinaries[backend]; ok {
		return bin, nil
	}
	dir := filepath.Join("..", "..", "selfhost", "lexer")
	res, err := build.Build([]string{dir}, build.Options{Mode: gen.Build, Backend: backend})
	if err != nil {
		return "", err
	}
	if res.Failed {
		return "", fmt.Errorf("check failed: %+v", res.Diagnostics)
	}
	bin := res.Binaries[build.Host()]
	lexerBinaries[backend] = bin
	return bin, nil
}

// nativeSkip is why a native backend cannot build on this host, or "".
func nativeSkip(backend string) string {
	if _, err := build.Clang(); err != nil {
		return err.Error()
	}
	if backend == "arm64" && (runtime.GOARCH != "arm64" || (runtime.GOOS != "darwin" && runtime.GOOS != "linux")) {
		return "the arm64 backend builds on darwin/arm64 and linux/arm64 hosts"
	}
	return ""
}

// repoXoFiles lists every .xo file under root, relative to it, skipping
// dot directories (.git, .tools, .xo, .claude).
func repoXoFiles(t testing.TB, root string) []string {
	var files []string
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() && p != root && strings.HasPrefix(d.Name(), ".") {
			return filepath.SkipDir
		}
		if !d.IsDir() && strings.HasSuffix(p, ".xo") {
			rel, err := filepath.Rel(root, p)
			if err != nil {
				return err
			}
			files = append(files, filepath.ToSlash(rel))
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(files)
	return files
}

// specExamples extracts the fenced code blocks of the spec (blocks with no
// language or `xo`) into files under dir.
func specExamples(t testing.TB, root, dir string) []string {
	var out []string
	for _, name := range []string{"core.md", "std.md"} {
		data, err := os.ReadFile(filepath.Join(root, "spec", name))
		if err != nil {
			t.Fatal(err)
		}
		var block []string
		in, keep := false, false
		n := 0
		for _, line := range strings.Split(string(data), "\n") {
			trimmed := strings.TrimSpace(line)
			if strings.HasPrefix(trimmed, "```") {
				if !in {
					lang := strings.TrimPrefix(trimmed, "```")
					in, keep, block = true, lang == "" || lang == "xo", nil
					continue
				}
				in = false
				if keep {
					n++
					p := filepath.Join(dir, fmt.Sprintf("spec-%s-%03d.xo", strings.TrimSuffix(name, ".md"), n))
					if err := os.WriteFile(p, []byte(strings.Join(block, "\n")+"\n"), 0o644); err != nil {
						t.Fatal(err)
					}
					out = append(out, p)
				}
				continue
			}
			if in {
				block = append(block, line)
			}
		}
	}
	return out
}

// fragments are spliced into mutated inputs: the starts and ends of every
// literal form, escapes, interpolation, comments, and bytes that are not
// valid UTF-8 or not printable.
var fragments = []string{
	`"`, `"""`, "\"\"\"\n  a\n  ${x}\n  \"\"\"", `${`, `${}`, `${ }`, `}`, `{`, `\`, `\\`, `\n`, `\$`,
	"\n", "\r\n", "\r", "\t", " ", `r"`, `r#"`, `"#`, `r##"x"#"##`, `'`, `'\''`, `'ab'`, `b"`, `b"\xff"`,
	`0x`, `0b`, `0o`, `0x_f`, `_`, `1__0`, `.`, `..`, `..=`, `t.0.1`, `e+`, `1e`, `1.5s`, `ms`, `ns`, `0xffs`,
	`\u{`, `\u{1F600}`, `\u{d800}`, `\u{110000}`, `\u{1234567}`, `\u{zz}`, `\x`, `\x7f`, `\xff`, `\q`,
	"\\\xc3", `///`, `////`, `//`, `use `, "use /a//b/", "use ", `???`, `??`, `#`, `#{`, `@`, `$`, "`",
	"\x00", "\x01", "\x7f", "\xff", "\xc3", "\xe2\x82", "\xed\xa0\x80", "\xf4\x90\x80\x80", "é",
	"\u00a0", "\u0085", "\u2028", "\u3000", "😀", "\ufeff", "\u200b", "\U000e0001",
	"99999999999999999999", "0b102", "0o78", "9223372036854775807ns", "9223372036854775s", "18446744073709551615",
	"\"${\"${a}\"}\"", "\"${'}'}\"", "\"${r#\"}\"#}\"", "\"${\n}\"", "\n  .x", "\n..x", "return\n", "x?\n",
}

// mutate returns a mutated window of src.
func mutate(rng *rand.Rand, src []byte) []byte {
	start := 0
	if len(src) > 0 {
		start = rng.Intn(len(src))
	}
	end := start + 1 + rng.Intn(1500)
	if end > len(src) {
		end = len(src)
	}
	b := append([]byte(nil), src[start:end]...)
	for range 1 + rng.Intn(6) {
		pos := 0
		if len(b) > 0 {
			pos = rng.Intn(len(b) + 1)
		}
		switch rng.Intn(5) {
		case 0, 1: // insert a fragment
			f := fragments[rng.Intn(len(fragments))]
			b = append(b[:pos], append([]byte(f), b[pos:]...)...)
		case 2: // replace a byte with a random one
			if pos < len(b) {
				b[pos] = byte(rng.Intn(256))
			}
		case 3: // delete a run
			n := rng.Intn(20)
			if pos+n > len(b) {
				n = len(b) - pos
			}
			b = append(b[:pos], b[pos+n:]...)
		case 4: // duplicate a run
			n := rng.Intn(40)
			if pos+n > len(b) {
				n = len(b) - pos
			}
			dup := append([]byte(nil), b[pos:pos+n]...)
			b = append(b[:pos], append(dup, b[pos:]...)...)
		}
	}
	return b
}

// fuzzCorpus writes n mutated inputs (seeded, so failures reproduce) under
// dir and returns their paths.
func fuzzCorpus(t testing.TB, root, dir string, files []string, n int) []string {
	seed := int64(20261008)
	if v := os.Getenv("XO_SELFHOST_SEED"); v != "" {
		fmt.Sscan(v, &seed)
	}
	rng := rand.New(rand.NewSource(seed))
	var srcs [][]byte
	for _, f := range files {
		data, err := os.ReadFile(filepath.Join(root, f))
		if err != nil {
			t.Fatal(err)
		}
		srcs = append(srcs, data)
	}
	var out []string
	for i := range n {
		var src []byte
		if len(srcs) > 0 {
			src = srcs[rng.Intn(len(srcs))]
		}
		p := filepath.Join(dir, fmt.Sprintf("fuzz-%05d.xo", i))
		if err := os.WriteFile(p, mutate(rng, src), 0o644); err != nil {
			t.Fatal(err)
		}
		out = append(out, p)
	}
	return out
}

// runeFiles writes every step-th rune (one per line, each an invalid
// character unless it is ASCII) into files under dir, to check Rune.quote
// (strconv.QuoteRune) on every rune.
func runeFiles(t testing.TB, dir string, step int) []string {
	var out []string
	var b strings.Builder
	flush := func() {
		p := filepath.Join(dir, fmt.Sprintf("runes-%03d.xo", len(out)))
		if err := os.WriteFile(p, []byte(b.String()), 0o644); err != nil {
			t.Fatal(err)
		}
		out = append(out, p)
		b.Reset()
	}
	for r := rune(0); r <= 0x10FFFF; r += rune(step) {
		if r >= 0xD800 && r <= 0xDFFF {
			continue
		}
		b.WriteRune(r)
		b.WriteByte('\n')
		if b.Len() > 64<<10 {
			flush()
		}
	}
	flush()
	return out
}

// runXo runs the Xo lexer on paths (relative to root or absolute).
func runXo(t testing.TB, bin, root string, paths []string) string {
	cmd := exec.Command(bin, paths...)
	cmd.Dir = root
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("xo lexer on %d files: %v\n%s", len(paths), err, stderr.String())
	}
	return stdout.String()
}

// goDump is the Go lexer's dump of the same paths.
func goDump(t testing.TB, root string, paths []string) string {
	var b strings.Builder
	for _, p := range paths {
		full := p
		if !filepath.IsAbs(p) {
			full = filepath.Join(root, p)
		}
		src, err := os.ReadFile(full)
		if err != nil {
			t.Fatal(err)
		}
		b.WriteString(lexer.Dump(p, src))
	}
	return b.String()
}

// firstDiff describes the first differing line of two outputs.
func firstDiff(got, want string) string {
	g, w := strings.Split(got, "\n"), strings.Split(want, "\n")
	file := ""
	for i := 0; i < len(g) || i < len(w); i++ {
		var gl, wl string
		if i < len(g) {
			gl = g[i]
		}
		if i < len(w) {
			wl = w[i]
		}
		if strings.HasPrefix(wl, "== ") {
			file = wl
		}
		if gl != wl {
			return fmt.Sprintf("%s, line %d:\n  xo: %s\n  go: %s", file, i+1, gl, wl)
		}
	}
	return "outputs differ only in length"
}

// compare lexes paths with both lexers in batches and fails on the first
// difference.
func compare(t *testing.T, bin, root string, paths []string) {
	const batch = 200
	for i := 0; i < len(paths); i += batch {
		chunk := paths[i:min(i+batch, len(paths))]
		got, want := runXo(t, bin, root, chunk), goDump(t, root, chunk)
		if got != want {
			t.Fatalf("Xo and Go lexers differ: %s", firstDiff(got, want))
		}
	}
}

// TestSelfhostDifferential runs the Xo lexer (selfhost/lexer), built with
// each backend (Go, and the native arm64 and LLVM backends where the host
// has them, decision 0081), and the Go lexer on every .xo file of the
// repository, the spec's code blocks, runes, and a seeded corpus of
// mutated inputs, and requires byte identical dumps, diagnostics included.
// -short uses a smaller fuzz corpus.
func TestSelfhostDifferential(t *testing.T) {
	for _, backend := range []string{"go", "arm64", "llvm"} {
		t.Run(backend, func(t *testing.T) {
			t.Parallel()
			if backend != "go" {
				if why := nativeSkip(backend); why != "" {
					t.Skip(why)
				}
			}
			bin, err := lexerBinary(backend)
			if err != nil {
				t.Fatal(err)
			}
			differential(t, bin)
		})
	}
}

// differential is TestSelfhostDifferential for the lexer binary bin.
func differential(t *testing.T, bin string) {
	root := repoRoot(t)
	files := repoXoFiles(t, root)
	if len(files) < 100 {
		t.Fatalf("found only %d .xo files under %s", len(files), root)
	}
	t.Run("repo", func(t *testing.T) { compare(t, bin, root, files) })
	tmp := t.TempDir()
	if keep := os.Getenv("XO_SELFHOST_KEEP"); keep != "" {
		// Keep the spec blocks and the fuzz corpus for inspection, one
		// directory per backend (the backends run in parallel).
		tmp = filepath.Join(keep, filepath.Base(t.Name()))
		if err := os.MkdirAll(tmp, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Run("spec", func(t *testing.T) {
		spec := specExamples(t, root, tmp)
		if len(spec) < 50 {
			t.Fatalf("extracted only %d spec code blocks", len(spec))
		}
		compare(t, bin, root, spec)
	})
	n := 3000
	if testing.Short() {
		n = 300
	}
	if v := os.Getenv("XO_SELFHOST_FUZZ"); v != "" {
		fmt.Sscan(v, &n)
	}
	t.Logf("%d repo files, %d fuzz inputs", len(files), n)
	step := 13
	if v := os.Getenv("XO_SELFHOST_RUNE_STEP"); v != "" {
		fmt.Sscan(v, &step)
	}
	t.Run("runes", func(t *testing.T) { compare(t, bin, root, runeFiles(t, tmp, step)) })
	t.Run("fuzz", func(t *testing.T) { compare(t, bin, root, fuzzCorpus(t, root, tmp, files, n)) })
}

// TestSelfhostXoTests runs the Xo tests of selfhost/lexer (`xo test`).
func TestSelfhostXoTests(t *testing.T) {
	if testing.Short() {
		t.Skip("builds and runs the Xo test binary")
	}
	dir := filepath.Join("..", "..", "selfhost", "lexer")
	res, err := build.Build([]string{dir}, build.Options{Mode: gen.Test})
	if err != nil {
		t.Fatal(err)
	}
	if res.Failed {
		t.Fatalf("check failed: %+v", res.Diagnostics)
	}
	cmd := exec.Command(res.Binaries[build.Host()])
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("xo test selfhost/lexer: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "0 failed") {
		t.Fatalf("unexpected test output:\n%s", out)
	}
}
