# The split LLVM build matches symbols by text: `@"x"` is not `@x`

Status: Observed (2026-10-15, decision 0234).

**Symptom.** A split build (XO_LLVM_SPLIT=3, TestGoldenSplit) fails in
clang with `use of undefined value '@xo_entry'` at a
`ptrtoint ptr @"xo_entry"`, while the single module build of the same
program works.

**Cause.** LLVM treats `@"xo_entry"` and `@xo_entry` as one symbol, but
compiler/xo split.xo finds a module's references and definitions by
their text (symbol_at): the definition `define void @xo_entry()` and the
quoted reference are different strings, so the module that holds the
reference got no declaration of it.

**Fix.** Write a C name plain (unquoted) when it is an identifier, as
the emitter's own definitions are (compiler/llvm call.xo c_sym). Quote
only names that need it, and the same way at every use and definition.
