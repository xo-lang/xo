# A byte offset passed as an rt.slot_* index

Status: Observed (2026-10-19)

## Symptom

Every program built for windows/amd64 from 2026-10-17 to 2026-10-19,
with either backend and either link (the native backend with the C
runtime or Xo's PE linker, LLVM), faulted with 0xC0000005 before main
on real Windows (Server 2025, the xo-windows runner, run 39313079963),
an empty program too, with no fault report on stderr. Under Wine with
Box64, and on Linux and macOS, the same programs passed.

## Cause

std/native/llrt/ctx.xo add_root and roots() (67c52321, decision 0251's
registry of root contexts) passed SV_ROOTS, 288, to rt.slot_load,
rt.slot_cas and rt.slot_add. Every SV_* constant is a byte offset (for
rt.mem_load and rt.mem_store), but an rt.slot_* index i is the word at
xs + 8 * i: the registry was at byte 2304 of the 296 byte services
record, past its end. Elsewhere that heap word was 0, so the CAS put the
registry there (overwriting memory past the record); on Windows the
process's environment strings lay there ("PATHEXT=" in the minidump), so
the CAS failed and the `lock xadd` through those bytes faulted at the
first root context.

## Fix

SV_ROOTS_SLOT (36) for the slot calls; std/native/llrt/ctx_test.xo
checks that roots() is the record's word at byte SV_ROOTS (it failed
before the fix: 0 against the registry's address). When a record's
offsets are bytes, give the slot index its own constant
(pool.xo P_ILEN beside P_ILEN_AT), and check new slot uses of a
record's constants against the record's size. Found from the fault
offset of the Application Error events (windiag.ps1), mapped with the
programs' COFF symbol table.
