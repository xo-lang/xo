# Front end cache: a body that queues work the replay does not make

Status: Observed, fixed (2026-10-19, decision 0220 addendum stage 2
step 1).

Symptom: the second build of a program with contracts failed with "the
front end cache's entry for withdraw does not replay (XO_FE_CACHE=off
builds without it)" (TestGolden contract_ensures, contract_invariant,
fault_unwind, and the debug information tests on the same programs).
The first build, and every build of xo, passed.

Cause: a function with `ensures` clauses (or a var method of a type
with an invariant) is built as a wrapper that makes its inner function
`<name>.body` with Module.new_func and queues a Job.Inner for it. The
entry recorded the new function (as a helper) but not the queued job,
so the replay left the queue one job short and its count check failed.
The builder's state fingerprint did not cover the queue's contents, and
the earlier tests had no contracts.

Fix: fe_entry records nothing for a body whose job queued anything but
a declare's Body jobs (compiler/ssa fecache.xo).
TestFrontCacheInterfaceEdits builds a function with contracts twice.
When the builder gains another side effect of a body (a queue, a
table), check that fe_entry refuses or records it; the replay's count
check turns a miss into a loud failure, never a wrong binary.
