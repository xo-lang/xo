# rt.any_of marks a list shared, and everything it holds

Status: Observed (2026-11-19, xo-lang-2, task #271).

Symptom: a profile shows `obj_rl_copy_n` (the out of line copy-on-write
check, xo_rl_copy) high under simple element writes to a list nothing
else holds: 13% of bench/perf/brotli enc1, from write_bits storing into
a `List[UInt8].filled` buffer. The generated code checks `count == 1`
inline and calls the slow path only when that fails.

Cause: putting a value in an `Any` shares it (decision 0334's `share`:
an Any may cross threads), recursively. A list's count becomes -1
(shared, one holder), and it stays shared after the Any is gone
(decision 0488: a held-once shared list stays shared), so each write
calls rl_copy, which finds it held once and writes in place. The prelude
used `rt.any_get[List[T]](rt.any_of[List[Int]](l))` as a type cast in
words_filled (List.filled), words_concat (List.concat), and list_sort
(Int and Str sorts). After `xs.sort()` the caller's own list is shared;
after `List.concat([xs, ys])` so are xs, ys and the result.

Fix: test the element type on a value that costs nothing to share
(the fill value, or an empty literal, which is immortal), and build or
move the list as its own type. words_filled does this since 2026-11-19;
words_concat, list_sort (Str), list_sort_in_place (Int) and list_map's
same type path since the next commit, which hand the list across with
`rt.obj_at[List[U]](rt.obj_addr(l))` (counted once more, so the source
binding's release balances it). To check a list,
read its object's first word in a std test:
`rt.slot_load(rt.obj_addr(l), 0, 0)` is 1 for a list only its owner
holds (std/native/prelude_test.xo "List.filled: a bulk filled list is
unique, not shared"; "sort, concat and map leave the caller's lists
unique").
