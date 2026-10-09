# 0091: Map and Set iteration order: keep insertion order (Appendix B question 2)

Status: Proposed (2026-10-09, spec audit; awaiting the owner)

## Context

Spec Appendix B question 2 asks whether `Map` (and `Set`) iteration is
insertion ordered (current, core 2.1, std.md 2.4) or sorted, "given the
cost on the Go backend". Iteration order is observable (printing, JSON
objects, `keys()`, `for (k, v) in m`), so it is part of the compatibility
promise; changing it after 1.0 would change program output silently.

Today: the Go backend's `Map` (`std/rt/map.go`, decision recorded in
`bench/results/perf-map-2026-10-08.md`) is an append only entry log with an
open addressing index; insertion order falls out of the log, and an
overwrite keeps the key's position through an `org` field. The native
backends (`std/llrt`) are insertion ordered as well
(`internal/llvm/testdata/golden/maps`).

## Options

1. **Insertion order (current).** Deterministic, matches Python and
   JavaScript, JSON objects keep the order a program built them in.
2. **Sorted by key.** Deterministic and canonical (two maps with the same
   entries iterate alike), but keys must be `Ord`, not just `Eq + Hash`.
3. **Unspecified** (Go's randomized order). Fastest to implement on Go maps,
   but breaks reproducibility (record and replay, golden outputs,
   deterministic tests): rejected by design goal 7 regardless of cost.

## Measurement

`go test ./std/rt -run '^$' -bench 'Order' -benchmem -count=3`
(`std/rt/audit_bench_test.go`), darwin/arm64 (M4 Pro), Go 1.26.1,
10,000 `Str` keys inserted in a scrambled order, medians of 3:

| Operation | ns/op | B/op | allocs |
|---|---:|---:|---:|
| build: Xo `Map` put x 10k | 358,000 | 1,575,979 | 50 |
| build: Go `map` put x 10k | 372,000 | 873,276 | 79 |
| iterate: Xo `Map`, insertion order, no overwrites | 18,100 | 0 | 0 |
| iterate: Xo `Map`, insertion order, after 5,000 overwrites and 1,000 removals | 44,800 | 67,584 | 2 |
| iterate: Xo `Map`, sorted (collect keys, sort, look up) | 797,000 | 163,840 | 1 |
| iterate: Xo `Map`, sorted, churned | 769,000 | 215,040 | 3 |
| iterate: Go `map`, unordered | 48,200 | 0 | 0 |
| iterate: Go `map`, sorted keys | 971,000 | 666,032 | 21 |

The insertion order field costs no memory: a `Map[Str, Int]` log entry is
40 bytes with `org` and 40 without it (`TestOrderEntrySize`; the field
fills alignment padding; `Map[Int, Int]` is 32 either way).

Reading: insertion ordered iteration is 2.7 times faster than Go's own
unordered map iteration (a dense log walk against bucket probing), 1.1
times slower than it after heavy churn, and puts cost the same as Go's.
The extra memory against a Go map comes from the persistent log (value
semantics, decision of 2026-10-08), not from ordering. Sorted iteration
on the same structure is 44 times slower than insertion order; a sorted
structure (a persistent B-tree or red-black tree) would instead make
every `put` and `get` O(log n) comparisons with more allocation, and
would need `Ord` keys.

So the "cost on the Go backend" the question worried about does not
exist in the current runtime.

## Recommendation

Keep insertion order (option 1) for `Map` and `Set` and close the
question. Programs that need sorted output write `m.keys().sort()` or
`m.entries().sort_by(...)`, which states the intent.

## Consequences

- Appendix B question 2 is removed; std.md 2.4's "Insertion ordered. `put`
  on an existing key keeps its position." becomes part of the 1.0 promise.
- Every backend must keep the order (already true of `std/rt` and
  `std/llrt`); a future backend may not use an unordered hash map without
  an order side table.
- Removal then re-insertion moves a key to the end (current behavior,
  matches Python); this should be stated in std.md 2.4 when accepted.

## Alternatives

- Sorted: canonical but 44x slower to iterate on the current structure or
  O(log n) per operation in a tree, and restricts keys to `Ord`.
- Unspecified: breaks reproducibility.
- A separate `SortedMap[K: Ord, V]` in std later is compatible with this
  decision and can be added without breaking anything.
