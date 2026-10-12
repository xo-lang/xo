# A std list default needs a default in the native implementation

Status: Observed (2026-10-11, decision 0172).

## Symptom

A native build of `os.proc.cmd("cat").output()` failed with XO1201
"this list literal" located at std/prelude.xo (the `args: List[Str] =
[]` of `Proc.cmd`), although the native prelude implemented `proc_cmd`.

## Cause

`callAs` takes a missing argument from the implementation's parameter
default first and falls back to the std declaration's. The std
declaration files are checked as signatures: the `[]` there has no
type the SSA builder can use, so lowering it fails. A Str or Int default
works from the declaration; an empty list (or map) literal does not.

## Fix

Repeat the default on the native implementation's parameter
(`fn proc_cmd(p: Proc, name: Str, args: List[Str] = []) -> Cmd`), where
the literal is checked with its type.
