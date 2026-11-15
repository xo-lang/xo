# A new TargetArch or TargetOs variant: a clean check misses matches

Status: Observed (Xo Lang 1, 2026-11-08, decision 0571 stage 1; again by Lang 3, 2026-11-15, any std enum: ClientErr.Tls)

## Symptom

`go run ./scripts/xocheck compiler/xo` passed with `TargetArch.Riscv64`
added, yet 11 `match target.arch` blocks in std/native/llrt still had no
arm for it (libcthr, libcsys, libcio, ifaces, host, dns, cabi_nolibc
twice, linux.xo's vDSO name, unwind.xo's win_raise). Each is XO0301 the
first time a build reaches it: a riscv64 build reaching libcsys, or a
windows/amd64 build reaching win_raise.

## Cause

Checking is lazy: a function body is checked when something the checked
program uses reaches it. A check of compiler/xo reaches only the llrt
files the compiler links for the host; nolibc, Windows only and network
code it never calls stays unchecked.

## Fix

After adding a variant, list every exhaustive match on the enum, not the
checker's errors: a scan of each `match target.arch {` block for an arm
naming the variant or `_ =>` (stage 1 of 0571 used a short Python scan
over std/ and compiler/). Then give
each the right arm by reading it: most take an existing arch's value
(the asm-generic system call numbers are arm64's), some differ (the
generic open flags are x86_64's).

Repeat the scan after every rebase: a match that main adds while the
branch is open (cabi_wincrt.xo's xo_win_start, 2026-11-08) has no arm
for the new variant, and only a build that reaches it fails (the
gate's wasm goldens and windows/amd64 cross builds, not the riscv64
runs).

## A generated block: fix its generator

Observed 2026-11-08 (tier 61): 63e4ddb63 added `TargetArch.Riscv64` to
sha3.xo's portable rounds arm by hand, but that arm is
scripts/gen/keccaklazy's output, so TestFieldGeneratorsInSync/keccaklazy
failed until the generator emitted the arm too (c2bd0dfc5). Before editing
a match, check whether the file holds a generator's output
(internal/llvm/fieldgen_test.go lists them: x25519fe64, nistecmul4p,
keccaklazy; `grep -rl 'TargetOs\|TargetArch' scripts/gen` finds the ones
that emit a target match), and fix the generator, then regenerate; gate
on TestFieldGeneratorsInSync.

## Any std enum, not only targets (Lang 3, 2026-11-15)

`http.ClientErr.Tls` (decision 0644) built xo, passed the http goldens and
the SDK checks, and still failed a full gate in 11 tests on one site:
std/grpc.xo's `from_client_err` listed every old variant and no `Tls`. Only
programs that use std/grpc check its bodies (merged std modules are checked
lazily too), so the gate's grpc goldens, TestStdModules grpc, TestStdChecks,
sepcomp and island builds failed together, with one root cause. Before
pushing a new variant of any std enum, grep std/ for a variant the match
must already list (`InvalidUrl(` for ClientErr) and read each hit: an arm
for the new variant or `_ =>`. Open enums (decision 0649) do not help
inside std: std is one package, so its own matches stay exhaustive.

