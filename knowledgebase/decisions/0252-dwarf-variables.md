# 0252: DWARF types, variables, and locations for native builds

Status: Proposed (claimed 2026-10-17 by Xo lang 3; the lead assigned
the work as the full debugger milestone)

## Context

Decision 0239 gave both native backends DWARF line tables, subprograms,
and inlined frames: lldb breaks and steps by `.xo` line and names
inlined callees, but `frame variable` shows nothing, because no
`.debug_info` entry describes a type, a local, or where a value lives.
Its point 4 left types, variables, and locations to a later decision,
with the debugger; this is that decision.

What the compiler has to work with:

- SSA values carry no source names. A `let`, a `var`, a parameter, or a
  pattern binding becomes values (block parameters where a `var` meets
  a join), and the name is gone after the builder.
- Every native value is a sequence of 64 bit words (arm64 gen.xo
  layout, the LLVM emitter's the same): Int, Bool, List, Box, Handle one
  integer word; Float one floating point word; Str two (the object and
  the length); a function value two; a struct or tuple the words of its
  fields in order; an enum the tag, then the most payload words of any
  variant. A list object is its count, length, capacity, then its
  elements; a string object its count and capacity (16 bytes, obj.xo
  S_HDR), then its bytes; a Map is a struct of such objects.
- arm64's register allocator gives each virtual register one home (a
  physical register or a spill slot) with holes in its live range;
  values are split across several virtual registers only word by word.
- The LLVM emitter already writes DICompileUnit, DISubprogram, DIFile,
  and inline DILocations; LLVM can carry DILocalVariable through its
  optimizations with debug value records (`#dbg_value`, LLVM 19 and
  later; `llvm.dbg.value` calls before).
- lldb on the Mac is lldb-2103 (Apple LLVM 21). It reads DWARF 5,
  location lists, composite locations (DW_OP_piece), and loads Python
  formatters.

## Decision

1. **Bindings in the SSA.** The SSA builder records each source binding
   (parameter, `let`, `var`, `for` variable, match and `if let` binding)
   as a debug variable of the function (ssa Func.dvars: name, type,
   declaration line, lexical scope) and, wherever the binding gets a new
   value, a binding record (Func.dbinds: variable, value, the line from
   which it holds). Records are a side table, not instructions, so no
   pass has to step around them; passes that delete or copy values
   (dead code, the arm64 inliner, rc) drop or copy the records of those
   values. Scopes are DW_TAG_lexical_block ranges from the block
   statement lines.

2. **Types.** One DWARF type per Xo type the variables use, with
   DW_LANG_C11 as the unit's language so lldb's clang type system reads
   them, and Xo's names (`List[Int]`, `Map[Str, Int]`):
   - Int, Float, Bool, UInt8 to UInt64, Int8 to Int32: DW_TAG_base_type
     (8 bytes for Bool: a word, DW_ATE_boolean).
   - A struct or tuple: DW_TAG_structure_type of 8 bytes per word, a
     member per field at its word offset (the memory layout is the word
     layout), so a struct split across registers is one composite
     location.
   - An enum: DW_TAG_structure_type with a DW_TAG_variant_part whose
     discriminant is the tag word and a DW_TAG_variant per variant
     (its fields as members at their payload offsets), plus a
     DW_TAG_enumeration_type for the tag naming the variants. If lldb's
     clang type system does not show variant parts (stage 2 checks it
     first), the fallback is the tag and the payload words as members,
     with the formatter of point 4 choosing the variant.
   - Str, List[T], Map, Set, Box, function values, handles: a structure
     of their words (Str: `obj`, a pointer to a structure `count, cap`
     with the bytes after it, and `len`; List: a pointer to a structure
     `count, len, cap` and a zero length array of T after it), so the
     raw words are always visible.

3. **Locations.**
   - LLVM: a DILocalVariable per debug variable (DW_TAG_formal_parameter
     for parameters) and a `#dbg_value` record at each binding record,
     with DW_OP_LLVM_fragment per word for multi-word values; LLVM
     computes the locations and their ranges, through its optimizations,
     as it does for C. Variables that the optimizer removes show as
     optimized out, as in clang -O2.
   - arm64: after register allocation, each binding record's words map to
     their virtual registers and those to their homes; the emitter
     writes a DW_TAG_variable with a location list (DW_LLE_offset_pair
     between labels, as the line rows use labels) of
     DW_OP_regN (x0 to x30 as 0 to 30, d0 to d31 as 64 to 95) or
     DW_OP_breg29 OFFSET for a spill slot, joined by DW_OP_piece 8 for
     multi-word values, over the ranges where each home holds the word,
     cut at the next binding record of the same variable. selfhost/macho
     dsym.xo and elflink carry `.debug_loclists` like the other
     sections. A word with no home at a pc (dead, or a constant the
     emitter did not keep in a register) is an empty piece: lldb shows
     it as unavailable; constants get DW_OP_constu in a later step.

4. **Pretty printing.** A Python formatter, std/lldb/xo.py, shipped in
   the std tree and loaded with `command script import`, gives
   summaries and children for Str (the text), List (the elements), Map
   and Set (the entries), Box, and enums if point 2's variant part is not
   shown natively. `xo debug PROG [ARGS]` starts lldb with it loaded and
   the program's dSYM; darwin dSYMs also carry it under
   Contents/Resources/Python for lldb's own loading when the user allows
   it. The formatter reads the documented object layouts only; a change
   of a layout changes the formatter in the same commit (a test reads
   every kind).

5. **Flags and cost.** Variables go with the line tables: on for build,
   run and test, off with `-s` and for `--release` unless
   `--debug-info` (0239 point 2). The function cache key includes the
   binding records when debug information is on. Code generation does
   not change with debug information (a test compares the text of a
   program built with and without it), so a debug build profiles like a
   stripped one.

6. **Test.** internal/arm64 and internal/llvm tests build a program with
   a struct, a List[Int], a Str, a Map, and an enum local, run it under
   lldb in batch mode to a breakpoint on a known line, and compare
   `frame variable` (with the formatter) against a golden, on the arm64
   backend (macho and --ld=cc) and on LLVM, plus `dwarfdump --verify`
   on the dSYM. Linux: the same program's `.debug_info` and
   `.debug_loclists` checked by llvm-dwarfdump in the Docker image.

## Stages

1. SSA debug variables and binding records (builder, inliner, dead code,
   rc cache), with a dump in `xo dev dump ssa`; no output change.
2. arm64 types and locations for Int, Float, Bool, structs, tuples
   (base types, structures, location lists, pieces), dsym.xo and elflink
   carrying `.debug_loclists`; the test for those types; check lldb on
   a variant part.
3. LLVM: DILocalVariable, DI types, `#dbg_value`; the same test.
4. Enums, Str, List, Map, Set, Box; std/lldb/xo.py; `xo debug`.
5. Lexical scopes, constants (DW_OP_constu), captured variables of
   closures (through the environment), the profiler and fault reports
   unchanged (they read only lines).

## Consequences

- lldb prints locals and fields of native Xo programs at breakpoints
  and while stepping, on both backends; Instruments and crash reports
  keep working as with 0239.
- Debug builds grow again (`.debug_info` types and `.debug_loclists`;
  measured per stage); release builds strip as before.
- Every pass that removes or copies SSA values keeps the binding
  records consistent; the SSA verifier checks that each record names a
  live value of the right type.
- The object layouts become an interface of the formatter; changing one
  changes xo.py and its test.

## Alternatives

- Variables as instructions in the SSA (an LLVM-style debug intrinsic):
  every pass would have to skip or move them; a side table keyed by
  value does not change what passes see.
- Spill every variable to a stack slot in debug builds (clang -O0):
  simple locations but different code from the release build, so a bug
  that depends on code generation hides under the debugger.
- Rust-style DW_LANG_Rust: lldb's clang type system does not read it;
  Apple's lldb has no Rust support.
- A private debugger (a Delve for Xo, consider-later): DWARF works with
  lldb, Instruments, and every other tool first; a Xo debugger can read
  the same DWARF later.
