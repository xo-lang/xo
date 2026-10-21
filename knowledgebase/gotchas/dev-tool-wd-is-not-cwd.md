# A dev tool's --wd is not its working directory

Status: Observed (2026-10-21, Xo Publish, compiler/wasm)

## Symptom

`wasm --wd=/abs/prog /abs/prog`, run from another directory (the
repository root), prints `== unsupported no \`main\` function in the root
module` for a program that has one; `wasm --wd=/abs/prog .` checks the
process's directory (the whole repository), not /abs/prog.

## Cause

Every compiler/* dev tool that takes `--wd` (check, ssa, llvm, gen,
arm64, wasm) resolves its directory arguments against the process's
working directory, and uses `--wd` only as the checker's notion of the
working directory (Checker.set_mod_env): positions are printed relative
to it, and the checker's load_dir turns a directory below it into a path
relative to it (rel_dir) and then reads that relative path from the
process's directory. The selfhost tests rely on the split: they run the
tools from the repository root with directories relative to it and
`--wd=` the test package's directory.

## Fix

Run the tool in the directory you pass as `--wd` (the test's
`cmd.Dir`), or leave `--wd` out (it defaults to the process's
directory). Directory arguments are relative to where the tool runs.
