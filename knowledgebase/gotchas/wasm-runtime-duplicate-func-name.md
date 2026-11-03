# A wasm runtime function name given twice took the other's calls

Status: Observed (2026-11-02)

## Symptom

Every wasip2/wasm command that used files built, then failed `wasm-tools
validate` with `func N failed to validate ... type mismatch: expected i64,
found i32`, in a function whose text was right.

## Cause

compiler/wasm's runtime is WebAssembly text assembled from several files
(rt.xo, rt4.xo, rt8.xo, ...). rt8.xo imported wasi:filesystem functions
named `$fs_open`, `$fs_stat`, `$fs_rename` and `$fs_symlink`, names rt4.xo
already gives its own functions. The assembler (asm.xo) kept the last
definition of a name, so rt4's calls of its own `$fs_open` went to the
import, with another signature.

## Fix

asm.xo now fails the module with `wat: the function name $NAME is given
twice`. Give a new runtime file's functions and imports a prefix of their
own (rt8.xo's imports are `$wfs_*`), and grep the other rt*.xo files for
a name before adding it.
