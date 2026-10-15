# 0174: What xo1 must do to replace the Go driver for native builds

Status: Accepted (2026-10-12), steps 1 to 6 implemented in `selfhost/xo`
(tools.xo, tls.xo, link.xo, tz.xo, rt.xo, cffi.xo, test.xo, main.xo),
`scripts/selfhost-samebin.py`, and `scripts/selfhost-goldens.py` (now
passing each golden's args.txt, env.txt, stdin.txt). Evidence:
`bench/results/selfhost-arm64-2026-10-11.md`, section "xo1 takes over
internal/build's native steps". The released `xo` is unchanged
(decision 0150).

## Context

The switch-over experiment of decision 0158 reached the fixed point on
the assembly: xo1 (`selfhost/xo`, the lexer through the arm64 emitter in
Xo, then `cc`) compiles itself byte identically and passed 132 of the
138 arm64 goldens. What internal/build adds beyond the C runtime was not
ported: the TLS library of std/http (the 6 failures), the zone data of
std/time, the prebuilt runtime packs and the runtime object cache
(decision 0157), `use c` sources (decision 0152), cross targets, and
test binaries. Replacing the Go driver for native builds needs all of
them, and needs them to agree with the Go driver, or the two drivers
would fill the user's caches twice and link different binaries from
the same program.

## Decision

1. **Same keys, shared caches.** Every cache key xo1 computes is
   internal/build's, byte for byte: the TLS library's (tlsLibrary: the
   embedded sources' hash, read from the repository's third_party
   files; the C compiler's path and `--version`; SDK; target; sanitizer
   flavor; FileDefines printed as Go's %v), the runtime object's
   (binKey "rtobj": toolID with the compiler's size and nanosecond
   modification time, flags, SDK, TLS flags and hash, the runtime
   files), and the pack key (RTPackKey, with Go's %q of the flag
   lists). The vendored library's configuration (mbedtls.Defines,
   IncludeDirs, SourceDirs, Exclude, FileDefines) is duplicated in
   tls.xo; `xo1 keys` against `xo rtpack --key` and the cache
   directories shows a drift.
2. **The steps, in internal/build's order.** TLS (step 1), zone data
   (step 2: xo_tzdata.c from Go's zoneinfo.zip, the zone use read from
   the module xo1 built, constants confirmed as literals in the
   source), packs and the object cache (step 3), `use c` adapters from
   cbind's cache and link inputs from the binding's key (step 4), the
   linux/arm64 target as assembly only, as internal/build does (step
   5), and `xo1 test` with the harness of `xo test --no-gc` (step 6).
3. **The measure is the binary.** `scripts/selfhost-samebin.py` builds
   every arm64 golden with both drivers under the same file name; 147
   of 147 are byte identical (2026-10-12).
4. **Workarounds for native gaps, not new std surface.** Env.program,
   Fs.mode, Fs.glob, and Proc.cmd are XO1201 on native builds, so xo1
   reads `$_`, calls stat(1), lists directories, and runs children
   through `sh -c 'cd DIR && exec env ...'` (gotchas/
   xo-check-passes-native-build-refuses.md).

## Consequences

- With steps 1 to 3 the self compiled compiler passes 147 of 147 arm64
  goldens and links the Go driver's binaries exactly. The fixed point
  holds with every step (2026-10-12): xo1 built by the Go compiler (A)
  builds B, B builds C, and the Go emitter's, B's, and C's assembly of
  selfhost/xo are byte identical (9,011,385 lines); B passes 147 of 147
  arm64 goldens.
- Not done, in the order they matter: tests with `uses` (`--real`): the
  Xo SSA builder leaves them out; the contract fuzz tests the Go driver
  lists as skipped; a time limit on a test process; the binary cache
  (XO_BIN_CACHE, off by default; on by default for `xo test` since decision 0236); linking linux/arm64 from a Mac (not
  in internal/build either); the zone use of `let n = "X"; zone(n)`
  next to a literal `zone("X")` of the same name is taken as constant
  (the Go builder embeds the whole database there).
- A change to internal/build's keys or to mbedtls.go's configuration
  must be ported to selfhost/xo in the same change; `xo1 keys` and
  selfhost-samebin.py catch a miss.

## Alternatives

- **A new std API for the missing pieces** (gzip, zip, process
  environment, executable path): cleaner, but std surface is a separate
  decision each; the workarounds use only what native builds have.
- **Own cache directories for xo1**: simpler keys, but two drivers
  building the same runtime and TLS library twice, and no way to show
  the drivers agree.
- **Port the Go driver's tlsArgs by reading the tgz with an Xo gzip
  reader**: tar(1) on the same archive gives the same tree in a line.
