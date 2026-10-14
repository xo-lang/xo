# 0216: Bootstrap seeds after the Go compiler: N-1 releases and a replay from source

Status: Item 1 Accepted (owner, 2026-10-13), with the seeds in a
private S3 bucket; item 2 Accepted as an opt in fallback (the seed
chain, implemented 2026-10-13; it never runs unasked, owner,
2026-10-13); item 3 Accepted as occasional, deferred (owner,
2026-10-14)

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

## Decision

1. **N-1 rule, prebuilt seeds (Accepted, owner, 2026-10-13).** Release N
   is built by release N-1. Each seed is a tarball per platform of the
   directory xo/ (xo/bin/xo and xo/std; xo finds std at bin/../std),
   built at a recorded commit (`scripts/seed-pack.sh`), and kept in the
   private S3 bucket `xo-seeds-ACCOUNT_ID` (us-east-1, public access
   blocked, versioned): `seeds/<seed>/<goos>-<goarch>.tar.gz` with a
   `.tar.gz.sha256` beside it, `seeds/<seed>/manifest.txt`, and
   `seeds/latest.txt` naming the newest seed, where a seed's name is
   `<version>-<commit, 8 hex>` (the first, 0.9.0-5e21dc31, uploaded by
   the lead on 2026-10-13). Seeds are not public releases. From the next
   seed on, each bumps the minor version (0.10.0-23ce8ce2, uploaded
   2026-10-14, cross built on the Mac for every platform but
   darwin-arm64; 0.11.0 next).
   internal/xotool fetches the newest seed with the aws CLI when no
   local xo is found (the development Mac's AWS user; EC2 instances
   with the instance profile `xo-ci-seed-read`, read only), checks its
   sha256, unpacks it into the user cache by version, and builds the
   checkout in one hop. Layout, variables, and the lead's pack and
   upload commands: infrastructure/toolchain.md "Prebuilt seeds".
   A failed fetch (no aws CLI, no access, no seed for the host, a
   sha256 mismatch) is an error that names the cause and the bucket
   path and says building from source is opt in; it never starts the
   chain (owner, 2026-10-13: the chain costs 25 to 37 minutes and hid
   fetch failures). Seeds unpack only into the user cache, never into
   $HOME (the tarball's top directory xo/ clashed with the EC2 checkout
   ~/xo).
2. **Runtime changes go in two hops (Accepted as an opt in fallback).** A
   change that makes the compiler's own code call new runtime functions
   lands so that an older seed first builds an intermediate `xo` from a
   tree that has the new runtime, and that one builds the final `xo`.
   scripts/seed-chain.txt records the intermediate commits;
   `scripts/bootstrap-xo.sh` (XO_SEED_CHAIN) and internal/xotool apply
   them; gotchas/seed-older-than-intrinsics.md documents the trap. With
   prebuilt seeds the chain runs only when asked for:
   `XO_SEED_FALLBACK=chain` (after the other seeds) or `go run
   ./scripts/xobuild --from-source` (the go-compiler-final root alone),
   for the replay from source (item 3) and for bootstrapping a new
   platform. It is only needed from the go-compiler-final root: once a
   seed exists past a runtime break, a new seed after each such break
   replaces adding hops, and the hops older than the newest seed are
   used only by that opt in.
3. **Replay from source (Accepted as occasional, deferred; owner,
   2026-10-14: "Occasionally but not now", after much better build
   speedups).** Not per release: on linux/arm64 only (the fastest
   path), at significant points such as before a public release or
   1.0, and not before the build speed work of decision 0220 lands. The
   oldest hops (the go-compiler-final Go compiler, then 0.9.0) keep
   their own speed forever; only hops after the speedups get cheaper.
   Measured per hop (2026-10-13/14): 411 s on linux/arm64 r8g.8xlarge
   and 311 s on the Mac (decision 0215), against 2,308 s on linux/amd64
   c7i.8xlarge and 3,541 s on the Intel Mac (both through LLVM), which is
   why the replay runs on arm64. The tag
   go-compiler-final (87873aa5) is the root: the Go toolchain builds that
   tree's `xo`, which builds xo1, which builds each later release in order
   to the current one. The lead replays the chain on EC2 once per release
   and records it in bench/results, so Xo can always be rebuilt from
   source and a seed binary never has to be trusted.

## Questions for the owner

- Answered 2026-10-13: the N-1 rule (1) with prebuilt seeds in a private
  bucket; the two hop procedure (2) as an opt in fallback, never
  automatic (owner, 2026-10-13).
- Answered 2026-10-14: the replay from source (3) is occasional, not
  per release, and deferred until builds are much faster.

## Consequences

- A seed must be packed and uploaded (all platforms, then latest.txt)
  before the next version can rely on it; until then hosts build from
  the previous seed, and a host whose platform has no seed fails unless
  given `XO_SEED` or the opt in `XO_SEED_FALLBACK=chain`.
- A fresh host with the aws CLI and bucket access needs neither Go's
  compiler build of go-compiler-final nor the hops: one download and one
  build of selfhost/xo. On a fresh EC2 machine the chain took 1,491 to
  1,936 s (decision 0215, toolchain.md "Bootstrapping without Go"); on
  the Mac with empty caches and a local seed directory it took 325 s
  (toolchain.md "Prebuilt seeds", 2026-10-13).
- A runtime addition the compiler itself uses costs a new seed (and a
  recorded hop for the fallback until that seed is uploaded).
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
