# 0612: A struct parameter whose consumed leaves are some of its fields stays borrowed

Status: Proposed (Xo Lang 1, 2026-11-11)

## Context

Borrow inference (decision 0046, rc.xo borrow_scan) makes a parameter
owned once any path consumes any of its leaves; decisions 0149 and 0489
keep enums and enum only structs borrowed unless every path consumes
them. A by-value method that returns one field of its receiver, such as
the arm64 backend's `fn (g: G) lbl_local() -> Str { ... g.lp_str }` or
`fn (g: G) memo_at(i) -> Memo { ... g.cm }`, therefore takes its
receiver owned, and a caller that keeps the receiver (any `var g`
method) counts up every heap field of it for the call while the callee
drops them all after. G has dozens of heap fields: on the aiplatform
test build these copies were most of `inc thunk_x9` (5.6% of CPU) and of
the List drops under `lbl_local` (1.5%) and `memo_at` (1%).

## Decision

To be filled in with the measurements: a struct or tuple parameter
(not enum like) whose consumed leaves have some, not all, of its heap
leaf paths, each a field's, stays borrowed; the paths that consume a
leaf count up that leaf where they consume it (the retain decisions
0149 and 0489 already insert).

## Consequences

To be filled in.

## Alternatives

To be filled in.
