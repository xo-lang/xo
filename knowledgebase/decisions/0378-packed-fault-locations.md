# 0378: A checked operation's position as one record address

Status: Proposed, deferred (written by DS-2, 2026-10-26; the lead
deferred it the same day: the file name half already landed in 0228's
addendum, and what remains moves about 8 bytes a site from `__text` to
read-only data with no instruction change, so other work goes first)

## Context

Decision 0228's 2026-10-25 addendum (c8f5f7c66) stores a fault position
as a per-part zero terminated file name plus the line and the column,
which the stub passes as three words: one `adrp`+`add` for the file and
one `movz` for each number. That is 11 to 13 bytes of `__text` for every
checked operation (CRUD: 2,917 sites, `__text` 1,412,032 to 1,444,720 B,
`__cstring` 88,910 to 5,485 B). The file name is now shared, but the two
immediates are not: two sites on the same line repeat them, and text is
the section that costs an executable page.

## Decision

1. A position is one 8-byte read-only record per unique position per
   part: `.p2align 2`, then `.long FILE - .` (a 32-bit self-relative
   pointer to the file name, FX_DIFF32, so it resolves at assembly time
   and needs no relocation), `.short line`, `.short col`. The records and
   the file name strings are emitted in the same section, because
   FX_DIFF32 resolves only between labels of one section.
2. The stub passes the record's address as its only position argument:
   `adrp`+`add` in place of `adrp`+`add`+`movz`+`movz`. Two instructions
   a site leave `__text` and 8 bytes a site enter read-only data.
3. The record reaches the runtime through new exports,
   `fault_raise_*_rec`, beside 0228's three-argument ones. The three
   argument exports stay until every backend passes a record, so the
   change lands one backend per commit: arm64, then amd64, then LLVM.
   `fault(msg)`, contracts, and `fault_raise_pos` (C callers, qsort's
   comparator in the fault test) keep the location they have today.
4. The decode happens only where the fault is recorded, in `fault.xo`
   `set_pos`, so the normal path is untouched: the file name is the
   record's address plus its signed 32-bit delta, the line and column are
   the two shorts. Line 0 keeps 0228's meaning ("the file name is the
   whole location"), which is also the overflow rule: a position whose
   line or column does not fit a `uint16` is emitted as a record pointing
   at its "file:line:col" text with line 0. One shape, no second ABI, and
   no position can be wrong.
5. The fault message text stays byte identical: the same "file:line:col"
   bytes `pos_text` builds today. DWARF line info is untouched, since the
   records are data and the debug line table is generated from the same
   SSA positions as before.

## Consequences

- `__text` loses about 8 bytes a checked operation (CRUD: about 23 KB of
  1,444,720) and read-only data gains the same, so file size moves little
  while the section that is executed shrinks; the fault path gains two
  cold loads (a 4-byte and a 2-byte load) that no non-faulting program
  pays.
- The function and part cache versions bump: the text shape changed.
- The three-argument exports are deleted once LLVM is the last backend
  moved, as 0228 deleted the C entry points.
- To take it up: measure first whether the 23 KB of `__text` changes
  anything a program does (instruction cache misses on CRUD under
  `xo bench --ab`); a size-only win does not justify three backends.

## Alternatives

- A 12-byte record, a full 8-byte pointer plus an 8-byte packed word:
  rejected, no benefit over a 32-bit self-relative delta.
- A 4-byte record packing a file index, the line, and the column:
  rejected, 8 bits of file index is below the 83 files CRUD has and 8
  bits of column below ordinary generated table lines, so the overflow
  path would fire on real code.
- One `movz` carrying line and column together: no, a 16-bit immediate
  is already the width of the line alone.
