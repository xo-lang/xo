# A --wd that is not the working directory checked nothing

Status: Observed and fixed (2026-11-12, Xo Lang 2).

## Symptom

A native build of compiler/wasm, run as
`xoc --std=STD --wd=/abs/run --play check /abs/run/p` from another
directory, printed `{"diagnostics":[]}` for a program with an error, and
`--play build` answered "no `main` function in the root module". Run from
/abs/run, or with relative paths, the same program reported its error.

## Cause

The checker names a path below its working directory relative to it
(Checker.rel_dir), and the file system resolves that relative path from
the process's own working directory. With `--wd` naming another
directory, `/abs/run/p` became `p`, read from the process's directory:
another tree, often with no such directory, and no error said so. The
browser was never affected: under WASI there is no process working
directory, and the page's `--wd=/w` is the only one.

## Fix

compiler/wasm exits 2 with "--wd=W is not the working directory (CWD): run
from W, or leave --wd out" when `--wd` names a directory other than the
process's (check.wd_mismatch, with W's symbolic links followed, since
macOS's getcwd names /tmp as /private/tmp). Unit test: compiler/check
check_test.xo "wd_mismatch".

## Avoid

Run a native compiler/wasm from the directory `--wd` names (as
bench/browserrepl/session.mjs does: cwd work, `--wd=.`). The other
entry points with `--wd` (compiler/check, compiler/arm64, the xo CLI's
internal flag) still assume it is the working directory: the selfhost
tests pass `--wd=` from their own directories, so they were left as they
are.
