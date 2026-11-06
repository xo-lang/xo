# A closure call anywhere below a Mutex.with body makes every write copy

Status: Observed 2026-11-05 (std/sql's memory driver, decision 0528)

Symptom: a `Mutex.with` body that changes a large value in place (a
`Map[Str, Table]` whose table rows it pushes to) still copied the whole
value on every call: 100,000 INSERTs into `sql.memory` took 31.7 s of CPU
on the Mac, quadratic in the table's size, and a profile showed
`obj.rl_copy` of the rows list under the write and `dropp` of the old
table after it. The same code on a local map, or under a Mutex in a small
repro, ran in place. Taking the table out of the map (`remove`, then
`put`) or through `Map.update` changed nothing.

Cause: the body can be canceled, so it keeps the value it was given
(core 5.4: a cancellation hands that value back) and its first write to
a list or map copies it. A function counts as cancelable when it has a
cancellation check with no known callee or calls one that has (compiler/ssa
passes.xo prune_unwind), and a call through a function value is such a
check: `items.map(fn(x) { coerce(ec, x) })` in `coerce`'s array branch and
`t.rows.filter(fn(row) { ... })` in `compact`, never run by the benchmark,
made every INSERT, UPDATE and DELETE path cancelable. The bisect that
found them: a never-run call in a block before the write was enough.

Fix: in code reached from a `Mutex.with` body that writes a large value,
use a loop instead of a prelude method with a closure (`map`, `filter`,
`any`, `fold`, ...) and avoid calling function values; sql.xo's `coerce`
and `compact` are loops now. To find the call, wrap each callee of the
body in a never-run branch before the write and time the loop (std
`_test.xo` files reach private functions; the first test of each run
also pays the test build, about 1 s). Since 2026-11-06 (Xo Lang 1, compiler/ssa
passes.xo walk_literal) a prelude walk given a closure literal (List
map, filter, filter_map, find, any, all, fold, sort_by, group_by; Map
and Set map, filter, any, all) counts as cancelable only when the
literal's own body is, so such a body writes in place
(test/codegen/withbody). A walk given a function value, or a call of
one, still counts; an `xo vet --cost` note naming the call that made a
`with` body cancelable would cover those.
