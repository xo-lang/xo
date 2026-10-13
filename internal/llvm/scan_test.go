package llvm_test

import (
	"encoding/hex"
	"math/rand"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/xo-lang/xo/internal/build"
)

// scanDiffProg reads lines of `<op> <hex a> <hex b>` from standard input
// and prints one line for each: `s` runs the Str operations of
// std/native/llrt/scan.xo on the UTF-8 texts a and b (find, contains,
// starts_with, ends_with, split, lines, rune_count, replace, the case
// maps and quoting that scan.span serves), `b` the Bytes operations on
// the bytes a (to_hex, quote, the scans of a Bytes) (decision 0201).
const scanDiffProg = `fn text(h: Str) -> Str {
  match (Bytes.from_hex(h) ?? b"").to_str() {
    Some(s) => s
    None => "?"
  }
}

fn main(os: Os) {
  let input = match os.stdio.read_all() {
    Ok(s) => s
    Err(_) => ""
  }
  var out: List[Str] = []
  for line in input.lines() {
    let f = line.split(" ")
    if f.len() != 3 {
      out.push("?")
      continue
    }
    let r = match f[0] {
      "s" => {
        let a = text(f[1])
        let b = text(f[2])
        let parts = [
          "${a.find(b) ?? -1}", "${a.contains(b)}", "${a.starts_with(b)}", "${a.ends_with(b)}",
          a.split(b).debug(), a.lines().debug(), "${a.rune_count()}", a.replace(b, "<>").debug(),
          a.to_lower().debug(), a.to_upper().debug(), a.debug(), a.trim_prefix(b).debug(), a.trim_suffix(b).debug(),
        ]
        parts.join(" | ")
      }
      "b" => {
        let a = Bytes.from_hex(f[1]) ?? b""
        [a.to_hex(), a.debug(), "${a.rune_count()}", "${a.len()}"].join(" | ")
      }
      _ => "?"
    }
    out.push(r)
  }
  os.stdio.println(out.join("\n"))
}
`

// scanCorpus is the input of scanDiffProg: needles of 1 to 20 bytes at
// every offset of texts of 0 to 70 bytes (so every start and end
// alignment of the 8 and 16 byte steps and of the reads that end at the
// end), near misses (the first byte of the needle without the rest),
// separators of 0 to 3 bytes, lines with \r\n, \n, and \r, runs of
// multibyte runes, quotes, backslashes, control bytes and 0x7F at every
// offset, random UTF-8 texts; and random and invalid bytes for the Bytes
// operations.
func scanCorpus(n int) string {
	r := rand.New(rand.NewSource(11))
	var b strings.Builder
	str := func(a, s string) {
		if !utf8.ValidString(a) || !utf8.ValidString(s) {
			panic("scan corpus: invalid UTF-8")
		}
		b.WriteString("s " + hex.EncodeToString([]byte(a)) + " " + hex.EncodeToString([]byte(s)) + "\n")
	}
	byt := func(a string) { b.WriteString("b " + hex.EncodeToString([]byte(a)) + " x\n") }
	fill := strings.Repeat("abcdefghijklmnopqrstuvwxyz0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZ-_=+", 2)
	for _, needle := range []string{",", "\n", "ab", "zz", "é", "日本", "\r\n", "x\x00y", "aaaaaaaaab", "0123456789abcdefghij", "\x7f", "😀"} {
		for l := 0; l <= 70; l++ {
			for k := 0; k+len(needle) <= l; k += 1 + l/24 {
				str(fill[:k]+needle+fill[k+len(needle):l], needle)
				str(strings.Repeat("q", k)+needle+strings.Repeat("r", l-k-len(needle)), needle)
				// a near miss: the needle's first byte only
				if len(needle) > 1 && utf8.RuneLen(rune(needle[0])) == 1 && needle[0] < 0x80 {
					str(fill[:k]+needle[:1]+strings.Repeat(".", l-k-1), needle)
				}
			}
			str(strings.Repeat("a", l), "a")
			str(strings.Repeat("ab", l/2), "ba")
		}
	}
	for l := 0; l <= 70; l++ {
		for k := 0; k <= l; k += 1 + l/24 {
			for _, c := range []string{"\"", "\\", "\x01", "\x7f", "é", "\u3000", "\u00a0", "\t", "日"} {
				s := strings.Repeat("m", k) + c + strings.Repeat("n", l-k)
				str(s, "")
				str(s, c)
				byt(s)
				byt(s[:len(s)-1])
			}
		}
	}
	for _, s := range []string{"", "\n", "\n\n", "\r\n", "a\r\nb\r\n", "a\rb", "\r", "a\n\rb", "x\r\r\n", "line\n", "\nline", "é\n日\r\n😀"} {
		str(s, "\n")
		str(s, "\r\n")
		str(s, "")
	}
	alphabet := []string{"a", "b", ",", ";", " ", "\n", "\r", "\t", "\"", "\\", "\x00", "\x7f", "é", "ß", "日", "本", "😀", "\u3000", "AB", "zz", "İ", "ﬀ"}
	for i := 0; i < n; i++ {
		var w strings.Builder
		for k := r.Intn(90); k > 0; k-- {
			w.WriteString(alphabet[r.Intn(len(alphabet))])
		}
		var s strings.Builder
		for k := r.Intn(4); k > 0; k-- {
			s.WriteString(alphabet[r.Intn(len(alphabet))])
		}
		str(w.String(), s.String())
		raw := make([]byte, r.Intn(70))
		r.Read(raw)
		byt(string(raw))
	}
	for _, s := range []string{"\xff", "\xc3", "\xe6\x97", "\xed\xa0\x80", "a\x80b", "\xf4\x90\x80\x80", "\xc0\xaf", strings.Repeat("\x80", 17)} {
		byt(s)
		byt(strings.Repeat("x", 9) + s)
		byt(s + strings.Repeat("y", 15))
	}
	long := strings.Repeat("0123456789", 40)
	str(long+",", ",")
	str(","+long, ",")
	str(long+"needle"+long, "needle")
	str(long+"needl", "needle")
	str(strings.Repeat("é", 300), "日")
	byt(long)
	return b.String()
}

// TestScanDiff runs scanDiffProg over scanCorpus on the Go backend (the
// reference: Go's strings and encoding/hex) and on both native backends
// with the scans in Xo (std/native/llrt/scan.xo) and with their C path
// (XO_LLRT_C=scan), and requires the Go backend's lines; the LLVM build
// of the Xo path also frees every allocation (decision 0201).
func TestScanDiff(t *testing.T) {
	if testing.Short() {
		t.Skip("builds five programs")
	}
	n := 3000
	if s := os.Getenv("XO_SCANDIFF_N"); s != "" {
		n, _ = strconv.Atoi(s)
	}
	in := scanCorpus(n)
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "main.xo"), []byte(scanDiffProg), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "stdin.txt"), []byte(in), 0o644); err != nil {
		t.Fatal(err)
	}
	inLines := strings.Split(strings.TrimSuffix(in, "\n"), "\n")
	t.Logf("%d input lines", len(inLines))
	ref := buildAndRun(t, dir, build.Options{Backend: build.BackendGo})
	if ref.exit != 0 {
		t.Fatalf("go backend: exit %d: %s", ref.exit, ref.stderr)
	}
	want := strings.Split(strings.TrimSuffix(ref.stdout, "\n"), "\n")
	if len(want) != len(inLines) {
		t.Fatalf("go backend: %d lines for %d inputs", len(want), len(inLines))
	}
	for _, path := range []string{"xo", "c"} {
		for _, be := range []string{build.BackendArm64, build.BackendLLVM} {
			t.Run(path+"/"+be, func(t *testing.T) {
				if path == "c" {
					t.Setenv("XO_LLRT_C", "scan")
				}
				r, res := buildRun(t, dir, build.Options{Backend: be})
				if r.exit != 0 {
					t.Fatalf("exit %d: %s", r.exit, r.stderr)
				}
				got := strings.Split(strings.TrimSuffix(r.stdout, "\n"), "\n")
				bad := 0
				for i := 0; i < len(want) || i < len(got); i++ {
					var g, w string
					if i < len(got) {
						g = got[i]
					}
					if i < len(want) {
						w = want[i]
					}
					if g != w {
						if bad < 10 {
							x := ""
							if i < len(inLines) {
								x = inLines[i]
							}
							t.Errorf("%.200q:\n got %.300q\nwant %.300q", x, g, w)
						}
						bad++
					}
				}
				if bad > 0 {
					t.Errorf("%d of %d lines differ", bad, len(want))
				}
				if path == "xo" && be == build.BackendLLVM {
					leakCheck(t, res.Binaries[build.Host()], dir)
				}
			})
		}
	}
}
