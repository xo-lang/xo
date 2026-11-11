# `xo test --std-module`: an importer through a declaration file saw an empty module

Status: Observed (2026-11-11, Xo Lang 3 found it with std/textproto; Xo Lang 2 fixed it)

## Symptom

With std's test variants cached (`xo dev stdcache --test`, or any earlier
`xo test`), `xo test --std-module=multipart` failed on the native backend:

    std/http.xo:141:47: error XO0601: module `multipart` has no type `Form`

and `MultipartErr` at http.xo:331, 340, 344. Lang 3 first met it with a new
module, textproto, once std/multipart, std/mail and std/smtp imported it.
`XO_STD_EXPORT=0`, `XO_SEPCOMP=0`, `XO_FE_CACHE=off` (it turns the record
off too) or an empty cache (no variant: the whole program is compiled) each
made it pass. The LLVM backend passed.

## Cause

A `--std-module` run builds the module and every std module importing it
from source (compiler/xo/stdcache.xo std_source_set, decision 0596), and its
std record leaves that set out: the set's modules are declared past the
record's block, so nothing outside the set may import one inside it.
std_source_set read the `use` lines of std/native's sources and of the
merged modules' std/NAME.xo, not the declaration file std/NAME.xo of a module
not merged. std/http.xo imports std/multipart; std/native/http.xo does not.
So http was outside the set: it was declared, and its types resolved, with
the record's modules, before multipart was declared, and found no `Form`.

## Fix

std_source_set reads every std/NAME.xo and names both `native/NAME` and
`std/NAME` for a module not merged (multipart's set: 10 to 17 modules).
internal/arm64 TestSepcompStdModuleEdits runs `--std-module=multipart` after
building the variants (it fails without the fix with the errors above).

## How to avoid

An import graph read from source text has to read every file that can hold
a `use` line of the module: implementation, declaration file, merged file.
A from-source set that misses an importer shows up only as a check error in
that importer, far from the cause.
