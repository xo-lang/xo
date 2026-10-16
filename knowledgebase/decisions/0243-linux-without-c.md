# 0243: Linux without C (static, no libc, Xo's own linker)

Status: Proposed (stub, claimed 2026-10-16 by xo-fa; owner approved the
work to start after seed 0.18.0)

## Context

To be filled in. Following Go: programs without `use c` on Linux are
static executables with no libc, linked by Xo's own ELF linker.

## Decision

To be filled in. Planned steps: (1) XO_NOLIBC static becomes the Linux
default for programs without `use c`, with the goldens run both ways
and the gaps filled (DNS, user lookups); (2) elflink gets a static mode
(no PT_INTERP, no dynamic imports, TLS local exec, plain ET_EXEC, not
PIE) and the ELF assembler gets :got_lo12:
(gotchas/elf-assembler-no-got-lo12.md); (3) the linux/arm64 defaults
become elflink with no libc, with --ld=cc and libc kept as options and
`use c` programs linking libc through cc. linux/arm64 is the gate;
amd64 best effort.

## Consequences

To be filled in.

## Alternatives

To be filled in.
