package selfhost

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// TestFrozenCopies requires the copies of frozen_test.go to be the
// originals' text (the declarations, renamed) while the Go
// implementation exists: a change there that the copies miss would make
// the replayed comparisons drift from the live ones. Delete this test
// with internal/llvm, internal/ssa, and internal/gen (decision 0178
// stage 7).
func TestFrozenCopies(t *testing.T) {
	root := repoRoot(t)
	copies, err := os.ReadFile("frozen_test.go")
	if err != nil {
		t.Fatal(err)
	}
	ren := strings.NewReplacer("IRParts", "irParts", "SplitIR", "splitIR", "DefName", "defName",
		"SplitModule", "splitModule", "headerName", "ssaHeaderName", "FormatFile", "formatGenFile")
	for _, c := range []struct{ file, decl string }{
		{"internal/llvm/split.go", "type IRParts struct"},
		{"internal/llvm/split.go", "const irHead ="},
		{"internal/llvm/split.go", "func SplitIR("},
		{"internal/llvm/split.go", "func DefName("},
		{"internal/llvm/fromssa.go", "const ssaRuntimeDecls ="},
		{"internal/ssa/stages.go", "func SplitModule("},
		{"internal/ssa/stages.go", "func headerName("},
		{"internal/gen/gen.go", "func FormatFile("},
		{"internal/gen/gen.go", "func dropRepeatedLineDirectives("},
		{"internal/gen/blank.go", "func dropBlankUses("},
	} {
		src, err := os.ReadFile(filepath.Join(root, c.file))
		if err != nil {
			t.Fatal(err)
		}
		text := declText(string(src), c.decl)
		if text == "" {
			t.Errorf("%s: no %s", c.file, c.decl)
			continue
		}
		if !strings.Contains(string(copies), ren.Replace(text)) {
			t.Errorf("%s: %s is not the copy in frozen_test.go: copy it again (and record the hashes again if a split or format changes)", c.file, c.decl)
		}
	}
}

// declText is the top level declaration of src that starts with decl:
// up to its closing line ("}", ")", or the end of a raw string).
func declText(src, decl string) string {
	i := strings.Index(src, "\n"+decl)
	if i < 0 {
		return ""
	}
	rest := src[i+1:]
	end := regexp.MustCompile("(?m)^(}|\\)|`)$").FindStringIndex(rest)
	if strings.HasPrefix(decl, "const ") && !strings.HasSuffix(strings.SplitN(rest, "\n", 2)[0], "`") {
		// a one line constant
		return strings.SplitN(rest, "\n", 2)[0]
	}
	if end == nil {
		return ""
	}
	return rest[:end[1]]
}
