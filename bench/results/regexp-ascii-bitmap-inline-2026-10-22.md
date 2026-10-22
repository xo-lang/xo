# std/regexp: ASCII bitmap and inlined hot calls, 2026-10-22

Program: bench `rxb` (the 2026-10-22 first byte filter's: three patterns on a
127 KB text; modes match, find_all, replace, captures), arm64, native. Go: the
same patterns with Go's regexp. Instructions retired, then
`xo bench --ab --rounds=7` (before against after, the same inputs; the machine
was loaded, so wall time is noise and the instruction counts decide).

| mode | Xo before | Xo after | Go | after / Go | before / Go |
|---|---|---|---|---|---|
| match | 5,663 M | 3,210 M | 3,034 M | 1.06 | 1.87 |
| find_all | 5,170 M | 4,172 M | 3,966 M | 1.05 | 1.30 |
| replace | 2,986 M | 2,535 M | 1,513 M | 1.68 | 1.97 |
| captures | 252 M | 175 M | 207 M | 0.84 | 1.22 |

`--ab` instructions (B / A, 95% interval): match 0.567 (0.566 to 0.567),
find_all 0.807 (0.806 to 0.807), replace 0.849 (0.849 to 0.850), captures
0.694 (0.692 to 0.696). cpu_ms: match 0.571, find_all 0.905, replace 0.833;
max_rss within noise. Output hashes are the same before and after in every
mode.

Changes (std/native/regexp.xo):

- the backtracker pushed a job through `BT.push` (a call, a `Prog.op` load and
  a visited check) for an ALT or a capture restore, which need none: they
  append the job directly; the visited check of each step is inline, not a
  `should_visit` call (match, one step at a time: 5,663 M to 5,061 M with
  the bitmap, the prefix and the unchecked reads, 4,342 M with the direct
  jobs, 3,209 M with the inline check);
- `Prog.ascii`: a bitmap of the ASCII bytes each rune instruction accepts
  (computed once in compile); an ASCII byte takes a bit test in the
  backtracker and the Pike step, not a class search;
- `Prog.prefix`: the literal prefix computed once in compile (exec rebuilt it
  with a BytesBuilder on every call);
- the Pike queue's `contains` inline in `add` (replace 2,880 M to 2,535 M,
  find_all 4,897 M to 4,173 M), `step_at` and the backtracker's byte reads
  unchecked after their length check, the backtracker's job list preallocated.

Tried and not kept: hoisting `g.op`, `g.out`, `g.arg` into locals in the
backtracker (+0.6% instructions).

Still above Go: replace 1.68x (Pike `add` is 60% of its profile: a call per
thread, the capture copy per thread). Go's one-pass matcher applies only to
patterns anchored with `^`, which none of these three is, so it would not move
these rows; it needs its own anchored row.
