# A bounds check has two sites, and the length in one of them is a register

Status: Observed (2026-10-26).

Symptom: teaching the arm64 bounds-check elimination a new fact
(`b.len() >> k < b.len()` once the length is at least 1) changed nothing:
the check was still emitted for `if b.len() > 0 { b[b.len() / 2] }`, and
the fact was not lost and the pattern was not wrong.

Cause: two separate things. `compiler/arm64` emits the index check in two
places: `g.bounds_check` in calls.xo for `list.index`, `list.set` and
`list.take`, and the `bytes.index` lowering in the same file, which has
its own compare and calls `g.proved_index` (Bytes indexes are not lists and
never reach `bounds_check`). A rule added to one of them does nothing for
the other. Second, `bounds_check` works on the value the backend keeps in
registers: the length there comes from `g.list_len` and is a *register*,
while `G.bounds`, `len_root` and the `Proof` predicates (`rem_of_len`,
`proven`, `nonneg`) take *SSA value* indexes. Passing a register to them
compiles and returns a wrong answer silently, since both are `Int`.

Fix: put the rule in the `Proof` (fieldlicm.xo `div_of_len`, with
`len_pos` for the `len > 0` fact found by container root, because a second
`Str.len` of the same value is a different SSA value) and call it from both
sites with the call's own SSA arguments (`x.args[0]`, `x.args[1]`).
`xo dev dump ssa` shows the shape without a rebuild, which is how the Div
of `x.args[0..1]` and the `call Str.len` argument were confirmed.

2026-10-26, the same domain mixing was already a soundness bug in the two
swap sites (fixed in be41164b8). `bounds_check` read the index and the list
off the operation's own arguments (`g.arg(v, 1)`, `x.args[0]`,
`x.args[1]`), which is right for `list.index`, `list.set` and `list.take`
and wrong for the swaps, whose second pair is elsewhere:
`rt.list_swap(xs, i, j)` called `bounds_check(v, l, j)`. So j was checked
with i's pair: when a dominating branch proved i in range, j's check was
dropped and the swap stored past the end of a list, and `less_fact`
recorded `i < len` for the wrong list. `proved_index` and `less_fact` were
also handed the index's *register* where they index SSA maps. The fix
passes the pair explicitly, `bounds_check(v, l, i, sl, si)`, and all seven
call sites give their own (`rt.list_swap` j: `x.args[0]`, `x.args[2]`;
`rt.list_swap2` j: `x.args[2]`, `x.args[3]`).

Under that sat the reason no swap index was checked anywhere: `b.rt` built
the call with no source position, and `bounds_check` returns early on a
value without one, since a fault has to name its location. The parent's asm
for a guarded swap is eight instructions with no compare. `b.rt_at` records
the call site, and `rt.list_swap` and `rt.list_swap2` joined
`faulting_rt`. A test of the swap shape cannot be a program's own
`rt.list_swap` call: `use std/native/rt` resolves but the call is refused
(`XO1201`, not available on the native backends), so the shape is reached
through std's own `internal.crypto_op` probe and the two `internal`
helpers (the golden `list_swap_guard` and the patterns
`internal.list_swap_at` and `internal.list_swap_both`).
