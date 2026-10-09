# 0089: Versioning before 1.0

Status: Accepted (2026-10-09, agreed with the user)

## Context

Xo had drafts 0.1 to 0.7 of the spec and frozen toolchain binaries
(`.tools/bin/xo-0.4`, `xo-0.6`), but no git tags, no `xo version`, and no
changelog. A spec 1.0 audit showed that 1.0 (a compatibility promise) still
depends on modules (0086), `std/time`, native TLS, and four Appendix B
questions. The owner wants to keep making the language as stable as
possible and decide personally when it is 1.0.

## Decision

- Xo stays on 0.x. Only the owner declares 1.0; agents never tag, announce,
  or freeze 1.0 or the 2026 edition on their own.
- The toolchain version and the spec draft share the minor number: spec
  draft 0.N is implemented by toolchain 0.N.x. `xo version` prints it
  (constant `version` in `cmd/xo/version.go`) with the source revision.
- A minor release closes a spec draft at a milestone: the spec header
  moves to the next draft with an empty "Changes in 0.N+1" section,
  `CHANGELOG.md` gets an entry, and `main` is tagged `v0.N.0`. Patch
  releases (`v0.N.1`) are fixes with no spec change.
- A breaking change between minors ships an `xo fix --from=draft-0.N`
  rewriter in the same release, as since draft 0.1.
- The stability work continues as if 1.0 were next: the spec audit sorts
  every feature into stable, preview, or gap, which is the input for the
  owner's 1.0 decision.

## Owner's 1.0 criteria (2026-10-09)

Native backends are not a preview at 1.0, but parity is scoped
("A-scoped"): native builds must cover everything a typical HTTP service
uses; anything else is documented as unsupported on native (a precise
`XO1201`) rather than blocking 1.0.

Required natively for 1.0 (from the native gaps in
`bench/results/spec-audit-2026-10-09.md`):
- Every core language construct: interface types (dynamic dispatch), list
  patterns, newtypes, `Float32`, `Secret[T]` in every output path, `Buf`
  and `StrBuilder`, calls with defaulted arguments, all `?` error
  conversions, `get_or_fault` for every key type.
- HTTP: serving and calling HTTPS (native TLS) in addition to plain HTTP,
  which stays first class for TLS terminated at a proxy or load balancer.
- Files: every `Fs` method and attenuation (`sub`, `read_only`,
  `write_only`), and `std/path`.
- `std/time` (decision 0094), `os.tasks`, `Once`, `Semaphore`,
  `map_concurrent`, standard input with cancellation (decision 0088).
- `xo test --no-gc` with the test fakes (`MemFs`, `set_input`,
  `FakeProc.run`).
- Targets: darwin and linux on arm64 and x86-64 (x86-64 through the LLVM
  backend; a dedicated x86-64 native backend is not required).

Documented as unsupported on native at 1.0: `use go` (impossible by
design; `use c` is the native FFI, decision 0093), `gc` lines, and the
deterministic scheduler and record and replay where the Go backend still
leads (preview on native).

## Consequences

- Agents and users can name exactly which toolchain built a program.
- 0.x minors may still break code, but never without a rewriter.
- Tags exist from v0.7.0 on; drafts 0.1 to 0.6 are recorded only in the
  spec's "Changes in" sections.

## Alternatives

- Declare 1.0 once the audit's gap list is empty: rejected, the owner
  decides.
- Calendar versions: the edition (2026) is already the calendar axis;
  semver on the toolchain keeps 0086's checked semver uniform.
