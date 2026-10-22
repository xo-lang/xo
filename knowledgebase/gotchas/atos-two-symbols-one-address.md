# atos names nothing at an address two symbols share

Status: Observed (2026-10-22)

Symptom: `atos -o hello 0x100000948` (xo_f_main, the program's first
function) printed only `0x0000000100000948 (in hello)`, no name and no
line, while every other function resolved to name and file:line from
the same executable's DWARF, and lldb resolved main there. With a dSYM,
atos printed `<deduplicated_symbol> (in hello) + 0`. A `--race` report's
frame in such a function would lose its name the same way.

Cause: the symbol table had two local symbols at that address,
`_xo_f_main` and `_xo_utext_<part>`, the start label of the part's text
for the asynchronous preemption table (compiler/arm64 apreempt.xo
unsafe_table). CoreSymbolication, which atos uses, treats an address
with two names as deduplicated code and names neither. Each part's
first function had the problem (about 12 in a parallel build).

Fix: Xo's Mach-O linker (compiler/macho link.xo symbols) leaves
`_xo_utext_` and `_xo_utend_` out of the executable's symbol table; the
table's relocations against them are resolved at link time and the
profiler already skipped them. TestDebugLineDarwin runs atos on
`_xo_f_main`. A new marker symbol at a function's address needs the same
treatment, or an assembler-local (L) name.
