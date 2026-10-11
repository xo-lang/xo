package check

import (
	"testing"

	"github.com/xo-lang/xo/internal/mod"
)

// TestEffectCeiling checks XO1309 with the ceiling of an xo.work `use`
// line (cmd/xo's TestEffectCeilings covers xo.mod requires with git).
func TestEffectCeiling(t *testing.T) {
	t.Setenv("XO_WORK", "")
	tick := "pub fn now() -> Int uses clock { 1 }\npub fn pure() -> Int { 2 }\npub fn apply[fx: Effects](f: fn() -> Int uses fx) -> Int uses fx { f() }\n"
	world := func(ceiling, main string) map[string]string {
		return map[string]string{
			"xo.work":      "xo preview\nuse ./app\nuse ./tick" + ceiling + "\n",
			"app/xo.mod":   "module example.com/app\nedition preview\n",
			"app/a/a.xo":   main,
			"tick/xo.mod":  "module example.com/tick\nedition preview\n",
			"tick/tick.xo": tick,
		}
	}
	runCases(t, []ruleCase{
		{name: "above the ceiling", files: world(" uses none", "use example.com/tick\n\npub fn f() -> Int uses clock { tick.now() }\n"), want: []string{"XO1309"}},
		{name: "function value above the ceiling", files: world(" uses none", "use example.com/tick\n\npub fn f() -> Int uses clock {\n  let g = tick.now\n  g()\n}\n"), want: []string{"XO1309"}},
		{name: "within the ceiling", files: world(" uses clock", "use example.com/tick\n\npub fn f() -> Int uses clock { tick.now() }\n")},
		{name: "pure call", files: world(" uses none", "use example.com/tick\n\npub fn f() -> Int { tick.pure() }\n")},
		{name: "callback effects are the caller's", files: world(" uses none", "use example.com/tick\n\npub fn f(c: Clock) -> Int uses clock { tick.apply(fn() -> Int { c.now().unix() }) }\n")},
		{name: "no ceiling", files: world("", "use example.com/tick\n\npub fn f() -> Int uses clock { tick.now() }\n")},
	})
}

// TestCeilingLabels keeps the effect labels an xo.mod ceiling may name
// equal to the checker's (decision 0086 step 3).
func TestCeilingLabels(t *testing.T) {
	have := map[string]bool{}
	for _, l := range mod.EffectLabels {
		have[l] = true
		if !effectLabels[l] {
			t.Errorf("mod.EffectLabels has %q, the checker does not", l)
		}
	}
	for l := range effectLabels {
		if !have[l] {
			t.Errorf("the checker has effect %q, mod.EffectLabels does not", l)
		}
	}
}
