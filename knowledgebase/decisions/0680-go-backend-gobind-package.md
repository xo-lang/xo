# 0680: Go backend `use go` adapters in their own package

Status: Accepted, alternative A (owner, 2026-11-16; proposed by Xo Lang 2, 2026-11-16)

## Context

Every Go backend build compiles the generated runtime package `xoprog/rt`
(about 240 files, 25 to 36 CPU seconds and up to 1.2 GB on a cold cache).
Go's build cache shares a compiled package between builds only when its
sources are identical, and two things make them differ per program:

1. decision 0551 prunes the protocol methods (XoGen, XoEq, XoHash, XoCmp) of
   rt's generic types that the program never reaches, which is what makes a
   large program's own package cheaper to compile (armnetwork: 14.7 GB and
   96 s to 10.8 GB and 71 s);
2. the `use go` adapters (`rt/xo_gobind_*.go`) are written into the rt
   directory, so a program with one has an rt of its own.

On 2026-11-16 the hourly `go test -short` timed out: with std/rt changed,
26 concurrent builds compiled xoprog/rt at once (load 60, 19 GB, 120 s
build timeouts). Decision (1) of that report, a lock per Go cache and rt
variant in go_build (gotchas/go-backend-cold-rt-compiled-per-build.md,
commit 22912c0a8), makes concurrent builds of one variant compile it once. This
ADR is about the number of variants.

Measured (Mac, 2026-11-16, l2-rtlock feedd654b):

- the 138 internal/gen goldens (Go trees written with `--go-tree` and a go
  that builds nothing): 30 rt variants; with XO_GO_PROTO=all (every method
  kept) 18; the only files that differ then are `xo_gobind_*.go` (44 names,
  1 to 14 programs each);
- ignoring `xo_gobind_*`: 12 pruned variants, 117 of the 138 programs on
  the most common one; with every method kept, 1;
- cold `go test -short ./internal/gen ./internal/devgoldens` with the lock:
  40 variants compiled, 154 s, peak 21.9 GB; with XO_GO_PROTO=all, 22
  variants, 124 s, peak 32.7 GB (the bigger rt instantiated in every
  program's package costs memory) and TestGoProtoNotKept fails, as it
  should;
- without the lock the same run was stopped at heavy.sh's 32 GB limit after
  132 s.

## Decision (proposed)

Option A: write the `use go` adapters into their own package
(`xoprog/gobind`, importing rt) instead of the rt directory. rt then
depends only on the program's protocol needs: 12 variants for the goldens
instead of 30, and every program without `use go` and with the common
needs shares one compiled rt. 0551's savings stay.

## Consequences

- Fewer cold rt compiles after a std/rt change (30 to 12 on the goldens;
  the lock compiles each once).
- The adapters use no unexported rt name and declare no method on an rt
  type (checked 2026-11-16 on the Go trees of the six `use go` goldens
  go_uuid, go_strconv, go_jwt_mapclaims, test_go_callback_task, go_outbuf
  and go_shared_type: 40 adapter files, a go/ast scan of the names they
  use against rt's other files). They declare their own handle types
  (`Go_<path>_<Name>`) with exported methods (XoDebug, XoEq, XoGoValue,
  XoFromGo), so they compile in a package that imports rt and qualifies
  its names.
- The program's package names adapter types as `rt.Go_<path>_<Name>`
  (`rt.Go_github_com_google_uuid_UUID`); compiler/gen writes `gobind.`
  for a binding's names instead (core names such as `Go_error`, `Go_any`
  and `Go_Error` stay in rt). compiler/gen/gen.xo writes the files at
  `gobind/xo_gobind_*.go`, and gobackend.xo's timer check
  (`rt/xo_gobind_` with NewGoCallback) follows the path.
- An adapter file binding a package many programs use (bytes,
  database/sql/driver) is compiled once per set of bindings, as today; the
  change only stops it from making rt a variant.

## Alternatives

- B: A plus every protocol method kept (XO_GO_PROTO=all as the default): 1
  variant, but the bigger rt is instantiated in every program's package
  (peak 21.9 to 32.7 GB on -short, and 0551's 14.7 to 10.8 GB on
  armnetwork lost). Rejected on memory.
- C: A plus pruning only above a program size: small programs share one
  full rt, large ones keep 0551's gain; two code paths and a threshold to
  tune.
- D: a shared core rt (no generic protocol methods) plus the pruned
  methods in a per-program package: Go requires a type's methods in its
  own package, so the generic types would move too; large change.
- E: the lock alone (decision (1)): each variant compiles once per cache,
  and -short stays at 154 s cold; no structural change.

## Implementation (Xo Lang 2, 2026-11-16)

compiler/gen writes the adapters to `gobind/xo_gobind_*.go` (package
gobind, importing rt), qualifies a binding's names with `gobind.`, and
rewrites the `%rt.` placeholders of a binding's call sites to gobind;
std/rt counts gobind frames as the runtime's in its deadlock stacks. Test:
internal/gen TestGoBindOwnPackage (failed before: the adapters were in
rt/), std/rt TestGoParked "parked in an adapter".

Measured on host 4 (linux/arm64, exclusive, a cold Go cache each side:
xobuild, then `go test -short ./internal/gen ./internal/devgoldens`,
host memory sampled), main a64db4573 against the change 76c7ee48c, in
both orders:

| run | main wall | change wall | main peak | change peak |
|---|---|---|---|---|
| main first | 315 s | 273 s | 14.1 GB | 17.9 GB |
| change first | 439 s | 260 s | 39.8 GB | 14.7 GB |

The change is faster in both orders (13% and 41%). The peak of host
memory varies with how many cold builds overlap (14 to 40 GB for the
same tree), so the two runs neither show a memory cost nor a memory
saving; the rt lock (gotchas/go-backend-cold-rt-compiled-per-build.md)
still bounds the compiles of one variant.
