# 0657: select gate goldens by their built binaries

Status: Accepted (owner, 2026-11-17, on the lead's recommendation: off by default, turned on per host only after a measured net gate-time win). Proposed by Xo Lang 2, 2026-11-15

## Context

The gate's golden selection followed the LLVM IR (llvm_goldens:
TestGoldenIRManifest, a manifest of every golden's IR hash at the merge
base and at HEAD) and, for the arm64 tests, reused that list unless the
change touched the native backend, when every golden ran. A compiler
change alters the IR of nearly every golden (647 to 719 of about 720 in
host 3's cached manifests, bench/results/gate-time-2026-11-14.md), so a
compiler change ran every golden, and making the manifests cost 1,000 to
2,200 s a gate.

Xo's builds are deterministic, and the binaries move far less than the
IR. Building every golden for linux/arm64 with the parent's and the
change's xo (~/xo-lead/notes/lang2/artifact-keyed-tests.md, 2026-11-15):

- Lang 1's E (f25c27369, compiler/ssa and compiler/arm64): 14 of 743
  binaries differ, each a real code change in the Str-view goldens it
  targets;
- Lang 3's openenum (2e71c2211..27fee28f2, compiler, 50 std and std/rt
  files): 8 of 742 differ, all goldens that link C.

Builds with the same xo twice, with caches on (cold and warm) or off, and
with the golden's directory moved were identical. What changes a binary
without a code change: the build-info id (compiler/xo/binfo.xo: a hash
of every source file's digest and the runtime's files, written twice into
.rodata, so any std edit changes it everywhere), the std path (in the
debug information and strings), and the C step's path in C-linked
goldens.

## Decision

gate.sh's arm64 step selects its golden tests (TestGoldenRun,
TestGoldenRunSpill, TestGoldenRunHeapGuard, TestGoldenPassesOff,
TestDebugInfoSameCode) by the binaries themselves (bin_select): the merge
base's xo is built in a worktree; every golden is built by it with the
base's std and by the branch's xo with the branch's std, each std copied
to one path in turn, the branch's golden sources at one path, both sides
to one output path (a Mach-O's signature and UUID take it), every cache
off; scripts/golden-bin-select.py then names

- the goldens whose binaries differ with the build-info id masked;
- those a side could not build (compile-error goldens, new goldens);
- those that use C (always: the C step's path differs from build to
  build);
- and gate.sh adds those whose own directory the branch changes.

The package's other tests run while the selection is made. Every golden
runs when the change touches the golden tests (internal/arm64 or
internal/llvm top-level files), internal/xotool, scripts/xobuild, the
selector, or go.mod/go.sum, when the base's xo does not build, with
XO_GATE_LLVM=all or XO_GATE_TESTS=all, and with XO_GATE_BIN_SELECT=0 (the
escape hatch; `force` skips the touched-files check, for proving the
selection on a branch that changes it). The continuous tier on main
(scripts/lead/ctier.sh) runs every golden, so a selection miss shows
there within a tier run (about 2 hours).

Addendum 2026-11-15 (lead): measured on E on host 2 (18 CPUs), the
arm64 step took 1,006 s with the selection (its package tests 1,000 s
beside 801 s of selection builds; the 20 selected goldens 5 s) and 912 s
without it: the arm64 goldens are not that step's critical path, and the
1,486 builds compete with the other tests for CPU. The selection is
therefore off by default (`XO_GATE_BIN_SELECT=1` turns it on), and a
selection mode turns on by default only when the same change, measured
with it on and off on the host it runs on, shows a net gate-time win.

Stage 2, after this one is measured: the llvm step keyed the same way on
llvm-backend binaries, replacing TestGoldenIRManifest.

## Consequences

- A compiler change no longer runs every arm64 golden: E's gate runs its
  14 plus the always-run classes where it ran all 743.
- The selection costs one base xo build (about 2 minutes on a host with
  the seed) and two builds of every golden (743 arm64 builds took 173 to
  384 s a side at half of host 3's CPUs).
- The variants (spill, heap guard, passes off) are keyed by the plain
  build, not their own: a change visible only in a variant's code path
  (a pass that runs only with the others off) is missed until the tier
  runs. Measured misses would make the variants build their own keys.
- A flake in a golden the selection leaves out is not seen per push; the
  tier sees it.

## Alternatives

- The LLVM IR selection as before: sound, but compiler changes select
  nearly everything, and the manifests cost more than the builds.
- A result store keyed on the binary's hash (Go's test cache; ADR 0439
  does this for xo test): replays across branches and pushes, not only
  against the merge base; more machinery (a shared store per host class,
  expiry), and this decision's per-gate comparison gets most of the
  saving first.
- `--binfo-stable` builds instead of masking: it drops the source digests
  but not the runtime files' sums, so a std/rt edit still changed every
  binary (openenum: 742 of 742).
