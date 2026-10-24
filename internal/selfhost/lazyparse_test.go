package selfhost

import (
	"os/exec"
	"strings"
	"testing"

	"github.com/xo-lang/xo/internal/testtmp"
	"github.com/xo-lang/xo/internal/xotool"
)

// TestParserLazyBodies: the lazy parse (decision 0351 stage 2: function
// bodies skipped by their braces, FnDecl.lazy) with every body parsed
// afterwards (parser.parse_body) dumps exactly as the full parse, spans
// included, for every .xo file of the repository that parses without a
// diagnostic (std files in signature mode, as the differential test
// parses them). A file with diagnostics may differ: a skipped body's own
// errors are reported only when it is parsed.
func TestParserLazyBodies(t *testing.T) {
	if testing.Short() {
		t.Skip("dumps every repository file twice")
	}
	cli, err := xotool.Command()
	if err != nil {
		t.Skip(err)
	}
	root := repoRoot(t)
	var plain, std []string
	for _, f := range repoXoFiles(t, root) {
		if strings.HasPrefix(f, "std/") {
			std = append(std, f)
		} else {
			plain = append(plain, f)
		}
	}
	dumpAll := func(files []string, flags ...string) map[string]string {
		t.Helper()
		out := map[string]string{}
		for i := 0; i < len(files); i += 200 {
			j := min(i+200, len(files))
			args := append(append(append([]string{}, cli[1:]...), "dev", "dump", "ast"), flags...)
			cmd := testtmp.Guard(exec.Command(cli[0], append(args, files[i:j]...)...))
			cmd.Dir = root
			b, err := cmd.Output()
			if err != nil {
				t.Fatalf("xo dev dump ast %v: %v", flags, err)
			}
			for _, part := range strings.Split(string(b), "\n== ")[0:] {
				part = strings.TrimPrefix(part, "== ")
				name, _, _ := strings.Cut(part, "\n")
				out[name] = part
			}
		}
		return out
	}
	compared, lazyBodies := 0, 0
	for _, set := range []struct {
		files []string
		sig   []string
	}{{plain, nil}, {std, []string{"--sig"}}} {
		full := dumpAll(set.files, set.sig...)
		lazy := dumpAll(set.files, append([]string{"--lazy"}, set.sig...)...)
		for name, want := range full {
			if strings.Contains(want, "\ndiag ") {
				continue
			}
			compared++
			lazyBodies += strings.Count(want, "body: Block")
			if got := lazy[name]; got != want {
				t.Errorf("%s: the lazy parse with its bodies parsed dumps differently", name)
			}
		}
	}
	t.Logf("%d files compared (%d bodies)", compared, lazyBodies)
	if compared < 600 {
		t.Fatalf("only %d files compared", compared)
	}
}
