# 0270: Native backend as the default on amd64 hosts

Status: Proposed (xo-fa, 2026-10-19; owner-approved checks). The
correctness check passes and the performance check does not, so the
default stays LLVM on amd64 hosts until the gaps below close; Xo lang 4
works on them (lead, 2026-10-19). When they do, the owner decides, and
this record is accepted as written or amended with the new figures.

## Context

Decision 0248 made the native backend the default where there is one:
arm64 on arm64 hosts, LLVM on x86_64 hosts, since the native amd64
backend (decision 0254) did not exist then. It now builds every golden
for linux/amd64 and darwin/amd64 with Xo's own assembler and linker.
The owner asked (2026-10-18, through the lead) whether the default
should become native on darwin/amd64 and linux/amd64 hosts too, on two
checks: (1) every golden passes with the native amd64 backend on both,
or only documented gaps the default path never hits; (2) no large
compute regression against LLVM at release settings, native within
about 10% of LLVM or better on the HTTP, JSON, and TLS workloads.

## Findings (2026-10-19, main 1f4ce3cc)

bench/results/amd64-native-vs-llvm-2026-10-19.md has the commands and
every figure.

- Goldens: 199 of 199 on linux/amd64 (Docker, emulated, no C
  toolchain) and 199 of 199 on darwin/amd64 (an Intel Mac). Check (1)
  passes.
- HTTP server, instructions a request (linux/amd64, callgrind), native
  over LLVM: plain 1.242, plain-newconn 1.246, tls 1.023, tls-newconn
  (the handshake) 1.241, h2 1.033.
- Compute (Intel Mac, instructions retired, geometric means): json
  1.237, rsa 1.439, text 1.223, portable 1.531 (2.52 in time), atomic
  1.179, ctx 1.049, digest 1.027.

Check (2) fails on HTTP plain, the TLS handshake, and JSON.

## Decision (proposed)

When the gaps close (HTTP plain and newconn, the TLS handshake, and
JSON within about 10% of LLVM's instructions, with the goldens still
passing), the default backend becomes the native one whenever the host
has a native backend: arm64 on arm64 hosts and amd64 on darwin/amd64
and linux/amd64 hosts (compiler/xo main.xo, today `host_arch() ==
"arm64"`). This supersedes decision 0248's x86_64 line. `--race`
still chooses LLVM (ThreadSanitizer), the Go-only flags still choose
Go, and Windows hosts stay on LLVM until the native windows/amd64
target exists.

Until then nothing changes: LLVM is the default on amd64 hosts.

## Gaps, ranked (to close first)

1. Field arithmetic code in x25519 and nistec (fe64_mul, fe64_sq, the
   P-256 mul): most of the handshake's +730k instructions a connection,
   and RSA's 1.44.
2. Small hot functions called where LLVM inlines them: fe64_add/sub,
   pt_add256, the fast half of obj.rl_free_n, the http and http1w
   helpers.
3. Runtime calls through the cabi bridges (obj_rl_copy_n,
   obj_rs_append_bytes) where LLVM calls the helper directly.
4. String building in JSON encoding and text formatting (enc_small
   1.72, debug 1.87).
5. Integer loops whose time ratio is far above their instruction ratio
   (portable conv 2.10 instructions and 7.08 time): stalls.

## Consequences

- Accepted later: amd64 builds stop needing clang and an LLVM
  toolchain by default, as arm64 builds already do (decision 0248), and
  build faster (no clang in the loop).
- Until then: amd64 users keep LLVM's code; the native backend is one
  flag away (`--backend=amd64`) and is tested by the goldens on both
  hosts.

## Alternatives

- Switch now: simpler toolchain story, but HTTP servers and TLS
  handshakes would take about 25% more instructions, against the
  owner's bar.
- Native by default with `--release` choosing LLVM: two code paths for
  one program, and the measured paths (servers) are release builds.
- Keep LLVM on amd64 for good: leaves amd64 hosts needing clang while
  arm64 hosts do not.
