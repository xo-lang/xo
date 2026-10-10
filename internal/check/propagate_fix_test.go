package check

import (
	"strings"
	"testing"
)

// The "propagate with ?" fix is offered only where `?` can propagate:
// inserting it in a closure or function that cannot fail only traded the
// error for XO0205 (Codex r3, task 07).
func TestPropagateFixOnlyWhereFailing(t *testing.T) {
	cases := []struct {
		name, src string
		want      bool
	}{
		{"pure closure", "enum E { Bad } derive Display\nfn load() -> Int ! E { 1 }\nfn apply(f: fn() -> Int) -> Int { f() }\nfn main(os: Os) {\n  os.stdio.println(\"${apply(fn() -> Int { load() })}\")\n}\n", false},
		{"failing function", "enum E { Bad } derive Display\nfn load() -> Int ! E { 1 }\nfn twice() -> Int ! E {\n  let n: Int = load()\n  n * 2\n}\nfn main(os: Os) {\n  os.stdio.println(\"${twice() ?? 0}\")\n}\n", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ds := checkSrc(t, tc.src)
			got := false
			for _, d := range ds {
				for _, f := range d.Fixes {
					if f.Title == "propagate with ?" {
						got = true
					}
				}
			}
			if got != tc.want {
				t.Fatalf("propagate fix offered = %v, want %v\n%s", got, tc.want, humanAll(ds))
			}
			if len(ds) == 0 {
				t.Fatal("expected a diagnostic")
			}
		})
	}
}

// Passing a test's TestOs where Os is expected points at testing.run_main
// (Codex r3, task 10).
func TestTestOsNotOsNote(t *testing.T) {
	ds := checkFiles(t, map[string]string{
		"m/main.xo":      "fn greet(os: Os) {\n  os.stdio.println(\"hi\")\n}\nfn main(os: Os) { greet(os) }\n",
		"m/main_test.xo": "test \"greets\" {\n  greet(os)\n}\n",
	})
	if !strings.Contains(humanAll(ds), "testing.run_main") {
		t.Fatalf("no run_main hint:\n%s", humanAll(ds))
	}
}
