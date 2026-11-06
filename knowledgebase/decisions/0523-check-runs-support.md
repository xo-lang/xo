# 0523: `xo check` runs the native support check by default

Status: Accepted (owner, 2026-11-06, with the walk left opt-in: point 1
below; proposed by Xo Lang 1, 2026-11-05, the Xo Lead's queue item
"check vs native gap" from the survey of 2026-11-05)

## Context

`xo check` type checks by the native backends' rules (owner, 2026-10-23),
but what only the SSA builder finds, XO1201 ("not available on the native
backends"), showed only with `xo check --support` (old name `--no-gc`),
and then only for a program: the builder lowered main and what it
reaches. A library, or a function main does not reach, never got its
XO1201 before a build that used it. The aws SDK's reports
(aws docs/reports/xo-issues.md 1, 3, 4, 8) were each found at build time
in code `xo check` had passed; agents check, then build, then learn.

Two problems turned up on the way:

- `--support` turned off decision 0513's per module check
  (`cx.per_module = !support`), so a whole workspace went through one
  checker: `xo check --support` of aws (438 modules) passed the 18 GB
  memory budget.
- Run per member, the support check lowered a shared module's bodies on
  each member's copy of the base, and `Checker.start_member` drops the
  base's per-node records (each name's object, each expression's type) to
  save memory: every name of such a body was a false XO1201 "the name `x`
  here" (57308 of them in a 10 member copy of aws with the front end
  cache off, 832 with it on, the cache replaying some bodies the base
  lowered itself; aws whole: 2828 then 863 on two runs as the cache
  filled).

## Decision

Points 2 to 4 landed with this ADR under the opt-in `--support` (they
fix it and widen it); point 1, the default, the owner accepted on
2026-11-06 with a walk left opt-in.

1. `xo check` runs the support check by default, when the check finds
   no error, on the modules of the directories it is given (`xo check
   .`, `xo check DIR`). A walk (`DIR/...`) keeps it opt-in: `--support`
   turns it on for every walked module (about 10 times the plain check
   on aws, the table below). `--support=false` skips it; `--support-all`
   runs it on every module of the workspace; `--no-gc` is `--support`.
   compiler/cli/core/check.xo run_check (a flag whose absence differs
   from `=false`, flags.xo opt_bool_flag), and cmd/xo TestBind: a `use
   go` program fails plain `xo check`, as a native build does, and
   passes with `--backend=go`.
2. The support check lowers every non generic function of the root
   modules (`Checker.lib_roots`, declared as roots beside main), so a
   library's functions and ones main does not reach get their XO1201.
3. In a per module check (decision 0513) it runs on each member's
   checker, which keeps the base's per-node records
   (`start_member_with(true)`).
4. Which members: the named directories' modules and every module a
   walk (`DIR/...`) finds; `--support-all` runs it on every module of
   the workspace. (With point 1 this is what a default would cover; the
   measured options are below.)

## Measurements

Mac (M-series, 24 GB), aws checkout, warm caches unless said; log
~/xo-lead/notes/lang1/0523-aws-timings.txt. compiler/xo (one module):
0.79 s plain, 1.5 to 1.8 s with the support check. aws/examples/presign:
0.50 s, 0.68 to 0.73 s. A small golden: 0.28 s, 0.33 s.

aws whole workspace (438 modules, 480 for the walk), xo at this change,
two runs each, every run 0 XO1201; the Mac's load was 45 to 70 from
other agents' jobs during the later runs, so the large figures read high:

| command | time | peak RSS |
|---|---|---|
| `xo check .` | 47.9 s, 46.9 s | 3.7 GB |
| `xo check ./...` | 23.9 s, 28.1 s | 3.2 GB |
| `xo check --support .` (the named module) | 51.1 s, 50.0 s | 3.7 GB |
| `xo check --support ./...` (every walked module) | 235.8 s, 224.6 s | 5.0 GB, 3.6 GB |
| `xo check --support --support-all .` | 502.7 s, 515.6 s | 3.4 GB, 3.1 GB |

Before this change `xo check --support .` stopped at the 18 GB budget in
about 9 s, and per member without the records fix reported 2828 and 863
false XO1201 on two runs.

## Consequences

- An agent sees XO1201 at `xo check DIR`, before a build, including in
  libraries; a walk `xo check ./...` shows it only with `--support`, so
  a workspace's CI that walks still needs the flag to see every module's.
- `xo check` of a `use go` program exits 1 (XO1201: Go packages need the
  Go backend) unless `--backend=go`, as a native build would fail.
- `xo check DIR` costs the support check's lowering: compiler/xo 0.79 s
  to 1.5 to 1.8 s, aws `check .` 47 s to 50 s (the table).
- A per member support check keeps the base's per-node records on each
  copy, so it needs more memory per member than a plain check.
- cmd/xo TestCheckSupportPerMember: one real XO1201 in a 12 module
  workspace, the same output on three runs, with the front end cache on
  and off (17243 false XO1201 without the records fix).

## Alternatives

- Support for the whole workspace by default: correct but about 10 times
  slower on aws (the table: 24 to 48 s plain, 225 s and more for every
  module).
- Keep `--support` opt-in: agents do not pass it (the aws reports).
- Lower only what main reaches: misses libraries, which is most of aws.
