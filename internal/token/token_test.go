package token

import "testing"

func TestKeywords(t *testing.T) {
	kws := Keywords()
	if len(kws) != 28 {
		t.Fatalf("got %d keywords, core 1.4 lists 28: %v", len(kws), kws)
	}
	for _, k := range kws {
		if Lookup(k) == Ident || !Lookup(k).IsKeyword() {
			t.Errorf("Lookup(%q) is not a keyword", k)
		}
	}
	for _, w := range []string{"derive", "from", "with", "after", "seed", "result", "old", "true", "self"} {
		if Lookup(w) != Ident {
			t.Errorf("%q should be an identifier", w)
		}
	}
}

func TestKindStrings(t *testing.T) {
	for k := Illegal; k < keywordEnd; k++ {
		if k == literalBeg || k == literalEnd || k == operatorBeg || k == operatorEnd || k == keywordBeg {
			continue
		}
		if s := k.String(); len(s) > 5 && s[:5] == "Kind(" {
			t.Errorf("kind %d has no name", int(k))
		}
	}
}
