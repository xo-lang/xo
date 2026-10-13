# 0216: Bootstrap seeds after the Go compiler: N-1 releases and a replay from source

Status: Proposed (owner asked for the write-up, 2026-10-13; awaiting the
owner's choice on the two questions below)

## Context

With the Go implementation of the compiler deleted (decision 0215), `xo`
is built only by an earlier `xo`, the seed (`scripts/bootstrap-xo.sh
SEED`). The seed's own runtime and compiler must be able to build the
current sources. That breaks when the runtime gains functions the new
compiler's code calls: on 2026-10-13 the deletion branch, after merging
decision 0206 stage 2 (syscalls2), could not be built by seeds older than
that merge ("undefined _xo_call_c2, _xo_sys_clone"). A rule is needed for
which seeds must work, where they come from, and whether the whole chain
can be rebuilt from source without trusting an old binary.

How other self hosted languages handle it:

- Rust: release N builds with release N-1 (a downloaded "stage0").
- Go: requires a Go release about a year older (Go 1.4's C bootstrap was
  the original root).
- Zig: commits a WebAssembly build of its compiler (zig1.wasm) and runs it
  through wasm2c and a C compiler, so a C compiler is the only seed.

## Decision (proposed)

1. **N-1 rule.** Each release's `xo` must be buildable by the previous
   release's `xo` on every platform it ships for. Released seed binaries
   are kept per platform. Between releases, main must build with the
   latest release or with a recorded intermediate seed (below).
2. **Runtime changes go in two hops.** A change that makes the compiler's
   own code call new runtime functions lands so that an older seed first
   builds an intermediate `xo` from a tree that has the new runtime, and
   that one builds the final `xo`. `scripts/bootstrap-xo.sh` gets a seed
   chain option (a list of commits or seeds) and the change records the
   intermediate commit; a gotcha documents the trap.
3. **Replay from source (proposed, owner to confirm).** The tag
   go-compiler-final (87873aa5) is the root: the Go toolchain builds that
   tree's `xo`, which builds xo1, which builds each later release in order
   to the current one. The lead replays the chain on EC2 once per release
   and records it in bench/results, so Xo can always be rebuilt from
   source and a seed binary never has to be trusted.

## Questions for the owner

- Accept the N-1 rule (1) and the two hop procedure (2)?
- Require the replay from source (3) per release, or only occasionally?

## Consequences

- Releases must publish seed binaries before the next release can be cut.
- Every runtime addition the compiler itself uses costs one extra build
  hop and a recorded intermediate commit.
- The replay chain grows with each release; its cost is one EC2 run per
  release (each hop is one self build, about 6 minutes on the Mac).

## Alternatives

- One permanent seed binary: rejected; any runtime addition eventually
  breaks it, and nothing proves it matches the sources.
- A committed wasm seed (Zig's approach, xo1.wasm run by an interpreter):
  deferred; it needs an interpreter in C or Go, against decision 0195's
  zero C goal unless in Go. Revisit with the wasm playground, which needs
  xo1.wasm anyway.
- Keeping the Go compiler as the permanent seed: rejected by decision 0215.
