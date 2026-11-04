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
