# 0157: Prebuilt runtime objects in releases

Status: Accepted (2026-10-11, lead, on the owner's task), implemented:
`internal/build/rtpack.go` (packs, keys, lookup, `BuildRTPack`,
`normalizeAr`), `internal/build/llvm.go` (`ccBuild` links a pack's
object; `ClangError.Runtime` and its hint), `internal/build/tzdata.go`
(`tzSource`: zone data in `xo_tzdata.c`), `std/llrt/xo_time.c`,
`cmd/xo/rtpack.go` (hidden `xo rtpack`), `scripts/release.sh`,
`scripts/disttar/`. Tests: `internal/build/rtpack_test.go`,
`cmd/xo/rtpack_test.go`, `scripts/disttar/main_test.go`. Evidence:
`bench/results/prebuilt-runtime-2026-10-11.md`.

## Context

The native backends link the C runtime std/llrt (14 files, about 24k
lines, one translation unit: xo_rt.c includes the rest). A native build
compiled it with the host's C compiler on the first build and cached
the object in `<user cache>/xo/rtobj` (rtobj.go); a program using
std/http also compiled the vendored Mbed TLS into a static library
(tls.go). Decision 0150 said a self hosted arm64 compiler would need
"no Go toolchain and no clang"; that was wrong twice over:

- The arm64 backend never assembled or linked itself: decision 0049 has
  `cc` assemble the generated `.s` and run the system linker.
- The runtime was compiled locally on the first build of every
  combination of the defines a program selects (XO_FS, XO_STDIN,
  XO_PROC, XO_TIME, TLS), and XO_TIME builds compiled the program's
  zone data into the runtime (`#include "xo_tzdata.h"`), so every
  distinct set of zones was another 1.8 s runtime compile.

Measured on 2026-10-11 (darwin/arm64, Apple clang 21, cold caches): a
first arm64 build of a small program takes 1.9 s, of which 1.75 s is
the runtime compile; an HTTP server 4.3 s (runtime plus the TLS
library). On Linux arm64 (container) 3.2 s and 6.2 s. Release archives
did not exist: users build xo from a checkout.

On Linux, gcc compiles the runtime too (checked with gcc 10 and 14), so
the arm64 backend never needed clang there, only some `cc`.

## Decision

1. **The zone data leaves the runtime.** `xo_time.c` declares
   `xo_tz_mode`, `xo_tz_blobs`, `xo_tz_zip`, and `xo_tz_zip_len` extern;
   the build writes `xo_tzdata.c` (`build.TZDataSource`) and compiles it
   as a source of the link. The runtime object no longer depends on the
   program, which makes XO_TIME a prebuildable variant and lets the
   local object cache reuse one object across zone sets.
2. **A pack per target.** `xo rtpack` compiles every variant of the
   runtime (32: the subsets of time, fs, stdin, proc, tls; `rt.o` to
   `rt-time-fs-stdin-proc-tls.o`) and the TLS library (`libxotls.a`)
   into `<root>/<os>-<arch>/` with `rtpack.json` (format, target, key,
   compiler, flags, SHA-256 of every file). Packs exist for the arm64
   backend's targets, darwin/arm64 and linux/arm64. **Objects, not one
   archive**: every variant is a whole runtime defining the same
   symbols, so one archive of them could not be linked as is (the
   linker would pull members by symbol and mix variants); the toolchain
   would have to extract a member first. A file per variant is linked
   by path and its checksum checked alone. The TLS library stays an
   archive, since its members are pulled selectively.
3. **The key.** `RTPackKey(target)` hashes the pack format, the target,
   the pack's compile flags (`rtPackFlags`: `-O2 -g0 -fexceptions
   -funwind-tables`, plus `-mmacosx-version-min=11.0` on darwin, `-fPIC
   -moutline-atomics` on Linux), the variant table, the runtime sources
   the xo binary embeds (`std.LLRuntime`), and the TLS library's
   sources and configuration. It leaves the compiler out on purpose, so
   one pack serves every xo built from the same sources. A pack is used
   only when its key equals the running xo's and the object's SHA-256
   equals the manifest's; an edited runtime changes the key, so a
   developer's xo never links a stale object.
4. **Lookup and fallback.** The pack root is `$XO_RTPACK`, else
   `lib/xo/rtpack` beside the binary's directory (the release layout
   `bin/xo`, `lib/xo/rtpack/<os>-<arch>/`). `ccBuild` uses a pack's
   object when the link's flags are exactly `-O2` and the variant's
   defines (the arm64 backend's links); every other link (the LLVM
   backend's `-flto`, sanitizers, `use c` include flags, an edited
   runtime, no pack) compiles the runtime locally into rtobj as before.
   `XO_RTPACK=off` forces the local compile; `XO_RTOBJ_CACHE=off` also
   skips the local cache. `xo rtpack --status` says which pack is used
   or why none is. The binary cache key includes the pack in use.
5. **Diagnostics.** A failed local compile of the runtime reports "the C
   compiler failed on the C runtime (std/llrt)" with a hint naming why
   no pack matched, what to install (macOS: `xcode-select --install`;
   Debian or Ubuntu: clang or gcc), and `XO_RTPACK`. No `cc` at all is
   `ErrNoCC`, which now names the same installs: a pack does not remove
   the need for a compiler driver to assemble and link.
6. **Releases.** `scripts/release.sh` (run on darwin/arm64) builds xo
   for darwin/arm64, linux/arm64, and linux/amd64 (`CGO_ENABLED=0
   -trimpath`), a pack for each arm64 target, and
   `dist/xo-<version>-<os>-<arch>.tar.gz` with `scripts/disttar`
   (sorted entries, zero times and owners, no gzip name or time) and
   `SHA256SUMS`. It never tags, pushes, or uploads.
7. **Compilers.** darwin/arm64 packs are built by the host's Apple clang
   (the platform compiler, the same defaults as the local fallback:
   stack protector, `___chkstk_darwin` probes). linux/arm64 packs are
   cross compiled on the Mac with **zig cc 0.15.1** (`-target
   aarch64-linux-gnu.2.31`, `zig ar`): zig ships glibc headers per
   version, so no sysroot or Docker is needed and the objects link on
   glibc 2.31 and later (checked: Debian bullseye's gcc 10 links and
   runs them with no clang installed). The manifest records the
   compiler.
8. **Determinism.** Sources are written to a work directory mapped to
   `/xo` (`-ffile-prefix-map`), `-g0`, archive members in name order,
   and `normalizeAr` rewrites every member header to time 0, owner 0,
   group 0, mode 644 (Apple's ar has no deterministic mode; with
   `ZERO_AR_DATE=1` it still records the user's uid and gid). Two packs
   built from different work directories are byte identical
   (`TestRTPackDeterministic`), as are two runs of `scripts/release.sh`
   (the same SHA256SUMS). Across machines the objects are the same for
   the same compiler: zig is pinned by version; the darwin pack depends
   on the Xcode clang and SDK of the release machine (recorded).

## Consequences

- A release archive's first arm64 build of a small program takes 0.13 s
  instead of 1.94 s on darwin/arm64, an HTTP server 0.28 s instead of
  4.27 s; on Linux arm64 0.07 s instead of 3.21 s and 0.17 s instead of
  6.21 s (medians of three; bench/results/prebuilt-runtime-2026-10-11.md).
- A native arm64 build still needs `cc` to assemble and link (on macOS
  the Command Line Tools, which bring clang anyway). On Linux the pack
  removes the runtime compile, and gcc alone suffices, as it did
  before. A compiler with no external tools needs the arm64 backend to
  write Mach-O and ELF objects and link them (see next steps).
- A native build that uses time zones by name reads Go's zoneinfo.zip
  (`go env GOROOT` or `XO_ZONEINFO`), so it still needs a Go toolchain or
  that file; packs do not carry it.
- The LLVM backend keeps compiling the runtime locally: it needs clang
  for the IR anyway, and links the runtime as LTO bitcode, which is tied
  to the clang that reads it.
- The darwin pack's objects declare macOS 11.0 (`LC_BUILD_VERSION`
  minos), the first arm64 macOS; linking them on a newer macOS is
  silent. The Linux pack uses outline atomics (`__aarch64_*` helpers
  from libgcc 10+ or compiler-rt), as Debian's clang does.
- A pack is about 16 MB per target (32 objects of 0.43 to 0.52 MB, the
  TLS library 0.7 MB); a release archive is 20 MB (darwin) or 19 MB
  (linux/arm64), 14 MB without a pack (linux/amd64).
- Building packs takes about 18 s (darwin, Apple clang) and 25 s (linux,
  zig) at GOMAXPROCS 4.

Next steps, ranked by likelihood of working:

1. Done (2026-10-12, for xo1, the released `xo`): release.sh copies
   Go's zoneinfo.zip to `lib/xo/zoneinfo.zip` in each archive, and
   selfhost/xo tz.xo reads it before `go env GOROOT` (decision 0182
   next step 2; bench/results/macho-2026-10-12.md, "Default linker and
   zone data"). The frozen Go `zoneinfoZip` is unchanged.
2. An arm64 object writer and linker in the arm64 backend (Mach-O first:
   `internal/arm64` emits `.s` today; start with `xo build
   --backend=arm64 --emit=obj` writing a relocatable object for the
   fib golden and linking it with `cc` to compare), then a static ELF
   linker for Linux; with the pack this removes `cc` from Linux builds.
3. Build the darwin pack with zig too (`--cc="zig cc -target
   aarch64-macos.11.0" -fstack-protector`; upstream clang has no
   `-fstack-check` probes for Mach-O, compare with `nm -u` against the
   Apple clang pack), which would make the whole release buildable on
   Linux.
4. Run `scripts/release.sh` in CI on tags and attach the archives (needs
   the owner's go-ahead to publish).

## Alternatives

- **Apple clang `--target=aarch64-linux-gnu --sysroot=<headers>`** for
  the Linux pack: works on this Mac (1.7 s per variant) with the
  headers of a Linux image (`docker run ... tar -chf - usr/include`; the
  `asm/` headers are symlinks, so `-h`), but needs Docker or a
  downloaded sysroot, pins glibc to whatever image the headers came
  from, and its output changes with every Xcode. Kept as the fallback:
  `xo rtpack --cc="clang --target=aarch64-linux-gnu --sysroot=DIR"`
  works unchanged.
- **Build the Linux pack in a Linux container** with Debian's clang:
  needs Docker on the release machine, and the glibc floor is the
  image's (2.41 for trixie).
- **Embed the pack in the xo binary** (`go:embed`): a single file to
  install, but every xo build from a checkout would need the generated
  objects present (or a build tag), and the binary grows by 16 MB per
  target it serves.
- **One object with every part, dead stripped at link**: the defines
  gate platform includes and static initializers, and keeping programs
  small depended on not compiling the parts in (decision 0102).
- **Key by compiler too** (as rtobj does): a pack would match only users
  with the release machine's exact compiler, which defeats the purpose.
- **Prebuilt LLVM bitcode**: tied to the clang version that reads it,
  and the LLVM backend needs clang anyway.
