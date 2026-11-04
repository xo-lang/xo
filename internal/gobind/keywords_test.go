package gobind

import (
	"os"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// TestKeywordsMatchLexer keeps xoKeywords equal to the lexer's keywords
// (the keys of KEYWORDS in compiler/lexer/token.xo; internal/token's
// table until decision 0215 deleted it).
func TestKeywordsMatchLexer(t *testing.T) {
	data, err := os.ReadFile("../../compiler/lexer/token.xo")
	if err != nil {
		t.Fatal(err)
	}
	s := string(data)
	i := strings.Index(s, "const KEYWORDS = {\n")
	if i < 0 {
		t.Fatal("no KEYWORDS in compiler/lexer/token.xo")
	}
	s = s[i:]
	s = s[:strings.Index(s, "\n}\n")]
	lex := map[string]bool{}
	for _, m := range regexp.MustCompile(`(?m)^  "(\w+)": Kind\.`).FindAllStringSubmatch(s, -1) {
		lex[m[1]] = true
		if !xoKeywords[m[1]] {
			t.Errorf("the lexer's keyword %q is not in xoKeywords", m[1])
		}
	}
	for k := range xoKeywords {
		if !lex[k] {
			t.Errorf("xoKeywords has %q, the lexer has no such keyword", k)
		}
	}
}

// TestVersionMatchesChecker keeps Version equal to compiler/check's
// GOBIND_VERSION, the binding format xo reads: a bump of one alone makes
// every `use go` binding fail with XO1101 "wrote binding format N, this
// xo reads M" (2026-11-04, the scope keyword fix bumped only Version).
func TestVersionMatchesChecker(t *testing.T) {
	data, err := os.ReadFile("../../compiler/check/goimport.xo")
	if err != nil {
		t.Fatal(err)
	}
	m := regexp.MustCompile(`(?m)^const GOBIND_VERSION: Int = (\d+)$`).FindStringSubmatch(string(data))
	if m == nil {
		t.Fatal("no GOBIND_VERSION in compiler/check/goimport.xo")
	}
	if want := strconv.Itoa(Version); m[1] != want {
		t.Errorf("compiler/check GOBIND_VERSION is %s, gobind.Version is %s: bump both", m[1], want)
	}
}
