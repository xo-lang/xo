# A lazily parsed body with a syntax error, and the caches

Status: Observed (2026-11-05, Xo Lang 1, decision 0517's default switch)

Symptom: with XO_DEP_LAZY=1, a dependency function the program reaches
whose body has a syntax error faulted the compiler ("a lazily parsed body
of ... does not parse", check.force_body). A first fix reported the syntax
error on a cold build, but every warm build after it reported
`XO0401: expected Int, found ()` at the body's closing brace instead.

Cause: two caches recorded the module or the body as if it had checked
clean. The module check cache's entries are written from the driver's
checker (`c` in compiler/xo/bcache.xo), while the syntax error was found
by the SSA builder's own copy of the checker (`b.c`), so the entry was
written; a warm build replayed it (cache chk 2/2), the module's bodies came
from its record, and its parse errors were dropped with the skipped
module's diagnostics. Standing an empty block in for the body also made
the empty block what later readers saw.

Fix: the body's declaration stays lazy and its errors are kept in
`Checker.lazy_bad` (check/stmt.xo force_body); the builder does not build
it (ssa/contract.xo body_of) and records no front end entry for it; and a
build whose `b.c.lazy_bad` is not empty writes no module check entries.
cmd/xo TestDepLazySyntaxError builds such a program cold and twice warm
and wants the same file:line diagnostic each time. When a checker fact
decides what a cache stores, check which checker (`c` or `b.c`) holds it.
