# `xo test --race` of a program with C sources failed where a race runtime is installed

Status: Observed (2026-11-20, tier 918 on the linux/arm64 host, task #558)

Symptom: internal/llvm TestRaceUseC failed on the linux/arm64 host with
"xo: --ld=macho links no C sources (.../cffi/xo_c_racec_.c): link with
cc", and passed on the Mac. With `XO_RACE_RUNTIME` naming clang's
runtime the Mac failed too: "--race on the arm64 backend links with Xo's
linker (not --ld=cc, decision 0273): darwin and linux".

Cause: `--race` with no `--backend` builds natively where a
ThreadSanitizer runtime is found (race_backend: lib/xo/race, in every
release and on that host) and with LLVM elsewhere. A checkout has no
runtime, so the test ran LLVM's path there and never the native one. The
native `--race` link is Xo's linker's, which takes objects, and the
program's `use c` adapter and link files are C sources.

Fix: a program with such sources builds with LLVM (decision 0273's
addendum of 2026-11-20; build_native returns Native.race_llvm after the
check and the caller builds again). TestRaceUseCWithRuntime sets
XO_RACE_RUNTIME, so both paths are tested in a checkout.

Avoid: a test of a default that depends on what is installed (a runtime,
a prebuilt object, clang) runs once with it and once without. The ELF
linker's message says `--ld=macho` (ld_objects serves both linkers).
