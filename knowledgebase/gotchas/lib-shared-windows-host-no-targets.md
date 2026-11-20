# `xo build --lib=shared` failed on a Windows host unless --targets named it

Status: Observed (2026-11-15, the owner's 0.51.1 Windows evaluation, bug 23; fixed 2026-11-20, task #244)

## Symptom

On windows/amd64, `xo build --lib=shared -o min.dll DIR` failed with
`prog-windows.0.s: not an x86-64 or ARM64 COFF object`; the header was
written, no DLL. `--targets=windows/amd64` built it.

## Cause

Three choices in compiler/xo/main.xo read the DLL case from the flag's
text, `o.targets.starts_with("windows/")`: the lazy runtime start
(clib_lazy), the assembler and linker (Xo's own for a DLL, cc for
darwin and linux), and the import library. With no --targets the
string is empty, so a Windows host took the cc steps: assembly text
went to the PE linker as if it were an object. Every DLL test cross
built with --targets from another host, so none saw the default.

## Fix

`lib_host_targets` names the host's target (windows/amd64 or
windows/arm64) for a library build with none given on a Windows host,
once, before anything reads it. Tests: compiler/xo opts_test.xo (the
choice, on every host) and cmd/xo TestCLibWindowsDLLHostDefault (the
build, on a Windows host).

A choice that depends on the target reads the resolved target, not the
flag: an empty flag means the host.
