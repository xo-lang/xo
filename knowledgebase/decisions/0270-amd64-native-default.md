# 0270: Native backend as the default on amd64 hosts

Status: Accepted (owner, 2026-10-19): native is the default backend on
every host now, amd64 hosts included (darwin/amd64, linux/amd64,
windows/amd64), and the speed bar below (about 10% of LLVM's
instructions) no longer gates it. The gap is still open when this is
accepted: HTTP about 1.15 times LLVM's instructions; on the Intel Mac
JSON 1.24, RSA 1.44, portable 1.53 times the instructions and 2.52 times
the wall time. Xo lang 4 keeps closing it (the gaps ranked below).
Proposed by xo-fa the same day, with the owner-approved checks.

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

## Decision

The default backend is the native one: arm64 or amd64 by the
architecture of the build's targets (`--targets`; else the host's), on
darwin, linux and windows (compiler/xo main.xo native_backend). This
supersedes decision 0248's x86_64 line. `--race` still chooses LLVM
(ThreadSanitizer, which the native backends lack; superseded by
decision 0273's 2026-10-22 addendum: the native backend wherever a
prebuilt runtime ships), the Go-only flags
(--hybrid, --holes, xo test --explore, xo run --seed and --record)
still choose Go, and `--backend=llvm` keeps LLVM's code for anyone who
wants it.

As proposed, the switch waited for the gaps to close (HTTP plain and
newconn, the TLS handshake, and JSON within about 10% of LLVM's
instructions); the owner made it on 2026-10-19 with them open, and the
work on them continues.

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

- amd64 builds stop needing clang and an LLVM toolchain by default, as
  arm64 builds already do (decision 0248), and build faster (no clang in
  the loop).
- amd64 programs run the native backend's code, slower than LLVM's by
  the gap above until it closes; `--backend=llvm` is one flag away.
- A cross build picks its backend by the target: `--targets=linux/amd64`
  from an arm64 Mac is the amd64 backend's, no longer an error.
- std's module tests (internal/stdtest) run on the host's native
  backend, amd64 on x86-64 hosts.

## Alternatives

- Switch now: simpler toolchain story, but HTTP servers and TLS
  handshakes would take about 25% more instructions, against the
  owner's bar.
- Native by default with `--release` choosing LLVM: two code paths for
  one program, and the measured paths (servers) are release builds.
- Keep LLVM on amd64 for good: leaves amd64 hosts needing clang while
  arm64 hosts do not.

## Addendum: --targets=all with the native default (owner, 2026-10-22)

0.25.2's `xo build --targets=all` failed with no `--backend`: one native
backend was chosen for every target, and the arm64 backend rejected
linux/amd64. Now each target builds with the native backend of its
architecture (compiler/xo main.xo backend_groups, build_by_backend:
arm64, amd64, and wasm for wasip1/wasm), and with no `--backend`, `all`
means the six targets a native backend builds: linux/amd64, linux/arm64,
darwin/amd64, darwin/arm64, windows/amd64 and wasip1/wasm. windows/arm64
has no native backend; a note on stderr says it needs `--backend=go`,
and the build exits 0. `--backend=go --targets=all` keeps all seven. The
owner chose this over building the rest and failing on windows/arm64,
which would have made the README's `xo build --targets=all` always exit
1. Naming windows/arm64 explicitly with no `--backend` is still an error
(after the other targets are built). Test: cmd/xo TestTargetsAllNative.
