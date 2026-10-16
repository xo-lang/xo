# --ld=macho cannot link a linux/arm64 program that uses tasks

Status: Observed (2026-10-16, decision 0241); queued with "Linux without C"

## Symptom

`xo build --backend=arm64 --targets=linux/arm64 --ld=macho` of any program
with tasks (or one referencing a C symbol through rt.sym_addr) fails:

```
xo: assembler: line 14993: unsupported instruction: ldr x9, [x9, :got_lo12:__tsan_switch_to_fiber]
```

## Cause

On Linux the arm64 emitter loads the address of a weak or external
symbol through the GOT (`adrp :got:` and `ldr :got_lo12:`): sanhook.xo's
weak __tsan_* and __sanitizer_* references are in every task program.
selfhost/macho's ELF assembler and linker (elflink) have no GOT
relocations.

## Fix

None yet: GOT entries (R_AARCH64_ADR_GOT_PAGE, R_AARCH64_LD64_GOT_LO12_NC)
in selfhost/macho's ELF path, part of the queued "Linux without C" item.
Until then link linux/arm64 programs with cc, in the container
(knowledgebase/infrastructure/toolchain.md "Linux in Docker on the Mac").
