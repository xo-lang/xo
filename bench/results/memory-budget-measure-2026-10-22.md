# Decision 0349's memory measure against RSS (2026-10-22, Xo Lang 2)

The heap's use as decision 0349 counts it (chunks and records as mapped,
large blocks as handed out; before this, every mapping's size), printed by
`xo build --phase-times`, against `/usr/bin/time -l`'s maximum resident
set size. Apple silicon Mac, 24 GB, macOS 27.0.1.

## A cold self build of xo (`xo build compiler/xo`, every cache off)

| phase (end) | heap MB | peak MB |
|---|---|---|
| check | 2,199 | 2,203 |
| ssa | 3,647 | 3,648 |
| cleanup | 4,503 | 4,571 |
| insert_rc | 5,377 | 5,426 |
| inline | 6,572 | 6,573 |
| inline.verify | 6,680 | 6,722 |
| emit | 8,948 | 9,909 |
| link | 8,284 | 9,909 |

Maximum resident set: 9,584,967,680 bytes (9,141 MiB). The counted peak
is 8% above it. Counting mapping sizes, the same build passed 12,288 MB
before phase emit (the stop that 75296384b's hotfix removed).

A default for xo needs the LLVM build of xo (the full tier) measured
too; the owner approves the number.
