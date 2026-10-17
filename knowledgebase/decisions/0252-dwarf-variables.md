# 0252: DWARF types, variables, and locations for native builds

Status: Accepted (the lead, for the owner's debugger milestone,
2026-10-17; proposed by Xo lang 3)

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
     with the Xo debugger (0253) choosing the variant.
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

4. **Readers.** Xo's own debugger (decision 0253, owner 2026-10-17)
   reads these types and locations; lldb, gdb, and other DWARF tools
   read them too (a cross check, not the product). No lldb formatter and
   no lldb-launching `xo debug` (superseded by 0253 before it was
   built); Str, List, Map, and enum values are shown by the Xo debugger,
   and to lldb as the structures of point 2.

5. **Flags and cost.** Variables go with the line tables: on for build,
   run and test, off with `-s` and for `--release` unless
   `--debug-info` (0239 point 2). The function cache key includes the
   binding records when debug information is on. Code generation does
   not change with debug information, a hard gate (lead, 2026-10-17):
   a test compares the code of every golden built with and without it,
   so a debug build profiles like a stripped one. The arm64 backend
   meets it by construction. The LLVM backend does not (stage 3, lead
   2026-10-17): its dbg.value calls change register allocation and the
   MachineOutliner's choices in some functions, as clang -g can, and
   carrying the metadata into every build costs too much; there the gate
   is that every golden built with `--release --debug-info` runs with the
   output of its `--release` build (TestDebugInfoSameOutputLLVM).

6. **Test.** internal/arm64 and internal/llvm tests build a program with
   a struct, a List[Int], a Str, a Map, and an enum local, run it under
   lldb in batch mode to a breakpoint on a known line, and compare
   `frame variable` against a golden (a cross check of the DWARF; the
   Xo debugger's own tests come with 0253), on the arm64
   backend (macho and --ld=cc) and on LLVM, plus `dwarfdump --verify`
   on the dSYM. Linux: the same program's `.debug_info` and
   `.debug_loclists` checked by llvm-dwarfdump in the Docker image.

## Stages

1. SSA debug variables and binding records (builder, inliner, dead code,
   rc cache), with a dump in `xo dev dump ssa`; no output change.
2. arm64 types and locations for Int, Float, Bool, structs, tuples
   (base types, structures, location lists, pieces), dsym.xo and elflink
   carrying `.debug_loclists`; the test for those types (dwarfdump
   --verify and the entries read back; lldb `frame variable` as a cross
   check); check lldb on a variant part.
3. LLVM: DILocalVariable, DI types, `#dbg_value`; the same test.
4. Enums, Str, List, Map, Set, Box as the structures of point 2
   (the Xo debugger, 0253, shows their contents).
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
- The object layouts become an interface of the Xo debugger (0253);
  changing one changes its reader and their test.

## Alternatives

- Variables as instructions in the SSA (an LLVM-style debug intrinsic):
  every pass would have to skip or move them; a side table keyed by
  value does not change what passes see.
- Spill every variable to a stack slot in debug builds (clang -O0):
  simple locations but different code from the release build, so a bug
  that depends on code generation hides under the debugger.
- Rust-style DW_LANG_Rust: lldb's clang type system does not read it;
  Apple's lldb has no Rust support.
- For the LLVM backend's gate (stage 3, measured 2026-10-17 with
  clang's instructions retired and peak RSS over the full LTO pipeline,
  -O2 -flto -c and the post link -O2 codegen, of a 1701 function
  program, against main's IR):
  (a) emit the metadata in every build and drop the DWARF at the link
  (no dSYM; -Wl,-S on Linux), the same code by construction: +6.4%
  instructions and +22% peak RSS for -s and release builds (and the
  self build -s 2306 s to 3170 s, wall clock under load); rejected, a
  self build that much slower hits every fixed point and seed cut.
  (a) with dbg.value only at definitions (none for a block's parameters):
  +2.4% instructions, +10.6% RSS, over the 10% bound, and a variable
  carried around a loop shows only its first value; rejected.
  (b) DILocalVariables without dbg.value: the same code, but lldb shows
  no LLVM locals; rejected. FullDebug alone costs nothing measurable
  (+0%); the cost is the dbg.value calls.
- A private debugger (a Delve for Xo, consider-later): DWARF works with
  lldb, Instruments, and every other tool first; a Xo debugger can read
  the same DWARF later.

## Implementation

- Stage 1 (2026-10-17): ssa Func.dvars (DVar: name, type, declaration
  line, parameter, inline site) and Func.dbinds (DBind: variable, value,
  line, 0 for a join's parameter). The builder records them in
  write_var and new_param for checker locals (Obj.kind Local, not `_`);
  passes.xo remaps them in remove_trivial_params and renumber (a record
  whose value is gone is dropped); the arm64 inliner copies the callee's
  under the call's site and replace_uses moves them; the verifier checks
  their indexes; the rc cache (v3) keys and stores them. `xo dev dump
  ssa --debug-vars` prints them (cmd/xo TestDebugVarsDump); the default
  dump is unchanged. No code generation change: the assembly of five
  programs (http, text, json, nbody, a test program) is byte identical to
  main's, and the fixed point holds.
- Stage 2, arm64 (2026-10-17): regalloc keeps each virtual register's
  live ranges from before coalescing (G.vranges, debug information on);
  dwvars.xo turns each binding record's words into instruction spans in
  their homes (DW_OP_regN, DW_OP_regx for v registers, DW_OP_breg31 for
  a spill slot, DW_OP_piece 8 per word of a struct), and per variable the
  binding defined last wins where several are live. Emission labels the
  first emitted instruction at each span boundary (`_dv<n>`, kept by
  pair_mem like the `_dl` rows); the debug record's seventh field carries
  the variables (the function cache keys on dvars and dbinds, v4).
  debug.xo writes base types (Int and the sized integers as 8 byte
  signed or unsigned, Float, Float32, Bool) and structures of words
  (structs and tuples), a DW_TAG_variable or DW_TAG_formal_parameter per
  variable with DW_FORM_loclistx, .debug_loclists, and an empty
  .debug_addr; the unit has DW_AT_loclists_base and DW_AT_addr_base at
  bytes 38 and 42, which selfhost/macho dsym.xo moves (dsymutil, for
  --ld=cc, rewrites the lists to address indexes and needs addr_base,
  gotchas/dwarf-dsymutil-addr-base.md). Inlined copies' variables and
  Str, List, Map, Set, enums come later (stage 4). internal/arm64
  TestDebugVars: dwarfdump --verify and lldb `frame variable` (k = 10,
  total = 0, d = 12 at main.xo:12) for the macho and cc links;
  TestDebugInfoSameCode: every golden's machine code is the same with
  and without debug information. Linux: llvm-dwarfdump --verify and
  gdb `info locals` give the same values (Docker).
- Stage 3, LLVM (2026-10-17): with debug information the compile unit
  is FullDebug, each function has a DILocalVariable per variable of a
  scalar type (Int and the sized integers, Float, Bool; a struct is an
  LLVM aggregate, which a dbg.value does not describe per field),
  parameters numbered by their place in the entry block, and a
  `llvm.dbg.value` after each binding record's value is defined (and at
  a block's start for its parameters), only for a value of the block or
  a constant (a debug use of another block's value makes instruction
  selection carry it across blocks). -s and release builds emit none of
  it, as before. internal/arm64 TestDebugVars's llvm case: lldb prints
  k = 10, total = 0, d = 12 at main.xo:12; internal/llvm
  TestDebugInfoSameOutputLLVM (point 5's gate for LLVM) and
  TestStrippedNoDWARF (-s and --release: no dSYM, no __DWARF, no
  compile unit; --release --debug-info keeps the dSYM).
- gdb on Linux (2026-10-17, GNU gdb 16.3 in scripts/docker-linux.sh's
  Debian image, linux/arm64, internal/arm64 testdata/debugvars; xo
  cross built at main 4141e07c): `break sum`, `break main.xo:12`, `run`,
  `bt`, `info locals`, `info args`. arm64 backend (elflink): `break sum`
  stops at main.xo:6, the first statement (the prologue_end row); at
  main.xo:12 `info locals` gives total = 0, d = 12 (p and pr are
  `<optimized out>` there: their words are dead past line 11) and `info
  args` k = 10; `bt` is sum, main (main.xo:24), xo_entry,
  entry.run_main, cabi.run_main, xo_clone. LLVM backend: sum is inlined
  into main, so `break sum` stops at main.xo:10 (LLVM folded line 6's
  test for k = 10); at main.xo:12 total = 0, d = 12, k = 10; `bt` shows
  the inlined frame (sum at main.xo:12, main at 24 and 22),
  xo_x_run_main, xo_clone. Both backtraces end with gdb's "Backtrace
  stopped: previous frame identical to this frame (corrupt stack?)" at
  xo_clone: the thread's first frame does not mark itself outermost (its
  CFI does not make the return address undefined), so gdb stops on a
  repeated frame instead of a clean end. Fixed the same day: the
  outermost frames on Linux arm64 (xo_clone's child, the no C library
  `_start`, xo_fiber_entry; both backends) have an unwind entry whose
  return address is undefined (`.cfi_escape 0x07, 30`, DW_CFA_undefined
  x30: every assembler Xo uses takes it; inside LLVM's naked functions,
  which Linux clang gives an unwind entry, the escape alone), beside the
  zero fp they already had; gdb's backtraces then end at xo_clone with no
  note, on both backends. Linux goldens in Docker: 183 of 183 on each
  backend. A fiber's backtrace (a task that parks on a channel receive
  and resumes on its own stack, then breaks in work): arm64 work, the
  task's closure, xo.run, tasks.run_entry, tasks.run_claimed,
  sched.fiber_main, cabi.fiber_main, xo_fiber_entry; LLVM work (n=9),
  the closure, xo.run, tasks.run_claimed, xo_x_fiber_main,
  xo_fiber_entry; both end there with no note.
