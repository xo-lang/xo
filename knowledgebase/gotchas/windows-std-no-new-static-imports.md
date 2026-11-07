# std must not add a static Windows import an older xo lacks

Status: Observed (2026-11-07)

## Symptom

On the Windows offload host, building xo from a branch failed with
`undefined symbol GetProcessHeap` from the seed xo and from the newest
earlier build, though the branch's own xo would have linked it.

## Cause

A Windows no-CRT build links with Xo's own PE linker, which resolves
imports only from the import table compiled into that xo
(compiler/macho/peimports.xo). xo on Windows is built by an older xo,
whose table predates the branch: a std/native file that declares a new
DLL function (here sys_windows.xo's HeapAlloc and GetProcessHeap for the
SQLite CRT shim) cannot link under it.

## Fix

Use functions an older table already has (the shim's malloc and free
went to libcmem, localtime to GetDynamicTimeZoneInformation), or add the
import to peimports.xo first and cut a seed before std uses it
(gotchas/seed-older-than-intrinsics.md is the same shape for runtime
intrinsics).

compiler/macho/peimports.xo also holds entries added by hand that
scripts/win-import-table.py does not produce (crash.xo's
SetUnhandledExceptionFilter, IPHLPAPI.dll's GetAdaptersAddresses, whose
DLL name the SDK spells IPHLPAPI.DLL): regenerating the table drops them
and fails internal/repotest TestPEImportsCoverSysWindows. Add new
entries to the existing table instead (2026-11-07, decision 0534 stage 2).
