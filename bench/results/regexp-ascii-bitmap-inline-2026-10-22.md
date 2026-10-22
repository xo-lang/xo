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

## Second step: the Pike thread without a call

`add` copies the first two group positions without a loop and adds the thread
of an ALT's first branch inline when that branch is a rune instruction not yet
queued (the `\w+` loop's case). `xo bench --ab` against the first step:
find_all instructions 0.887 (0.886 to 0.888), replace 0.911 (0.911 to 0.911),
wall replace 0.947 (0.943 to 0.947); outputs the same. Against Go's
instructions: find_all 3,696 M (0.93x Go), replace 2,309 M (1.53x Go).

Tried and not kept: the same inline thread in the step loop (the next
instruction's thread copied straight from the running one, no `tc` copy):
replace 2,386 M against 2,309 M, worse.

## Third step: no empty width context without an assertion

Profile of replace (XO_PROF=cpu, 300 passes): add 51%, exec 30%, empty_context
and is_word_char 8%. `Prog.has_empty` (any I_EMPTY instruction) lets exec skip
`empty_context` at every position when the pattern has none, and the step's
thread copy takes its first two entries without a loop. `xo bench --ab`
instructions against the second step (7 rounds): find_all 0.968 (0.968 to
0.968), replace 0.965 (0.965 to 0.965), match 1.003 and captures 1.002 (code
layout; the backtracker is unchanged); outputs the same. Against Go's
instructions: find_all 3,577 M (0.90x Go), replace 2,227 M (1.47x Go).

Tried and not kept: the capture instruction's recursive `add` replaced by a
saved-position stack in the queue (restore at the call's end): find_all 1.079x,
replace 1.024x instructions, worse.

## Fourth step: Go's one-pass matcher (anchored row)

The three rows above use unanchored patterns, which Go never runs one-pass.
New row `anchored` in the bench program and its Go twin: `^(\w+)@(\w+)\.com`
and `^(\d{4})-(\d{2})-(\d{2})$` on 10-17 byte texts, `is_match` twice and
`captures` twice per round, 200,000 rounds (outputs equal, 1,600,000).
Before: Xo's backtracker ran these at 2,983 M instructions against Go's
2,496 M (1.20x). The port (Go's makeOnePass: the `onePassCopy` rewrites, rune
set merging, the unambiguity check, `AltMatch`; the matcher dispatches an
alternation by binary search over the merged ranges, and a rune instruction
tests the ASCII bitmap first) gives 2,616 M, 0.877x of before (0.877 to
0.877, 7 rounds), 1.045x Go; `\A` and `\z` are tested by position, with no
context, 2,593 M (0.991x of that), 1.04x Go. The other rows against the
previous step (`--ab`, instructions): match 1.001, captures 1.002, compile
1.002, find_all and replace 1.000 (one more list in `Prog`).

A first version kept the tables as six fields of `Prog` and cost compile
+9.5% (a larger `Prog` copied by value); one flat list `Prog.one` brought it
to +0.2%.

Not the same as Go: Go's parser factors a common prefix out of an
alternation (`a|aa` becomes `a(?:|a)`), so three of onepass_test.go's
one-pass patterns (`^(?:a|(?:aa))$` and two like it) are not one-pass in
Xo, where the alternation is not factored; results are the same, they take
the backtracker. Porting the parser's `factor` is the next candidate (it also
shrinks every program with a common prefix, so the Pike rows move too).
