# An xo older than an intrinsic cannot build the compiler/xo that uses it

Status: Observed (2026-10-13, decision 0215).

## Symptom

After merging main (syscalls2, decision 0206 stage 2) into the branch
that deleted the Go compiler, `go run ./scripts/xobuild` failed with
every seed older than that merge: the link of compiler/xo stopped at
`Undefined symbols for architecture arm64: _xo_call_c2, _xo_sys_clone,
_xo_sys_words`. The go-compiler-final tag's xo1 (87873aa5) is older too.

## Cause

The merge added intrinsics (declared in std/native, lowered by
compiler/ssa and the emitters) and used them in std/native code that
xo itself is built from. An older xo does not know them: it compiles
the calls as calls of external functions, which no C file defines.
Before decision 0215 the Go driver had the same intrinsics (every change
landed on both sides), so a Go built xo1 always knew them.

## Fix

Two hops (decision 0216 item 2): the older xo builds an intermediate
xo, the new compiler (main 4f792c55's compiler/xo) against an older std
that does not use the intrinsics (the go-compiler-final std), and the
intermediate, whose emitters lower them, builds today's sources (311 s
each on the Mac). scripts/seed-chain.txt records the hop; internal/xotool
takes it when a seed fails, and scripts/bootstrap-xo.sh takes it with
XO_SEED_CHAIN=file. A change that makes xo's own sources use something
an older xo cannot compile records its hop in the same change.

## Avoiding the seed bump: a C shim (2026-10-14, work item B02)

An intrinsic that the emitters lower inline but that an older xo
compiles as a call of `xo_<name>` can land without a new seed when the
C runtime keeps a definition of `xo_<name>` with the same meaning:
`rt.static_word` (decision 0207) has `xo_static_word` over a C table in
std/llrt/xo_sys.c. The seed's s1 calls it; s2 and every later build
lower it inline, so the fixed point holds. Delete the shim with the
first seed that has the intrinsic. This does not work for a change the
seed's SSA builder rejects in std code (a new type accepted by
`rt.obj_at`, say): that is a build error, not a link error, and needs
the seed first.

Seen again 2026-10-17 (Xo Lang 3): an rt.map_parts intrinsic used in
std/native/http.xo stopped the build of xo itself at the seed
(`XO1201 ... the intrinsic map_parts`): xo imports std/http, so every
std module xo compiles must use only the seed's intrinsics. The change
went into compiler/ssa instead (small maps with no table), which needs
no std change.

Observed again, for Windows imports (2026-10-22, decision 0343 stage 2,
18efab744): crash.xo called a new Windows function,
SetUnhandledExceptionFilter, through sys_windows.xo. Xo's PE linker
resolves imports by a table compiled into xo (compiler/macho
peimports.xo), so the seed (0.25.0) and every cached xo built before
that commit failed to link any windows/amd64 program ("undefined symbol
SetUnhandledExceptionFilter"), and the CI windows job's xobuild found no
builder (run 39826368911). Fix without a new seed: crash.xo finds the
function in kernel32's export table at run time (k32_proc, one
forwarder followed) and calls it through rt.call_c2. A new
sys_windows.xo function is usable only once the seed's peimports.xo has
it; check with the seed: `xo build --std=NEW_STD --backend=amd64
--target=windows/amd64 hello`.
