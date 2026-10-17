package selfhost

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestSelfhostFormat runs the self hosted formatter (compiler/cli/core/
// format, which `xo fmt` uses; decision 0178 stage 2) on every .xo file
// of the repository and requires the recorded output (internal/format's
// until decision 0215): the same formatted text, or a syntax error.
func TestSelfhostFormat(t *testing.T) {
	skipDumpReplay(t)
	bin, err := selfhostBinary("cli/core/format", "go")
	if err != nil {
		t.Fatal(err)
	}
	root := repoRoot(t)
	files := repoXoFiles(t, root)
	t.Run("repo", func(t *testing.T) { formatCompare(t, bin, root, files) })
	// The repository is formatted already: the layout rules show on
	// copies whose blanks are changed (indentation dropped or added,
	// trailing blanks, doubled blank lines).
	tmp := t.TempDir()
	var moved []string
	for i, f := range files {
		src, err := os.ReadFile(filepath.Join(root, f))
		if err != nil {
			t.Fatal(err)
		}
		lines := strings.Split(string(src), "\n")
		for j, l := range lines {
			switch i % 3 {
			case 0:
				lines[j] = strings.TrimLeft(l, " \t") + "  "
			case 1:
				lines[j] = "   " + l
			case 2:
				if strings.TrimSpace(l) == "" {
					lines[j] = "\n \t"
				}
			}
		}
		name := filepath.Join(fmt.Sprintf("m%d", i%3), f)
		if err := os.MkdirAll(filepath.Join(tmp, filepath.Dir(name)), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(tmp, name), []byte(strings.Join(lines, "\n")), 0o644); err != nil {
			t.Fatal(err)
		}
		moved = append(moved, filepath.ToSlash(name))
	}
	t.Run("blanks", func(t *testing.T) { formatCompare(t, bin, tmp, moved) })
}

// formatCompare formats files (relative to root) with bin and fails on
// the first differences from the recording.
func formatCompare(t *testing.T, bin, root string, files []string) {
	t.Helper()
	cmd := exec.Command(bin, files...)
	cmd.Dir = root
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("%s: %v\n%s", bin, err, stderr.String())
	}
	got := splitFormatDump(string(out))
	same, errs, bad := 0, 0, 0
	for _, f := range files {
		gp := goFormat(t, root, f, got[f])
		if gp.missing {
			t.Error(gp.missingText())
			bad++
			continue
		}
		g, ok := got[f]
		// replayed, the recorded side's text is known when the hashes agree
		want, known := gp.resolve("-", g)
		switch {
		case !known:
		case strings.HasPrefix(want, " syntax errors "):
			errs++
		case strings.HasPrefix(want, "== "):
			t.Fatalf("%s: %s", f, want)
		}
		if !ok {
			t.Errorf("%s: no output from the self hosted formatter", f)
			bad++
			continue
		}
		if !known || g != want {
			bad++
			if bad <= 5 {
				diff := gp.diff("-", g)
				if known {
					diff = firstLineDiff(want, g)
				}
				t.Errorf("%s: formatted differently:\n%s", f, diff)
			}
			continue
		}
		same++
	}
	t.Logf("%d files: %d identical (%d syntax errors in both), %d different", len(files), same, errs, bad)
}

// goFormat is the recorded formatter result for file f (relative to
// root) as the Xo formatter prints it: "\n" and the text, ` syntax
// errors <n>\n`, or `== error <why>` (the format kind, gogoldens_test.go:
// the whole text is piece "-"); x is the Xo formatter's, which a
// recording run records.
func goFormat(t *testing.T, root, f, x string) *goldPieces {
	t.Helper()
	full := filepath.Join(root, f)
	src, err := os.ReadFile(full)
	if err != nil {
		t.Fatal(err)
	}
	cx := newGoldenCtx(full)
	hint := goldHint("TestSelfhostFormat", "cli/core/format", f, root)
	return recordedPieces("format", f, hint, []string{cx.fileKey(f, src)}, cx, func() *goldPieces {
		g := newPieces()
		g.add("-", x)
		return g
	})
}

// splitFormatDump splits the formatter's output into the text after each
// `== <path>` header, by path.
func splitFormatDump(s string) map[string]string {
	out := map[string]string{}
	for _, part := range strings.Split("\n"+strings.TrimSuffix(s, "\n"), "\n== ")[1:] {
		name, rest, found := strings.Cut(part, "\n")
		if p, n, ok := strings.Cut(name, " syntax errors "); ok {
			out[p] = " syntax errors " + n + "\n"
			continue
		}
		if !found {
			out[name] = "\n" // an empty file
			continue
		}
		out[name] = "\n" + rest + "\n"
	}
	return out
}

// firstLineDiff shows the first differing line of want and got with context.
func firstLineDiff(want, got string) string {
	wl := strings.Split(want, "\n")
	gl := strings.Split(got, "\n")
	for i := 0; i < len(wl) || i < len(gl); i++ {
		w, g := "<end>", "<end>"
		if i < len(wl) {
			w = wl[i]
		}
		if i < len(gl) {
			g = gl[i]
		}
		if w != g {
			return fmt.Sprintf("line %d:\n  want %q\n  got  %q", i, w, g)
		}
	}
	return "(same lines)"
}
