package check

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/xo-lang/xo/internal/diag"
)

// checkFiles writes files into a temporary directory tree and checks every
// module in it.
func checkFiles(t *testing.T, files map[string]string) []diag.Diagnostic {
	t.Helper()
	dir := t.TempDir()
	for name, src := range files {
		p := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	res, err := Check([]string{dir})
	if err != nil {
		t.Fatal(err)
	}
	return res.Diagnostics
}

func checkSrc(t *testing.T, src string) []diag.Diagnostic {
	t.Helper()
	return checkFiles(t, map[string]string{"m/main.xo": src})
}

func codes(ds []diag.Diagnostic) []string {
	var out []string
	for _, d := range ds {
		out = append(out, d.Code)
	}
	sort.Strings(out)
	return out
}

func humanAll(ds []diag.Diagnostic) string {
	var b strings.Builder
	for _, d := range ds {
		b.WriteString(diag.Human(d))
		b.WriteByte('\n')
	}
	return b.String()
}

// ruleCase is one program and the exact sorted list of codes it must
// produce (nil: no diagnostics).
type ruleCase struct {
	name  string
	src   string
	files map[string]string
	want  []string
}

func runCases(t *testing.T, cases []ruleCase) {
	t.Helper()
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var ds []diag.Diagnostic
			if tc.files != nil {
				ds = checkFiles(t, tc.files)
			} else {
				ds = checkSrc(t, tc.src)
			}
			got := codes(ds)
			want := append([]string(nil), tc.want...)
			sort.Strings(want)
			if strings.Join(got, ",") != strings.Join(want, ",") {
				t.Errorf("codes = %v, want %v\n%s", got, want, humanAll(ds))
			}
		})
	}
}
