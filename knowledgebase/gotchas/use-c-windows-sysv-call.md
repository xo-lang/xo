# A `use c` call on windows/amd64 used the System V convention

Status: Observed (2026-11-07)

## Symptom

std/sqlite's golden built natively on Windows (xo's x86 backend) crashed
with `EXCEPTION_ACCESS_VIOLATION write at 0x0` in cabi.cffi_str, called
from sqlite.version: the adapter wrote a Str result through a zero
length pointer.

## Cause

The x86 emitter uses System V internally on Windows and reaches C only
through `xo_w64_<sym>` thunks (x86win.xo), but a `use c` binding call
(`rt.c_<module>_<hash>_<function>`) took the generic runtime call path,
which calls `xo_<name>` with System V and no thunk. The adapter, which
clang compiled for Win64, read its arguments from rcx, rdx, r8, r9, so
the out slot arrived as 0. Linux and darwin share the convention, so
nothing else showed it.

## Fix

calls.xo's rt_call routes an `rt.c_` call on windows/amd64 through
w64_sym with the argument words' kinds (a Str or closure two words, a Str
result's out slot last).
