# 0631: Warm build sequences in the gate and the 2-hourly check

Status: Accepted (lead, 2026-11-14: asked for the test, its gate subset for compiler/ changes and its run in the 2-hourly main check). Written by Xo Lang 2.

## Context

0.51.0 needed a respin (2026-11-13): after building one std/log program,
building another failed with XO1201, because a replayed front end cache
entry's interface type lacked its method tables (fixed by b8c13a7c8). The
bug was deterministic and hit every user on their second std/log build,
yet no check saw it: release builds are cold (every cache off), and every
cache test builds one program per fresh cache or edits one program. Nothing
built two different programs through one cache with the default settings,
which is what users do (release-0.51.0 retro).

## Decision

internal/arm64 `TestWarmBuildSequence` builds std goldens through caches that
other programs filled, with xo's default cache settings (every XO_*CACHE*
variable and XO_SEPCOMP removed from the environment):

- pairs: every ordered pair of goldens that share a std module, A then B,
  each pair on its own fresh cache;
- sequences: the set one after another through one cache, in name order,
  reversed and shuffled (XO_WARMSEQ_SEED; the day number by default).

Each build must be the same file as its cold build (every cache off,
XO_SEPCOMP=0; the output name, the only other differing byte, is kept the
same), its run must match want.txt, and no front end cache entry may fail
to replay (xo warns and falls back; a known miss is listed with its owner
and logged until its fix removes it).

Sets (`XO_WARMSEQ`): `gate` (default; the std/log goldens and all their
ordered pairs, one golden of ten common modules in the sequences: 16
programs, 30 pairs, about 50 s on the Mac); `modules` (one golden per std
module any golden uses and the pairs among the first 3 of each: 97
programs, 188 pairs, about 3 minutes); `all`.

- scripts/gate.sh runs the gate set after the named steps when the branch
  changes compiler/ (not compiler/macho, compiler/wasm or `_test.xo`), and
  as the named step `warmseq`; XO_GATE_WARMSEQ=0 skips it.
- scripts/lead/hourly.sh runs the modules set in the 2-hourly main check.

## Consequences

- A compiler/ gate grows by about 50 s (16 programs built warm and cold,
  30 pairs), under the same machine-wide gate slots.
- The 2-hourly check grows by about 3 minutes.
- At its first runs it found two more misses of the same family on main,
  both falling back with a warning (no wrong code): `log.record` after
  log_error or log_lazy (an itab slot's function type not recorded as
  read; Lang 3's 715369420), and `http.Server.serve` closures after
  websocket_deadline (`read type Chan[()]`). Both are listed as known
  until their fixes land.
- A sequence alone can miss a bug: in the reversed order a later std/log
  build repaired the entry before log_error read it, so the pre-fix xo
  passed the sequences and failed only the pairs.

## Alternatives

- Only sequences: cheaper, but shown to mask the respin's bug.
- All pairs of all std goldens (311 programs, about 96,000 pairs): far
  too slow; pairs within a module's goldens are where shared entries are.
- A release-only check (CUT.md 1b): finds the bug after the tag, as 0.51.0
  did; the per-push gate finds it at the change that makes it.

## Verification

- Fails at b8c13a7c8's parent 72c2e3881 (pairs: log_error after log_lazy,
  log_state or log_with: XO1201); passes on main a08085cde (with the
  log.record miss logged as known).
