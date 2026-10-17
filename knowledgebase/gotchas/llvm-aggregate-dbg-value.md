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
`{}` map's zero words, before this). So on LLVM a struct, enum, or
string literal shows by its parts, Lists, Boxes, and handles (one
pointer) and the scalars fully, and an aggregate parameter or call
result as `<variable not available>`.

Next: describe the parts of aggregates that arrive whole. Options, in
order of likelihood: (1) a fragment dbg.value on the code's own
extractvalue of a bound aggregate (in def and set, when the operand is a
variable's value of the block): a live instruction, so it survives; a
new debug only extractvalue does not, even after a live one, since
EarlyCSE deletes dead instructions before it merges equal ones; (2) llvm.fake.use of each part in debug builds (keeps
them alive: a code cost in every default build, to measure against
0252 point 5); (3) passing Str and small structs as scalars in the LLVM
backend's calling convention (a larger change with its own benefits).
