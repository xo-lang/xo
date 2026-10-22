# An LLVM aggregate's fields cannot be described from the aggregate

Status: Observed (2026-10-17, decision 0252 stage 4 on LLVM, Xo Lang 3)

Symptom: the LLVM backend binds Str, struct, tuple, function, and enum
values as first class aggregates (`%xo.str = { ptr, i64 }`, a struct of
the fields, `{ i64, [n x i64] }` for an enum). Describing them to the
debugger from the aggregate did not work under clang -O2 -g:

- `llvm.dbg.value(metadata %xo.str %s, ...)` of an aggregate built in
  the same block: `DEBUG_VALUE: s <- undef`;
- of an aggregate held in virtual registers (a parameter, a phi):
  `DEBUG_VALUE: s <- $x21`, the first register as the whole 16 byte
  variable (wrong), and no location in the DWARF;
- an `extractvalue` per field with a fragment dbg.value
  (`DW_OP_LLVM_fragment`): every part undef, because an instruction that
  only debug information uses is dead; EarlyCSE deletes it and LLVM's
  salvageDebugInfo has no rule for extractvalue.

Cause: instruction selection does not split a dbg.value of an aggregate
into its registers, and metadata uses do not keep an instruction alive.

Fix (compiler/llvm debug.xo): describe a part by the scalar the block
inserted there. def and set record each insertvalue (aggregate, index,
operand) while debug information is on; debug_bind walks a part's path
through those chains and constant structs (`{ ptr @.rs.2, i64 3 }`,
zeroinitializer) to its scalar, and emits a fragment dbg.value of it
(none for a global's address, a string literal's object: on Linux
code generation drops `dbg.value(ptr @.rs.2)`, gdb shows `obj =
<optimized out>`, and on macOS dsymutil would relocate it as the
function's, gotchas/dsymutil-loclist-address-function-offset.md). A
part not found that way (an aggregate that arrived whole: a parameter, a
call's result, a phi) gets `poison`, which ends the part's earlier
location instead of letting it go on showing an earlier value (the
`{}` map's zero words, before this). And where the code itself
extracts a part of a value that a variable is bound to (and to no
other value in the function: a parameter, `let x = f()`), a fragment
dbg.value of that live extractvalue follows it (dbg_note_ext; a part
that is itself an aggregate, a Str in a struct, is followed through its
own extractvalues). So on LLVM a struct, enum, or string literal shows
by its parts, Lists, Boxes, and handles (one pointer) and the scalars
fully, a parameter or call result by the parts the code extracts
(report's `name.len`, `m.n`), and its other parts as empty pieces: lldb
prints an empty piece as 0 or NULL, gdb as `<optimized out>`.

Next, for the parts the code never extracts: (2) llvm.fake.use of each
part in debug builds keeps them alive, a code cost in every default
build (lead, 2026-10-17: skipped unless measured at zero for default
builds, 0252 point 5); (3) passing Str and small structs as scalars in
the LLVM backend's calling convention (backlog: a combined speed and
debuggability item; it touches the ABI and every call). A new debug
only extractvalue does not help even after a live one: EarlyCSE
deletes dead instructions before it merges equal ones.

Also observed (2026-10-22, e5a34b0c3): a part the code extracts is only
described while it stays a run time value. Once the LLVM backend stored a
new list's header where LLVM sees it, clang folded TestDebugTypes' m.n
(the map main fills with two constant keys) to 2 after inlining report;
the fragment dbg.value went with the extractvalue, m had no location left
in the inlined report, and lldb answered "use of undeclared identifier
'm'". The test program now adds a key under an environment variable that
is never set, so m.n is read at run time (and builds m before xs, whose
location in main the branch would otherwise end). A variable whose parts
are all constants after optimization can still show nothing on LLVM.
