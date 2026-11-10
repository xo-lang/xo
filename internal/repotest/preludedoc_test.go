package repotest

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// TestPreludeFnsDocumented: every fn of std/prelude.xo has a /// doc
// comment of its own, since xo lsp's hover and `xo doc` show it for the
// methods every program calls (Str.split, List.sort_by). A group of fns
// under one comment left the rest of the group with an empty hover; 254
// of 421 had none on 2026-11-10. `/// @moves` and other `/// @` lines
// are attributes, not a doc comment.
func TestPreludeFnsDocumented(t *testing.T) {
	data, err := os.ReadFile(filepath.Join(root, "std", "prelude.xo"))
	if err != nil {
		t.Fatal(err)
	}
	fnLine := regexp.MustCompile(`^(pub )?fn `)
	lines := strings.Split(string(data), "\n")
	var missing []string
	for i, l := range lines {
		if !fnLine.MatchString(l) {
			continue
		}
		j := i - 1
		for j >= 0 && strings.HasPrefix(lines[j], "/// @") {
			j--
		}
		if j < 0 || !strings.HasPrefix(lines[j], "///") {
			missing = append(missing, "std/prelude.xo:"+itoa(i+1)+": "+l)
		}
	}
	if len(missing) > 0 {
		t.Errorf("%d prelude fns have no /// doc comment of their own (write one line, Go style, naming the Go equivalent where there is one):\n%s",
			len(missing), strings.Join(missing, "\n"))
	}
}
