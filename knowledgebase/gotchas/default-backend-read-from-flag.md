# The build read --backend, not the backend it chose

Status: Observed (2026-10-19, while making native amd64 the default,
decision 0270; found by reading, before any build went wrong).

## Symptom

None seen yet: it would have shown on the first x86-64 host to build
with the new default. A build with no `--backend` that picked amd64
would have been lowered as no amd64 build is: cpu.xo's carry arithmetic
not lowered for the amd64 emitter (soft_cpu off), and a windows/amd64
program linked with the MSVC C runtime and lld-link instead of Xo's PE
linker.

## Cause

compiler/xo main.xo resolves the default backend into a local
`backend`, but lower() (soft_cpu, imm_rc) and link.xo nolibc (the
Windows no C runtime choice) read `o.backend`, the flag as given, which
is empty for a default build. On arm64 hosts every answer happened to
match arm64's. On x86-64 hosts a default LLVM build got imm_rc on
(`"" != "llvm"`), unlike `--backend=llvm`, so the two differed; now
only `--race` picks LLVM by default, and it gets `--backend=llvm`'s
flags. With amd64 the default, soft_cpu and the Windows link would have
been wrong.

## Fix

build_native sets `o.backend` to the backend it chose and records
`o.backend_auto` for the errors that should say the default was used
(a `use go` program, no clang). cmd/xo TestDefaultBackendAmd64 builds
cpu_prims for linux/amd64 and windows/amd64 with no `--backend` and with
`--backend=amd64` to the same path and requires the same bytes. Any new
choice that depends on the backend reads `o.backend` inside
build_native, never before it.
