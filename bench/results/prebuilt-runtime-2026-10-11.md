# Prebuilt runtime objects: first native build time (2026-10-11)

Decision 0157. The first `xo build --backend=arm64` of a program with
every cache cold, with the C runtime (std/llrt) compiled locally
(`XO_RTPACK=off`) and with the prebuilt pack of a release archive.

## Setup

- Host: darwin/arm64, Apple clang 21.0.0 (clang-2100.1.1.101), Go
  1.26.1, `GOMAXPROCS=4`. Another agent's test runs may have shared the
  machine.
- Linux: the `xo-linux-test:arm64` image (Debian trixie, clang 19.1.7,
  glibc 2.41) in Docker Desktop on the same Mac (linuxkit 6.10.14).
- xo: `scripts/release.sh v0.0.0-test` from this branch, the darwin
  archive extracted (the pack found beside the binary, no `XO_RTPACK`);
  for Linux, `GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build` plus the
  pack cross compiled with zig cc 0.15.1 (`-target
  aarch64-linux-gnu.2.31`).
- Every run: new `XO_BUILD_DIR`, `XO_RTOBJ_CACHE`, `XO_TLS_CACHE`, and
  `XO_BIN_CACHE=off`; wall time of the whole `xo build` (check, SSA,
  assembly, runtime, link).
- Programs: `internal/arm64/testdata/run/boxes` (plain runtime, variant
  `rt.o`), `internal/gen/testdata/golden/time_zones` (XO_TIME and two
  zones, a `rt-time*.o` variant plus `xo_tzdata.c`),
  `bench/perf/http/xo` (an HTTP server: a TLS variant plus
  `libxotls.a`; locally the TLS library is compiled too, about 2.5 s).
- Script: `bench/perf/rtpack/first-build.sh <xo> 3` (the Linux runs
  used the same loop inside the container, with `date +%s.%N`).

## Results (seconds, median of three; runs in brackets)

| Program | darwin local compile | darwin pack | linux local compile | linux pack |
|---|---|---|---|---|
| boxes | 1.94 (1.94 1.92 1.97) | 0.13 (0.13 0.13 0.12) | 3.21 (3.37 3.21 2.92) | 0.07 (0.08 0.07 0.07) |
| time_zones | 1.91 (2.06 1.91 1.84) | 0.19 (0.19 0.19 0.18) | 3.09 (3.13 3.09 3.05) | 0.11 (0.13 0.11 0.11) |
| http server | 4.27 (4.27 4.12 4.43) | 0.28 (0.28 0.28 0.28) | 6.21 (6.14 6.21 6.24) | 0.17 (0.18 0.17 0.16) |

For comparison, the second local build (the runtime object cache warm,
build directory cold) on darwin: boxes 0.12 s, time_zones 0.21 s, http
0.31 s. The pack gives the first build the time every later build had.

Outputs: the boxes and time_zones binaries print the same bytes with
and without the pack on Linux; the HTTP server built against the pack
starts and listens.

## Other checks

- `go test ./internal/arm64 -run 'TestGoldenRun|TestRun'` with
  `XO_RTPACK` set to the pack: passes on darwin/arm64 (58 s) and in the
  linux/arm64 container with the zig built pack (35 s), and the local
  object cache stays empty (no runtime compiled).
- `go test ./internal/llvm -run
  'TestTimeDiff|TestZoneDataPerUseNative|TestTLSDiff|TestH2Diff|TestStdin|TestGenNative'`
  with the darwin pack: passes (30 s).
- Debian bullseye (`gcc:10` image: gcc 10.5, glibc 2.31, no clang),
  `XO_CC=gcc`: boxes, time_zones, and the HTTP server build and run with
  the zig built pack. With `XO_RTPACK=off` gcc compiles the runtime
  too, so on Linux the arm64 backend never needed clang.
- Determinism: two packs from different work directories are byte
  identical on darwin (Apple clang) and for linux (zig); two runs of
  `scripts/release.sh` print the same SHA256SUMS for all three
  archives.

## Sizes

| | darwin/arm64 | linux/arm64 |
|---|---|---|
| pack (32 objects + libxotls.a) | 16 MB | 17 MB |
| one object | 0.43 to 0.52 MB | 0.44 to 0.55 MB |
| release archive (.tar.gz) | 20 MB | 19 MB |

linux/amd64 (no pack, the LLVM backend compiles its runtime): 14 MB.
Building a pack: 18 s (darwin, Apple clang) and 24 to 25 s (linux, zig,
cold zig cache), GOMAXPROCS 4.

## Cross compilation for linux/arm64 on the Mac

| Compiler | Works | Needs | Notes |
|---|---|---|---|
| zig cc 0.15.1 `-target aarch64-linux-gnu.2.31` | yes, chosen | the zig tarball (48 MB, SHA-256 checked against ziglang.org's index.json) | its own glibc headers for a chosen version; `-g0` needed (zig adds debug info by default: 2.7 MB objects); `zig cc ... --version` exits 1 with the glibc suffix but prints the version first |
| Apple clang `--target=aarch64-linux-gnu --sysroot=DIR` | yes | Linux headers (`usr/include` of a Linux image, `tar -h` for the `asm/` symlinks; 43 MB) | 1.7 s per variant; no outline atomics unless asked (Debian's clang turns them on); output changes with Xcode |
