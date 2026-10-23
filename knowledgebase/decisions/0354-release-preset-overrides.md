# 0354: --release is a preset of settings, each with its own override

Status: Accepted (owner, 2026-10-23, as drafted). Built in two stages:
(1) the override flags and their cache keys, with spec/core.md 8,
toolchain.md and the help; (2) native build information (`xo version -m`,
native replay, the crash report line). `--debug-info=split` is Xo Lang
1's value of the debug info setting (its exact meaning comes with their
change).

## Context

`xo build --release` (decision 0099, spec/core.md 8) changes several
things at once, and only some can be turned back:

| setting | debug build | `--release` | override today |
|---|---|---|---|
| `(debug)` contract clauses | checked | compiled out | none |
| `dbg(expr)` | prints | compiled to `expr` | none |
| parameter values in error frames | recorded | recorded | `--error-args=false`, release only (refused otherwise) |
| debug information | on | off | `--debug-info` (on in release), `-s` (off) |
| runtime profiler (XO_PROF) | in | out | `--profile-support` (in) |
| execution trace recorder | in | out | `--trace-support` (in) |
| LLVM optimization level | split config (tests -O0) | -O2, fixed | none in release |
| workspace overrides (xo.work) | allowed | refused (XO1308) | none (owner's choice A: stays) |

So a release build cannot keep its `(debug)` clauses or `dbg` output, a
debug build cannot drop parameter values, and a release LLVM build cannot
pick its level. And native binaries carry no build information: only the
Go backend's embeds it (rt.SetBuildInfo, which `xo replay` reads), so a
native binary cannot say how it was built.

## Decision

1. **A preset.** `--release` sets the release value of every setting
   below; any setting can then be given either way, before or after
   `--release` (the explicit flag wins over the preset regardless of
   order; between two explicit flags the last wins). Without `--release`
   every setting starts at its debug value and the same flags apply.
   `--release` stays the one name for the preset; `--ci` stays a gate,
   not a mode.
2. **One spelling.** Every setting is `--NAME=VALUE`; a bare `--NAME`
   means on; boolean values accept `on|off` and `true|false`:

   | flag | values | debug | release |
   |---|---|---|---|
   | `--debug-contracts` | on, off | on | off |
   | `--dbg` | on, off | on | off (`dbg(x)` is `x`) |
   | `--error-args` | on, off | on | on |
   | `--debug-info` | on, off, split | on | off |
   | `--profile-support` | on, off | on | off |
   | `--trace-support` | on, off | on | off |
   | `--opt` | O0, O1, O2, O3 | the split config's (tests O0) | O2 |

   `--debug-info=split` (Xo Lang 1) keeps the debug information in a
   file beside the binary. `-s` keeps its meaning (no debug information
   and no local symbols). `--opt` applies to the LLVM backend; the native
   arm64 and amd64 backends have no optimization levels, so `--opt` with
   them is a usage error naming the backend, and the Go backend ignores
   it with a note. The existing flags keep working with their old
   meaning (`--debug-info` = on, `--error-args=false` = off).
3. **Semantics.** Spec 8's rule stays: debug and release builds have
   identical semantics except the `(debug)` clauses. A setting changes
   what a build checks or records, never what a program computes:
   `--debug-contracts=off` in a debug build compiles the `(debug)` clauses
   out exactly as release does (`requires(debug) c else E` no longer
   returns E), and `--debug-contracts=on` keeps them in a release build.
4. **Workspace overrides** stay refused in `--release` and `--ci`
   (XO1308; owner's choice A); there is no override flag for that.
5. **Build information records every effective value.** Each binary
   records the effective value of every setting above (and the existing
   holes, mono, arenas, race, tzdata, stamps, backend and target), not
   the flags typed: the Go backend in its rt.SetBuildInfo string, the
   native backends in a new read-only data blob the runtime finds by
   symbol. `xo version -m BINARY` prints them, `xo replay` rebuilds with
   them, and a crash report (and crash bundle) includes them.

## Consequences

- spec/core.md 8 is rewritten around the preset table; 11.1
  (`--error-args` allowed in any build), 11.4 (`dbg` and `--dbg`), the
  command table (`xo build` flags) and toolchain.md change with it.
- `--error-args=false` in a debug build is no longer an error; the
  message that said it needs `--release` goes.
- Native build information is new work: the blob, its reader in
  `xo version -m`, `xo replay`'s native path, the crash report line.
  A release build keeps it (a few hundred bytes).
- Cache keys: every setting already in the SSA flags (release, prof,
  trace) stays; `debug-contracts`, `dbg`, `error-args` and `opt` join
  the front end, fn and part keys so a build with one setting never
  reuses another's entries.
- Tests: one golden per setting built with each value on a debug and a
  release base (the clause checked or not, dbg printing or not, frames
  with or without values, profiler present or not by symbol, the LLVM
  level by the clang command), and `xo version -m` showing each value.

## Alternatives

- **More release flags without a preset model** (add `--keep-debug-
  contracts`, `--no-error-args`): each one is another special case; the
  owner asked for every setting to work both ways.
- **Profiles in xo.mod** (Cargo's `[profile.release]`): a later step
  could read defaults from xo.mod; the flags and the build information
  come first, and a profile would only set them.
- **Recording the flags typed instead of the effective values**: replay
  would then depend on the defaults of the xo that replays it.
