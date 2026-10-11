# 0156: Supply-chain hardening

Status: Accepted, implemented (2026-10-11). All six items built and
tested; the attack replication measured 10 of 15 classes blocked
(`bench/results/supply-chain-2026-10-11.md`). Scope set by the owner on
2026-10-11:
only the consumer's vouches count for `use c ... uses none`; an effect
lock in xo.lock by default (`--accept-effects` to widen); capability
attenuation (for example `os.net.only(hosts)`); `xo audit`; a
replication of the Backstabber's Knife Collection attacks; the ambient
logging channel.

## Context

Supply-chain safety is meant to be a headline property of Xo, and the
owner wants it measured. What Xo had before this decision:

- Authority only through capability values rooted at `main(os: Os)`
  (decision 0004), and `uses` effects checked across modules.
- Effect ceilings per dependency in xo.mod (`require ... uses none`,
  XO1309, decision 0086), opt-in.
- `xo publish` refuses a minor or patch release that adds an effect
  (XO1310).
- The `go` effect of `use go` cannot be declared away (decision 0041).
- No code runs at fetch or build setup: no install scripts, no hooks.

Known gaps:

1. A dependency vouches for itself. `use c "x.h" uses none` in a
   dependency says its C code has no effects, and every module that
   imports it believes that. Likewise a dependency's `use go
   "os/exec"` is charged the honest `go` effect, but nothing makes the
   consumer look at it: `main` is not `pub`, so its effects are
   inferred and `go` flows into it silently.
2. Ceilings are opt-in. An update of a dependency that starts using
   `net` passes silently unless the consumer wrote a ceiling first.
3. Effects are coarse: `net` means any host, `fs` any path, `env` every
   variable. A library handed `os.net` to talk to one API can talk to
   every host.
4. Logging is ambient (core 6.1): any code can write log records
   without a capability or an effect. A dependency handed a secret (a
   token it needs) can write it to the logs, which are commonly shipped
   to a third party.

Peers. Flix reports an effect-aware package manager that catches 48 of
the 51 attacks it replicated from the Backstabber's Knife Collection
(Ohm et al., DIMVA 2020). WASI components grant capabilities per
component. LavaMoat (JavaScript) confines each package to a policy file
of globals and builtins, generated and then reviewed. Capslock (Go)
reports the capabilities a package's call graph reaches. PermRust
proposes permission annotations for Rust crates. Java's
SecurityManager, the long running runtime permission system, was
removed (JEP 486, JDK 24): its checks lived in the libraries, were
expensive, and almost nobody configured them. The lesson taken here:
authority must be structural (values and checked signatures), defaults
must protect without configuration, and the remaining runtime checks
must be few and cheap.

## Decision

### 1. Only the consumer vouches for a dependency's escape hatches

An escape hatch is code whose authority the checker cannot see: a
`use c` line (C code), or a `use go` of a Go package that is not pure.
The pure Go packages are those already marked `replay rerun` by
default (core 4.4: `strconv`, `strings`, `bytes`, `unicode`,
`unicode/utf8`, `unicode/utf16`, `math`, `math/bits`, `regexp`,
`regexp/syntax`, `path`, `html`, `net/url`, `encoding/hex`,
`encoding/base64`, `encoding/base32`).

- In a module the build gets as a required version (the module cache
  or vendor/), every escape hatch needs a `vouch` line in the xo.mod of
  the consuming module: the main module, or any module of the active
  workspace. Vouch lines of a dependency's own xo.mod are ignored: a
  dependency cannot vouch for itself or for its dependencies.
- The main module's own `use c ... uses none` and `use go` lines, and
  those of workspace modules, stay trusted: they are the developer's
  own code.
- Syntax, after the existing `require` grammar:

  ```
  vouch github.com/acme/zip v1.2.0 c "zlib.h" uses none
  vouch github.com/acme/img v0.3.1 c "png.h"            // accepted as ffi
  vouch github.com/acme/pg  v1.0.0 go "github.com/jackc/pgx/v5"
  ```

  `vouch <module path> <version> c "<header>" [uses <effects|none>]`
  and `vouch <module path> <version> go "<Go import path>"`. The header
  is the string of the dependency's `use c` line. The version is the
  exact version the build selects: a vouch is a review of one version's
  code, so an upgrade needs a new review.
- Effects of a vouched `use c` binding are the vouch's `uses` (the
  consumer's word), not the dependency's: without `uses` the binding
  performs `ffi`. If the dependency's functions then perform more than
  they declare, that is XO0701 in the dependency, which is right: the
  consumer disagrees with the dependency's claim.
- `xo get` and `xo mod accept-effects` also refuse, before writing, a
  selected version with an escape hatch the main module does not vouch
  for (XO1316 from a syntax scan, `mod.ModuleHatches`), with or without
  `--accept-effects`, which accepts effects only (added 2026-10-11, after
  the website session found `xo get --accept-effects` let an update add
  an unvouched impure `use go`; `xo mod tidy` still records and leaves
  the report to the check).
- Unvouched, or vouched for another version: error **XO1316** on the
  dependency's `use` line, naming the vouch line to add (or the stale
  one) and the xo.mod to add it to. An unvouched `use c` binding is
  charged its worst case, `ffi`, whatever its `uses` says. There is no
  machine fix: a vouch is a human review of foreign code, and `xo check
  --fix` must never apply one.
- A malformed vouch line is XO1301.

### 2. An effect lock in xo.lock, on by default

- xo.lock records, for each selected module version (not workspace
  modules), the effects its exported API can reach:
  `<module path> effects <labels|none>`, for example
  `github.com/acme/billing effects net.dial,stdio`. The set is the union
  of the effect labels in the `uses` clauses of the module's `pub`
  functions and methods (every package, test files excluded). The
  checker already makes every effect a function reaches appear in a
  `pub` caller's `uses`, and escape hatches are covered by section 1,
  so the declared surface is the reachable set; it is computed from
  syntax, so writing it needs no type check.
- The line is keyed by module path, not version, so it survives an
  upgrade: the lock remembers what the consumer accepted, and the new
  version is compared with it.
- Every command that loads the program (`check`, `build`, `run`,
  `test`, `vendor`, ...) compares each module's current effects with
  its line: an effect not admitted by the locked set (`fs` admits
  `fs.read`) is error **XO1317** on the lock line, listing the added
  effects. A module with no effects line (a lock written before this
  decision) is not checked; `xo audit` reports it as unlocked.
- `xo get` and `xo mod tidy` write the lines: a module without a line
  is recorded as it is (first lock creation records without failing), a
  module whose effects narrowed gets the narrower line, and a module
  whose effects widened fails the command with XO1317, writing nothing
  (xo.mod is restored).
- Accepting a widening is a separate, explicit step: `xo mod
  accept-effects [<module path>...]` rewrites the lines of the named
  modules (default: all) to their current effects and prints what
  changed; `xo get --accept-effects <path>@<version>` does the upgrade
  and the acceptance together. Rationale: `xo check` stays read only,
  `xo build` and `xo check` take no flag that accepts (a flag on build
  would end up in CI scripts and accept silently), and the acceptance
  shows up as an xo.lock diff a reviewer can see.

### 3. Capability attenuation

Narrowed capabilities are values of the same capability type whose
operations outside the narrowed authority fail at run time:

- `os.net.only(hosts: List[Str]) -> Net`: `dial` and `listen` succeed
  only for an address whose host matches an entry: an exact host
  (`api.example.com`, `127.0.0.1`, compared without case), a
  `*.example.com` entry (any subdomain), or `host:port` (that host and
  port only). A listen address with no host (`:8080`) has host
  `0.0.0.0`. Anything else fails with the new `NetErr.Denied(addr)`
  before any name lookup. An HTTP client made from such a `Net`
  (`http.client`) checks each request's host and port the same way and
  fails with `ClientErr.Connect("denied by Net.only: ...")`;
  `sql.open` with an attenuated `Net` fails (the driver dials by
  itself, so the allowlist cannot be enforced; fail closed).
  `only` of an attenuated `Net` intersects the lists.
- `os.env.only(names: List[Str]) -> Env`: `get` of another name is
  `None`, `vars` holds only the listed names, `home` is
  `Err(Missing("HOME"))` unless `HOME` is listed; `args` and `program`
  are unchanged.
- `os.proc.only(cmds: List[Str]) -> Proc`: `run` of a command not
  listed (compared as written) fails with the new
  `ProcErr.Denied(cmd)`; `exit` and `set_grace` are unchanged.
- Files already have attenuation: `os.fs.sub(dir)`, `read_only()`,
  `write_only()` (core 6.3).
- Test fakes (`FakeNet`, `FakeEnv`, `FakeProc`) attenuate the same way.
- Visible statically: `xo audit` lists every attenuation call (`only`,
  `sub`, `read_only`, `write_only`) in the consuming modules with its
  arguments when they are constants (a list literal of string
  literals), and every capability argument of a call into a dependency
  with whether it is attenuated (the argument is an attenuation call,
  or a local bound once to one).
- Backends: the Go backend implements all three. The native backends
  (arm64, LLVM via std/llrt) carry no state in their `Net`, `Env`, and
  `Proc` values yet, so `only` there is XO1201 (planned), from the
  native code generator like every XO1201.

### 4. `xo audit`

`xo audit [--json] [--dir=<dir>] [--no-gc]` loads and checks the module
like `xo check` and reports, per dependency (direct and indirect): its
version, its current effects, its locked effects and the lock state
(`ok`, `widened`, `narrowed`, `unlocked`), its effect ceiling, its
escape hatches (`use c` and impure `use go` lines, with the claimed and
effective effects and whether a vouch covers them), plus the
attenuation sites and the capabilities passed into dependencies, the
vouch lines that match nothing, and the diagnostics of the check. JSON
follows the conventions of the other commands (one object, `--json` or
a non terminal stdout). Exit status 1 when the check has errors, else 0.

### 5. Measured: the Backstabber's Knife Collection replication

`cmd/xo` `TestSupplyChainAttacks` replays attack classes from the
Backstabber's Knife Collection taxonomy (malicious updates of existing
packages and new name-confusing packages; exfiltration, droppers,
backdoors, sabotage, financial gain; triggers at install, import,
test, and run time) as small malicious Xo dependencies and consumers,
each in two consumer configurations: default (no ceilings, no
attenuation) and hardened (a ceiling equal to the dependency's
documented effects, attenuated capabilities). It records the first
stage that blocks each attack: fetch (nothing runs), check, lock, run
time, or not blocked, with the reason. Measured 10 of 15 classes blocked
(fetch 1, check 7, lock 1, runtime 1); the 5 not blocked are
out-of-scope logic/DoS (2), default-posture choices a ceiling or
attenuation closes (2), and a wrong human vouch (1). Results:
`bench/results/supply-chain-2026-10-11.md`.

### 6. Logging is an effect: `log`

- `log` is an effect label (core 6.1) with no capability: `std/log`'s
  `debug`, `info`, `warn`, and `error` perform it. Like every effect it
  is inferred for functions that are not `pub` (so `main` and helpers
  are unchanged) and must be declared by `pub` functions and interface
  methods. A library that logs says so in its signatures, ceilings can
  forbid it (`require ... uses net.dial` no longer admits logging), the
  effect lock catches an update that starts logging, and `xo audit`
  lists it.
- `log.configure` keeps `uses stdio`. std's own records (a handler
  fault in `std/http`, `ignore_err`) stay attributed to std and are not
  the effect.
- Attribution: the `log` effect makes a dependency's logging visible in
  its signatures, ceilings, the effect lock, and `xo audit`. Tagging
  each emitted record with the dependency that wrote it is left for
  later (it needs the code generator to thread the module into each log
  call); it is not implemented.
- Migration: XO0701 whose only missing effect is `log` on a function
  declaration carries a machine fix that adds `log` to its `uses`, so
  `xo fix` rewrites existing code.

### New diagnostics

- XO1316 (error, checker): an escape hatch (`use c`, `use go` of an
  impure package) in a required module without the consumer's `vouch`
  for that version.
- XO1317 (error, modules): a dependency's effects are wider than its
  `effects` line in xo.lock.

## Consequences

- A dependency gets no authority the consumer did not grant or review:
  capabilities for Xo code, vouches for foreign code, the lock for
  what changes on update.
- Every dependency that uses an impure Go package needs one vouch line
  per version in each consumer; upgrades of such dependencies need a
  re-review. This is the intended cost.
- Logging in `pub` functions needs `uses log`; existing code migrates
  with `xo fix`.
- New xo.lock lines; older toolchains reject them as malformed
  (XO1301). Fine before 1.0.
- `NetErr` and `ProcErr` gain a `Denied` variant (appended, so the
  existing tags keep their numbers); an exhaustive `match` on them needs
  the new arm.
- What stays open (measured in the replication): pure code that
  computes the wrong answer (a payment address swapped, a crypto miner
  burning CPU, an infinite loop), misuse of authority the consumer
  granted unattenuated, and a new malicious package adopted with its
  effects recorded on first lock.

## Alternatives

- Trust dependency claims and rely on `xo audit` to surface them:
  leaves gap 1 open by default.
- Effect lock as an opt-in, or a ceiling written automatically into
  xo.mod: a ceiling in xo.mod is a policy the human writes; the lock is
  a record the toolchain keeps. Keeping both separate lets a ceiling
  stay a deliberate statement.
- `--accept-effects` on `xo build` and `xo check`: rejected, see 2.
- Runtime permission manifests per package (LavaMoat, Deno flags):
  coarser than per call effects checked at compile time, and need a
  runtime policy engine; attenuation keeps the runtime part to the
  three capability types where the static type is too coarse.
- Logging through a capability passed from `main` (a `Log` value):
  precise, but every library function that logs would take one more
  parameter; an effect gives the same visibility (signatures, ceilings,
  lock, audit) without the plumbing.
- Logging stays ambient but dependency records are filtered or
  attributed only: makes a leak traceable, but does not prevent it and
  does not show in signatures.
