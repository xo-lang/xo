# A browser worker's wasm stack holds every declared local, and it is smaller than Node's

Status: Observed (2026-10-12), compiler/wasm stage 3 (decision 0179).

**Symptom.** The wasm build of the self hosted checker checked the
structs golden in a dedicated Worker of headless Chrome 154, but on the
recursion golden (and 81 of the 144 LLVM goldens) it stopped with
`RangeError: Maximum call stack size exceeded` about four levels into
the program's nested expressions. Under Node, run.mjs's worker passed
with a 1 MB stack.

**Cause.** Two things, measured with a one function module that
recurses with N declared locals (no locals used):

- V8's baseline compiler (Liftoff, which runs every function first)
  gives every declared local an 8 byte stack slot, i32 or i64, used or
  not: 1,000 locals allowed 100 frames in a 1 MB Node worker, 4,000
  allowed 25. The checker's expression walk (eleven functions per level,
  `Checker.type_of_expr` to `Checker.type_member`) declared 14,217
  locals and 620 parameters, about 120 KB per level; most were never
  used (the leaves of values whose code emits nothing) or never live at
  once, but each value class had its own (plan_locals shares only within
  one leaf signature, over widened intervals, and the dispatch loop's
  br_table made every block reachable from every jump).
- A Chrome dedicated Worker allows about 0.63 of what a 1 MB Node worker
  allows (7,933 against 12,675 frames of an empty function, 62 against
  100 with 1,000 locals). A page cannot set V8 flags or a worker's stack
  size.

**Fix.** The backend coalesces locals after lowering
(`compiler/wasm/coalesce.xo`): exact liveness over the emitted wasm
code, an interference graph, a greedy coloring per type, unused locals
dropped. The cycle went from 14,837 parameters and locals to 2,224, the
module's program functions from 423,218 locals to 73,366, and every
golden the checker checks under Node also checks in the Chrome worker
(144 of 144, from 63). When a wasm module overflows a browser's stack,
count the declared locals of the functions on the cycle, not the
recursion depth alone, and test with a Node worker of about 0.6 MB
(`XO_WASM_STACK_MB`), not 1 MB.

Also seen: V8 refuses a function type of more than 1,000 parameters
(`param count of 1001 exceeds internal limit of 1000`); the checker's
largest has 168.

Locals were not all of it (2026-10-17, xo-4b): V8's baseline compiler
also sizes a frame's spill area by the function's deepest operand
stack, and pushing a wide value whole (a release of the 286 leaves of
`Checker`, an `Expr` of 44 as an argument) made the checker's frames
about 10 KB even with few locals. The backend now passes wide
parameters by address, retains and releases wide values through an
address, and keeps the cold locals of big functions in a memory frame
(decision 0179's addendum "Smaller frames"). To find the cost of a
frame, compute the function's deepest operand stack from its .wat as
well as counting its locals.

Again (2026-11-11, Xo Lang 2): the playground's compiler built from main
overflowed the Chrome worker's stack on every build (the tour's basics
included), where 0.35.0's did not: `Checker` had grown to about 1,143
leaves, and two shapes put a whole one on the operand stack, so even a
69-local function had a 9 KB frame. A call returning a struct of 65 to
1,000 leaves returned it as values (MAX_RESULTS was the engine's limit,
1000), and a wide value read from memory (an sret block, a home, a value
held in memory) was pushed whole and then popped into its locals. Now a
result of more than 64 leaves goes through the caller's block, and
`mem_set` loads such a value a leaf at a time into its locals. The
deepest frame on the build's stack went from 1,143 operand slots to 42,
and a build passes in a 200 KB Node stack (0.35.0's compiler needed
350 KB). Measure with a Node `--stack-size` sweep and a per-function
`locals + deepest operand stack` count of the module, not the Chrome
page alone.

Also (2026-11-11, Xo Lang 2): compiler/wasm home.xo's mem_store and
mem_set move a value through helpers (mem_store_fn, mem_load_fn) in
chunks of at most MEM_FN_MAX (64) leaves, since a helper's call holds a
chunk's leaves on the operand stack; a helper for a whole value would
bring the deep frames back (a Checker is about 1,143 leaves, and a helper
of 1,152 parameters also passed the engine's 1,000).
