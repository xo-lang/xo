# A weak reference before a strong one made the name weak

Status: Observed 2026-11-10 (decision 0573 `use c` stage 2, Xo Lang 1)

## Symptom

A freebsd/arm64 program with a `use c` binding taking a Str faulted with
SIGSEGV at address 0x6 in memcpy, called from the binding's adapter
(xo_cffi_cstr). The adapter's `bl malloc` had been linked as a `nop`, so
x0 still held `n + 1` (6 for "hello") when memcpy wrote to it. The link
reported nothing although no object defined malloc.

## Cause

compiler/macho/elflink.xo `undefined_in` kept the binding of the first
reference it saw: the runtime's weak reference to malloc came before the
adapter's strong one, so malloc was a weak undefined name, which a static
link resolves to 0 (a call to it a nop).

## Fix

A strong reference anywhere makes the name strong (as every ELF linker
does); the static link then names it as not defined. macho_test.xo
covers it. When a C call does nothing, check the disassembly for a `nop`
where the `bl` was.
