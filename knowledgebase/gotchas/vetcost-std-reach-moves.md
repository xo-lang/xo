# TestVetCostStd's findings move with what bench/perf reaches

Status: Observed (2026-11-03, Xo Lang 1's bisect for the Lead)

## Symptom

cmd/xo TestVetCostStd fails on main with new XO0913 notes in lines that
have not changed in weeks (std/native/crypto.xo's PEM decoder, written
2026-10-20), and with "no note matches ... any more; remove it" for an
allow line whose source line is also unchanged (archive.xo's pax record
check). It looks like a compiler change judging old code differently.

## Cause

The test vets std only through the programs under bench/perf (cost notes
need a lowered program). A new bench (bench/perf/jwt, cacd5306a) reached
the PEM decoder for the first time, so its notes appeared; a change to
std/archive (067a24a4c) left no bench reaching the pax parser, so its
allowed note disappeared. No codegen changed.

## Fix

Bisect with `git bisect run` over the test (it names the commit that
changed the reach), then fix the newly reached code or allow it with a
measured reason, and drop an allow line whose code no bench reaches.
A commit that adds a bench/perf program or changes which std paths one
reaches runs `go test ./cmd/xo -run TestVetCostStd` in its gate.

## Addendum 2026-11-15: per module, and what an allow line can cover

Observed (Xo Lang 2, the stdfast vet check). `xo vet --cost` lowers only
the functions its program reaches: a std function nothing calls has no
notes, even with `XO_CHECK_EAGER=1`. Before this change `xo vet --cost
--std-module=M` ignored the module and vetted the current directory as
a program. It now checks M as `xo test --std-module` does with every
function of M a root (check.Checker.lib_roots, decision 0523), so it
sees code no bench reaches: its first sweep of the 77 modules off the
closure found three XO0913 notes in std/amqp.xo's parse_addr (fixed).
Such a note cannot go in vetcost-std-allow.txt: TestVetCostStd fails an
allow line no bench/perf note matches. Fix the code instead, or add a
bench that reaches it first.
