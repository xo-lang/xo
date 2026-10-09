# The LLVM lowering must visit a value before its uses

Status: Observed (2026-10-08, decision 0074)

Symptom: clang rejects the generated IR with `use of undefined value
'%v116'` for a program whose SSA verifies, after generated code (a JSON
decoder) created a join block before the blocks that jump to it.

Cause: internal/llvm lowers some values as aliases of other operands
(`alias`: a payload read through a stack slot is named `%t54`, not
`%v116`). A use lowered before its definition prints the default name,
which then never gets defined. Block order in `f.Blocks` is creation
order; the builder's own code happened to create dominating blocks
first, but `cleanup` replaces a trivial block parameter of a join with
the value from a later block, so the use moves before the definition.

Fix: `GenerateSSA` lowers the blocks in reverse postorder
(`reversePostorder`, a block after every block that dominates it) and
still prints them in their original order. The arm64 backend has no
aliases by name and was not affected.
