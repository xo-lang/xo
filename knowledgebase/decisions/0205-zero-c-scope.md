# 0205: What "no compiled C" covers

Status: Accepted (owner, 2026-10-13, the lead's recommendation).
Clarifies decision 0195.

## Context

The zero-C audit (bench/results/zero-c-audit-2026-10-13.md, "Owner
decisions found", D) noted that programs with `use c` bindings compile
small C adapters (cffi, xo_<prefix>.c) and link C libraries, so they
need a C toolchain whatever the runtime is written in. Decision 0195's
goal had to say whether it means no C toolchain for the runtime, or for
every Xo program.

## Decision

The goal is no C toolchain for the runtime and for every program that
does not use `use c`. A program that uses `use c` calls C by choice and
needs a C compiler for its adapters and libraries, as Go's cgo does;
that stays supported and is not part of decision 0195's goal.

## Consequences

- Build and release work aims at C-free builds of programs without
  `use c`; `use c` keeps its toolchain requirement and its `ffi`
  effect (spec 4.5, 6.1).
- On Linux, a `use c` program links libc for its C libraries even
  though the runtime itself does not (decision 0203).

## Alternatives

- No C toolchain for any Xo program: would require removing or
  reimplementing `use c`'s adapters in Xo; not chosen.
