package repotest

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// TestGateRunPatterns: every `go test ./PKG ... -run 'PATTERN'` in
// scripts/gate.sh names tests that exist (task #249, 2026-11-17). Each
// top-level alternative of the pattern's first level must match a Test
// function of the package, and no alternative may hold a '/': go test
// splits levels only at slashes outside brackets and parentheses, so
// '^(TestA|TestB/sub)$' never runs TestB (the tsanmaps step skipped every
// conc_ golden that way). nohttp's 'Go|Replay|Holes' matched all four
// TestDevGoldens* ('Go' is in 'Goldens') while Replay and Holes matched
// none, so the step ran the Arm64, LLVM and ELF goldens too (~6 min a
// gate; found by Lang 3).
func TestGateRunPatterns(t *testing.T) {
	data, err := os.ReadFile(filepath.Join(root, "scripts", "gate.sh"))
	if err != nil {
		t.Fatal(err)
	}
	use := regexp.MustCompile(`(\./[\w/.-]+)[^'\n]*?-run '([^']*)'`)
	fn := regexp.MustCompile(`(?m)^func (Test\w+)\(\w+ \*testing\.T\)`)
	n := 0
	for _, m := range use.FindAllStringSubmatch(string(data), -1) {
		pkg, pat := m[1], m[2]
		files, _ := filepath.Glob(filepath.Join(root, pkg, "*_test.go"))
		var tests []string
		for _, f := range files {
			b, err := os.ReadFile(f)
			if err != nil {
				t.Fatal(err)
			}
			for _, x := range fn.FindAllStringSubmatch(string(b), -1) {
				tests = append(tests, x[1])
			}
		}
		if len(tests) == 0 {
			t.Errorf("gate.sh runs %s -run '%s', which has no tests", pkg, pat)
			continue
		}
		n++
		first := splitRunLevels(pat)[0]
		re, err := regexp.Compile(first)
		if err != nil {
			t.Errorf("gate.sh %s -run '%s': %v", pkg, pat, err)
			continue
		}
		if !anyMatch(re, tests) {
			t.Errorf("gate.sh %s -run '%s' matches no test of the package", pkg, pat)
		}
		pre, body, post := "", first, ""
		if strings.HasPrefix(first, "^(") && strings.HasSuffix(first, ")$") {
			pre, body, post = "^", first[2:len(first)-2], "$"
		}
		for _, alt := range splitTop(body) {
			if strings.Contains(alt, "/") {
				t.Errorf("gate.sh %s -run '%s': the alternative %q holds a '/' inside the first level, so go test never runs its subtests (write '^(A|B)$/^(sub)')", pkg, pat, alt)
				continue
			}
			are, err := regexp.Compile(pre + alt + post)
			if err != nil || !anyMatch(are, tests) {
				t.Errorf("gate.sh %s -run '%s': the alternative %q matches no test of the package", pkg, pat, alt)
			}
		}
	}
	if n < 10 {
		t.Fatalf("found %d go test -run patterns in scripts/gate.sh; the parse is broken", n)
	}
}

func anyMatch(re *regexp.Regexp, names []string) bool {
	for _, s := range names {
		if re.MatchString(s) {
			return true
		}
	}
	return false
}

// splitRunLevels splits a -run pattern at slashes outside brackets and
// parentheses, as testing's splitRegexp does.
func splitRunLevels(s string) []string {
	var out []string
	cs, cp, start := 0, 0, 0
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '[':
			cs++
		case ']':
			if cs > 0 {
				cs--
			}
		case '(':
			if cs == 0 {
				cp++
			}
		case ')':
			if cs == 0 && cp > 0 {
				cp--
			}
		case '\\':
			i++
		case '/':
			if cs == 0 && cp == 0 {
				out = append(out, s[start:i])
				start = i + 1
			}
		}
	}
	return append(out, s[start:])
}

// splitTop splits s at '|' outside parentheses and brackets.
func splitTop(s string) []string {
	var out []string
	depth, start := 0, 0
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '(', '[':
			depth++
		case ')', ']':
			depth--
		case '\\':
			i++
		case '|':
			if depth == 0 {
				out = append(out, s[start:i])
				start = i + 1
			}
		}
	}
	return append(out, s[start:])
}
