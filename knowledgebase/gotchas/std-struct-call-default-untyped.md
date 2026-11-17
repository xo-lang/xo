# A std implementation struct's call default reached the SSA builder untyped

Status: Observed (2026-11-17, xw-1 and xo-lang-1, task #279)

## Symptom

`xo check` of a program that mounts a std/ui host reported
`std/native/websocket.xo:96:28: error XO1201: not available on the native backends ... this call`
at `comp: Mutex[CompState] = Mutex[CompState].new(...)`, while `xo build` of the same program
succeeded. `XO_FE_CACHE=off` or `XO_BACKEND_CACHE=off` made it go away.

## Cause

A field default of a std implementation struct (std/native, decision 0074) is typed only when a body
that builds the struct is checked. Two paths left it untyped when the builder lowered it: std's record
joins a struct's tree lazily (`need_tree`), which typed parameter defaults for functions and nothing for
structs; and a front end cache hit or a restored checker skips the body that would have typed it. The
default `[]`, `{}` and enum cases are lowered from the field's type, but a call needs its call
information, so the lowering reported "this call".

## Fix

`Checker.need_tree` types a joined struct's field defaults (`Checker.defaults_of`, under the struct's own
file), and the native build types every loaded std implementation struct's defaults before lowering
(`ssa.Builder.type_std_defaults`). Test: compiler/ssa stddefault_test.xo (the call information removed
as a restored checker has none). A new kind of std default (a call, a method call) should be checked
this way; look for "this call" XO1201 at a std file under `xo check` with the caches on.
