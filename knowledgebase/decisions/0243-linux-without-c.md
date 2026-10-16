# 0243: Linux without C (static, no libc, Xo's own linker)

Status: Accepted (owner approved the work, 2026-10-16, to start after
seed 0.18.0; lead's plan); steps 1 and 2 implemented 2026-10-16 (xo-fa,
elflink reviewed by Xo Lang 3). The toolchain's own Linux self build
keeps the C library until a Linux self rebuild fixed point has passed
with none (below).

## Context

Decisions 0203 and 0206 made a Linux program with no C library possible
(`XO_NOLIBC=1`: raw system calls, the runtime's C library functions in
Xo, sockets, epoll and the DNS resolver in Xo), but opt in, and linked
only by `cc -static -nostdlib`: selfhost/macho's ELF linker (decision
0182 stage 3) made dynamic glibc executables only, and its assembler
failed on any task program (gotchas/elf-assembler-no-got-lo12.md). Go
links static binaries with no libc on Linux by default; Xo's spec already
promises static binaries (core.md 9).

## Decision

1. A Linux program with no `use c` module links no C library by default,
   on both native backends (arm64, and LLVM through clang).
   `XO_NOLIBC=0` links the C library (dynamic, glibc); `XO_NOLIBC=1` asks
   for none and is an error for `use c` programs, as before
   (selfhost/xo link.xo nolibc).
2. selfhost/macho elflink gets a static mode (`ElfLinkOpts.static`,
   chosen by the driver for a no-libc link with `--ld=macho`): the
   program's own `_start` (the backends' entry stub); no loader, no
   imports, no .interp, .dynamic, dynamic symbols, versions, PLT stubs or
   dynamic relocations, no PT_INTERP or PT_DYNAMIC; PT_PHDR, PT_TLS and
   PT_GNU_EH_FRAME stay (the runtime reads them). Weak undefined symbols
   (the sanitizer hooks) are 0, GOT slots included; a call of one is a
   nop, as GNU ld does on arm64, and a tail call a ret (what calling an
   empty function does; sanhook.xo guards every call anyway); any other
   undefined symbol is an error. A plain
   ET_EXEC at 0x400000, not PIE, as Go's static binaries. The outline
   atomics helpers are still synthesized.
3. The ELF assembler: `:got_lo12:` (the prefix was compared as 11 bytes,
   so it never matched: the whole of the GOT gotcha), `msr tpidr_el0`
   (the static `_start` sets the thread pointer), `.size SYM, .-START`
   in code, and a 4 byte pointer in a section that is not loaded as
   R_AARCH64_ABS32 (decision 0239's .debug_info offset of its
   abbreviations; a loaded one stays an error).
4. The toolchain's own native Linux builds of selfhost/xo pass
   `XO_NOLIBC=0` (internal/xotool hostBackend for xobuild, the seed
   chain and BuildXo; scripts/bootstrap-xo.sh; scripts/xo1-host-check.sh)
   until a Linux self rebuild fixed point has passed with no C library
   (pending: CI is off by the owner's order, and a self build does not
   fit the 8 GB Docker VM, gotchas/docker-vm-oom-self-build.md). The
   seeds' Linux xo is cross built on the Mac with `--backend=go`
   (scripts/seed-cut.sh), which no-libc does not touch.

linux/arm64 is the gate; linux/amd64 follows best effort (the LLVM
backend's no-libc path; elflink is arm64 only).

## Evidence (2026-10-16, Docker linux/arm64, scripts/linux-goldens.sh)

- arm64 backend, cc link: 180 goldens with the C library, 180 with none
  (all 180 statically linked);
- LLVM backend (clang), no C library: 180 of 180 pass, all static;
- arm64 backend, no C library, `--ld=macho` (elflink static): 180 of 180
  pass, all static (before the review's fixes), and the final build's 57
  task, fault, network, http, tls, file system and process goldens pass;
- DNS without libc: http://localhost:1 (refused), https://example.com
  (200), http://no-such-host.invalid (no address) give the same results
  with and without the C library;
- llvm-readelf -lSd of a static elflink binary: no warnings; PHDR, three
  LOAD, TLS, GNU_EH_FRAME, GNU_STACK; `file`: statically linked.

## Consequences

- Linux binaries of programs without `use c` no longer need glibc (any
  distribution, any glibc version, scratch containers).
- A C library function the runtime reaches that cabi_libc.xo lacks fails
  the link, naming it; `XO_NOLIBC=0` is the way out until it is added.
- Next: flip the toolchain's self build once a Linux fixed point passes
  (CI back or Docker with more memory, the owner's setting); then the
  default linker for linux/arm64 (`--ld=macho`, step 3 of the plan),
  with `--ld=cc` kept; `use c` programs keep linking libc through cc.
- elflink still writes local symbols under -s (a strip option is Xo Lang
  3's follow-up, with decision 0246's LinkOpts.strip).

## Alternatives

- Keep no-libc opt in until the self build's fixed point passes (option
  (a)): users would wait on a toolchain-only check.
- PIE static executables: relocations at start up for no benefit to
  these programs; Go makes plain ET_EXEC.
- Flip the self build too, on the goldens and the Mac fixed point only:
  the seeds and the Linux fixed point would change untested.
