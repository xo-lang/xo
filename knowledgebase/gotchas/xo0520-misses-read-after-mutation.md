# XO0520 misses a lost update when the stale copy is read afterwards

Status: Observed (2026-10-07, probe on bench/xo/v0.4/01-crud-api). Resolved by decision 0037; values derived with `with` remain untracked by design.

Deleting `st.users.put(id, u)` from `Store.update` produces no diagnostic:
`u` is copied out with `.or_err(...)?`, mutated, then read (validated and
returned), so "mutated and never read again" does not hold. The PATCH returns
the new values while the store keeps the old ones. This is the same bug as
planted Go variant 01 X6, which the planted bug estimate wrongly counted as
caught by Xo.

Candidate fix for spec 0.5: a `var` copied out of a collection and then
mutated must be stored back (or passed to a `var` parameter) on every path
that completes normally, regardless of later reads. Opt out explicitly by
copying through a local first when a scratch copy is intended.
