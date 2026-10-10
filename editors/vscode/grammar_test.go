// Package vscode holds the VS Code extension for Xo. The extension itself is
// JSON; this test keeps its grammar in step with the lexer. The tokenization
// tests run with `npm test` (see knowledgebase/infrastructure/editors.md).
package vscode

import (
	"encoding/json"
	"os"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/xo-lang/xo/internal/token"
)

type rule struct {
	Name     string `json:"name"`
	Match    string `json:"match"`
	Patterns []rule `json:"patterns"`
}

type grammar struct {
	ScopeName  string          `json:"scopeName"`
	Repository map[string]rule `json:"repository"`
}

func loadGrammar(t *testing.T) grammar {
	t.Helper()
	data, err := os.ReadFile("syntaxes/xo.tmLanguage.json")
	if err != nil {
		t.Fatal(err)
	}
	var g grammar
	if err := json.Unmarshal(data, &g); err != nil {
		t.Fatalf("grammar is not valid JSON: %v", err)
	}
	return g
}

// TestKeywordsMatchLexer checks that the grammar's keyword rules name exactly
// the keywords of core 1.4, as the lexer defines them.
func TestKeywordsMatchLexer(t *testing.T) {
	g := loadGrammar(t)
	kw, ok := g.Repository["keywords"]
	if !ok {
		t.Fatal("grammar has no keywords rule")
	}
	word := regexp.MustCompile(`[a-z]+`)
	var got []string
	for _, p := range kw.Patterns {
		// Each match is \b(?:a|b|c)\b or \bword\b.
		m := strings.TrimSuffix(strings.TrimPrefix(p.Match, `\b`), `\b`)
		m = strings.TrimSuffix(strings.TrimPrefix(m, "(?:"), ")")
		for _, alt := range strings.Split(m, "|") {
			if !word.MatchString(alt) || word.FindString(alt) != alt {
				t.Fatalf("keyword rule %q has a non word alternative %q", p.Name, alt)
			}
			got = append(got, alt)
		}
	}
	want := token.Keywords()
	slices.Sort(got)
	slices.Sort(want)
	if !slices.Equal(got, want) {
		t.Errorf("grammar keywords differ from internal/token\n got: %v\nwant: %v", got, want)
	}
}

type grammarEntry struct {
	Language  string `json:"language"`
	ScopeName string `json:"scopeName"`
	Path      string `json:"path"`
}

func TestManifestPointsAtGrammar(t *testing.T) {
	g := loadGrammar(t)
	data, err := os.ReadFile("package.json")
	if err != nil {
		t.Fatal(err)
	}
	var pkg struct {
		Contributes struct {
			Languages []struct {
				ID         string   `json:"id"`
				Extensions []string `json:"extensions"`
			} `json:"languages"`
			Grammars []grammarEntry `json:"grammars"`
		} `json:"contributes"`
	}
	if err := json.Unmarshal(data, &pkg); err != nil {
		t.Fatalf("package.json: %v", err)
	}
	c := pkg.Contributes
	if len(c.Languages) != 1 || c.Languages[0].ID != "xo" || !slices.Contains(c.Languages[0].Extensions, ".xo") {
		t.Errorf("package.json must register language xo for .xo files: %+v", c.Languages)
	}
	i := slices.IndexFunc(c.Grammars, func(e grammarEntry) bool { return e.Language == "xo" })
	if i < 0 || c.Grammars[i].ScopeName != g.ScopeName || c.Grammars[i].Path != "./syntaxes/xo.tmLanguage.json" {
		t.Errorf("package.json grammar entry for xo does not match the grammar: %+v", c.Grammars)
	}
	for _, e := range c.Grammars {
		if _, err := os.Stat(e.Path); err != nil {
			t.Errorf("package.json grammar %s: %v", e.ScopeName, err)
		}
	}
}
