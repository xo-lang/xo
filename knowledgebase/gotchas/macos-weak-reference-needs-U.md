# macOS ld rejects a weak reference to a symbol nothing defines

Status: Observed (2026-10-15, decision 0232).

## Symptom

A program with a weak reference (`declare extern_weak` in LLVM IR,
`.weak_reference _sym` in assembly, C's `weak_import`) to a symbol no
linked image defines, such as `__tsan_acquire` in a build without
ThreadSanitizer, fails to link on macOS:
`Undefined symbols for architecture arm64: "___tsan_acquire"`. The same
program links on Linux, where an undefined weak symbol is 0.

## Cause

ld64 treats a weak reference as "may be missing at run time", not "may
be missing at link time": the symbol must still come from some dylib
on the link line (weak linking of an OS version's API).

## Fix

Pass `-Wl,-U,_sym` for each such symbol: the link leaves it to dyld,
which binds a weak import it cannot find to 0. selfhost/xo link.xo
(weak_undefined_args) and internal/xotool (WeakUndefinedFlags) add it
for every weak reference of the program's .s or .ll; selfhost/macho's
own linker marks such an import weak in the chained fixups. A new place
that links Xo output with the system linker on macOS needs the same.
