# 0517: A build checks a dependency's function bodies as the SSA builder reaches them

Status: Accepted (owner, 2026-11-05, through the Xo Lead). The owner
chose both recommendations: (1) a type error in a dependency function the
program never reaches no longer fails `xo build` or `xo run` (`xo check`,
`xo test` and `xo vet` of the dependency stay complete); (2) option (b)
below: `xo get` checks each newly fetched module version whole once and
records the result. Condition: XO_DEP_LAZY becomes the default only after
the remaining soundness piece (inferred effects read across modules,
decision 0496 (c), checked when first read) is done and tested. The
default switch is one push that updates spec/core.md with the semantic
change and runs the touched packages' full suites and a self build (the
default-change rule); until then XO_DEP_LAZY=1 opts in.

## Context

A cold `xo build` of a program using the AWS SDK's s3 and dynamodb
(linux/arm64, main 72b4b94c8, XO_SEPCOMP_BG=0) peaks at 1,100 MB; `go
build` of the same program with aws-sdk-go-v2 peaks at 625 MB. Heap
profiles: at the end of the check 429 MB are in use, 258 MB of them
parse trees; the checker parses and checks every function body of every
module it loads before the SSA builder starts. The SSA builder already
builds only what it reaches from the roots (a queue, compiler/ssa
build.xo run_queue): 3,668 bodies here.

Decision 0247 checks the runtime's bodies (std/native) only when the SSA
builder first builds them (`Checker.lazy_fns`, `check_lazy`), and decision
0351 parses such bodies lazily (their source ranges kept, parsed on
demand). Program modules are still parsed and checked whole.

## Decision (proposed)

A build (`xo build`, `xo run`; not `xo check`, `xo test`, `xo vet`)
treats each non-root module's function bodies as 0247 treats the
runtime's: parsed lazily (0351) and checked by `check_lazy` when the SSA
builder first builds them. A root module (named on the command line or
found under a named directory, `Module.root`) is checked whole as today.

What stays eager, for every module:

- Every declaration: types, signatures, constants, struct fields and
  defaults, derives, interfaces, and every declared effect and capability
  set (`uses ...`, declared errors). A dependency cannot hide a use it did
  not declare: a pub function's effects are its declaration, checked
  against its body when the body is checked, and a call's effects are
  read from the declaration.
- Bodies whose effects are inferred and read from another module (a
  non-pub Display, Debug or Error method another module calls, decision
  0496 (c)): checked when first read, before the whole-program passes
  (as the module check cache's ModEntry.eager does, 6b0ba813d), so no
  effect is read unsolved.

Cache keys: unchanged in kind. The front end cache keys a body by its
module's interface closure (fecache.xo) and is filled as bodies are
built; the module check cache (modcache.xo) already holds modules whose
bodies wait for check_lazy. The parse cache keys lazy and eager parses
apart (ParseEntry.lazy).

On by default since 2026-11-05 (XO_DEP_LAZY=0 turns it off; spec/core.md
10.1 "What a build checks"); it was opt in at 23cfc50ae (check/stmt.xo dep_lazy_mod, check/load.xo
parse_lazy_path), for `xo build` and `xo run` only. The 0496 (c) reads
are covered (stmt.xo dep_cross_eager: a dependency's non-pub `display`
and `debug` methods are checked with the declarations), so a logging
`display` called from another module's `pub fn` without `uses log` is
XO0701 lazy or eager (cmd/xo TestDepLazyBodies; with dep_cross_eager off
the build passed). The module check cache keys dep_lazy (mod_flags): a
lazily checked dependency's entry has no diagnostics for its unreached
bodies. The run cache already keys every XO_ variable.

## Consequences

- Measured (prototype, 2 runs each, linux/arm64 host 1): peak RSS 1,100
  to 811 MB (-26%); cold build 4.49 to 4.72 s down to 4.18 to 4.22 s;
  check phase 435 to 235 ms, heap after check 456 to 194 MB; the program
  builds and runs the same. The SSA builder still builds 3,668 bodies,
  so the next step for memory is freeing a body's tree and records once
  it is built (fe.build adds about 525 MB).
- Semantic change: a type error in a dependency's function that the
  program never reaches no longer fails `xo build` or `xo run` of the
  program. Today it does, as Go's package-level errors do. `xo check`,
  `xo test` and `xo vet` of the dependency itself stay complete, so its
  own CI finds the error. The runtime's bodies already work this way
  (0247, with TestStdChecks checking them whole).
- A published version: `xo get` and `xo mod tidy` do not check today.
  Options for the owner: (a) nothing more (a module's own CI is the
  check, as for the runtime); (b) `xo get` checks a newly fetched
  module version whole once and records the result beside it in the
  module cache, so an error is reported before use and never again;
  (c) a flag (`xo build --check-deps`) for a full check. Chosen: (b),
  since it costs one check per fetched version.
- Diagnostics in a dependency's reached body still stop the build, now
  reported when the SSA builder reaches the body (as 0247's runtime
  errors are), with the same JSON and codes.

## Alternatives

- Lazy parsing alone (0351 for every file, measured as XO_LAZY_PARSE=all):
  8 MB less, since the checker parses every body anyway.
- Separate compilation of dependencies (as std's store, decision 0388):
  removes their check and build from every program build after the
  first, but not from the cold build measured here; complementary.
- Freeing trees after the SSA build only: helps the fe.build peak, not
  the check's 260 MB; also complementary.
