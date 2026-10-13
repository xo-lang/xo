# A browser worker's wasm stack holds every declared local, and it is smaller than Node's

Status: Observed (2026-10-12), selfhost/wasm stage 3 (decision 0179).

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
(`selfhost/wasm/coalesce.xo`): exact liveness over the emitted wasm
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
