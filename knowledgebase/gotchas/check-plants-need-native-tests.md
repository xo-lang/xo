# A memory-check plant cannot live in a program

Status: Observed (2026-11-18, task #349)

## Symptom

A stress program meant to prove that `--check=all` (decision 0638) fails
the stress evidence on a use after free ran to the end with
`xo leakcheck: 0 live`, on darwin/arm64 and on linux/arm64:

- `use native/llrt/obj` in a program (or under std/evidence/): XO0601,
  module not found;
- a `use c` source that writes after `free`: rc 0 under `--check=all`;
  an overrun past a `malloc` block ends in glibc's
  `free(): invalid next size` on linux, in SIGSEGV on darwin, with or
  without the check.

Also: `--race` with `--check=guard` cannot run on linux/arm64. Race
needs `--backend=llvm` there (no ThreadSanitizer runtime ships for the
native backend), and LLVM builds refuse free and guard
(compiler/xo/main.xo check_refusal).

## Cause

Safe Xo frees nothing a program can reach. `native/` modules are
importable only from std's own module tests (`xo test
--std-module=native/llrt/...`). A `use c` program's `malloc` and `free`
are the C library's, not Xo's heap, so the free check and the guard
pages never see them.

## Fix

Plant memory bugs in a native/llrt module test, as TestCheckFlagPlants
(internal/arm64/heapguard_test.go) does: heapos's "write after free"
under `--check=free`, libcmem's overrun under `--check=guard`. To test
a consumer of the check's output, run that plant and feed its output to
the consumer. scripts/stdproven TestCheckFailureNamesModuleAndTrap does
this for the stress kind's error.
