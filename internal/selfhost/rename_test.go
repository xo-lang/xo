package selfhost

import (
	"math/rand"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// TestRenamer checks the renamer of canonClosures against the regular
// expression it replaced (two ReplaceAllStringFunc passes), on random
// texts of closure names (some holding separators, as generic instances
// do), other words, and separators.
func TestRenamer(t *testing.T) {
	ren := map[string]string{
		"f.fn1":           "f.fn#1",
		"f.fn2":           "f.fn#2",
		"f.fn1.fn3":       "f.fn#1.fn#1",
		"g.fn10":          "g.fn#1",
		"h[A, B].fn4":     "h[A, B].fn#1",
		"h[A, B].fn4.fn5": "h[A, B].fn#1.fn#1",
		"B].fn4":          "B].fn#1",
		"k(x).fn1":        "k(x).fn#1",
	}
	var keys []string
	for k := range ren {
		keys = append(keys, regexp.QuoteMeta(k))
	}
	sort.Slice(keys, func(i, j int) bool { return len(keys[i]) > len(keys[j]) })
	pat := regexp.MustCompile(`(^|[ (,])(` + strings.Join(keys, "|") + `)([(, \n]|$)`)
	old := func(text string) string {
		for range 2 {
			text = pat.ReplaceAllStringFunc(text, func(s string) string {
				sm := pat.FindStringSubmatch(s)
				return sm[1] + ren[sm[2]] + sm[3]
			})
		}
		return text
	}
	rn := newRenamer(ren)
	words := []string{"f.fn1", "f.fn2", "f.fn1.fn3", "g.fn10", "g.fn1", "f.fn1x", "closure", "%1", "f", ")", ":", "[f.fn1]",
		"h[A, B].fn4", "h[A, B].fn4.fn5", "h[A", "B].fn4", "k(x).fn1", "k", "x).fn1"}
	seps := []string{" ", "(", ",", "\n", ", ", ")", ""}
	rng := rand.New(rand.NewSource(1))
	for range 20000 {
		var b strings.Builder
		for range rng.Intn(12) {
			b.WriteString(words[rng.Intn(len(words))])
			b.WriteString(seps[rng.Intn(len(seps))])
		}
		text := b.String()
		if got, want := rn.pass(rn.pass(text)), old(text); got != want {
			t.Fatalf("%q: renamer %q, regular expression %q", text, got, want)
		}
	}
}
