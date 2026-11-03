# Writing a field's own value into a shared list copies the list

Status: Observed (2026-11-03, Xo Lang 2)

## Symptom

A heap profile of an ELF link from std's half at link.resolve charged 5.75 MB to
ELinker.gc_sections (elfstrip.xo), a loop that only sets a flag.

## Cause

`l.objs = h.objs + prog.objs` shares each object's section list with the half. The
loop wrote `l.objs[o].sects[k].dead = true` for every store_dead section, and
every one of those was dead already. A write through a shared list copies it
first (core 5.2), whatever the value, so each of std's 40 objects had its section
list (51,628 records) copied to store values that were already there.

## Fix

Write only when the value changes (`if s.store_dead && !s.dead`). In any loop
over a collection another holder may share, test before a write that is often a
no-op. A heap profile at the phase (XO_PROF=heap, XO_PROF_HEAP_AT) shows these
copies at the writing line.
