# Str.trim_chars against Go's strings.Trim, 2026-10-23

Program: bench/perf/trimchars/main.xo (200,000 iterations: one trim of a
text from a six-text table with a cutset from a seven-cutset table, so the
ASCII bitmap, one byte, Unicode and empty cutset paths all run). Native
arm64 on the development Mac. Go twin: bench/perf/trimchars/twin.go.txt
(strings.Trim, TrimLeft, TrimRight, go1.26.1); its output is equal line
for line: trim 5814291, trim_start 5933341, trim_end 5919055, index
6538106.

`xo bench --ab --rounds=9 --keep-loaded`, load 8.5 from other sessions (so
wall is load bound; instructions are exact). B / A is Xo / Go.

| workload | Xo instructions | Go instructions | Xo / Go | wall (95%) |
|---|---|---|---|---|
| index (loop only) | 65.1 M | 22.6 M | 2.878 | 1.469 (1.405 to 1.823) |
| trim | 108.6 M | 40.4 M | 2.686 | 1.972 (1.892 to 2.044) |
| trim_start | 90.9 M | 36.4 M | 2.497 | 1.791 (1.708 to 1.901) |
| trim_end | 90.7 M | 35.9 M | 2.525 | 1.776 (1.719 to 1.894) |

Net of the loop (instructions per call: the row minus index, divided by
200,000): trim 217 against Go's 89 (2.44), trim_start 129 against 69
(1.87), trim_end 128 against 66 (1.93). Peak memory is 0.67 of Go's.

The loop, not the trim, is the larger gap: two lookups in `const
List[Str]` tables, a modulo, a slice index and two `len` calls cost Xo
325 instructions an iteration against Go's 113. A first version also
picked the workload with `w == "trim"` inside the loop; its `noop` row was
435 Xo instructions against Go's 148 for a loop that does one Str compare
and a call, so the dispatch was moved out of the loop and is not a trim
figure.

The CPU profile of trim (XO_PROF=cpu, 20,000,000 iterations, 169 samples
in 515 ms, so relative) splits it three ways:

- The const table lookups: cabi.const_once 45 ms cumulative (27%),
  consts.once 31, consts.table 19. Every element access goes through the
  lazy-initialization guard.
- Bounds checks: prelude.llrt_str_slice_chk 27 ms cumulative (16%) plus
  prelude.slice_check 10 (6%), 22% together, for the views the trim makes
  and the loop's own table index.
- trim_chars itself, 71 ms (42%): trim_end_byte 19, trim_start_byte 9,
  find_short and find_byte 15, trim_end_ascii 6, ascii_set 5, raw.decode 2.
  Dropping the views and the const tables at exit: drop List[List[Str]] 16
  ms, drop Str 14.

So the trim itself is 1.87 to 2.44 times Go per call and the remaining gap
is per-call overhead that a Go `trim` does not pay, not a missing fast
path: the implementation already has Go's split (empty, one byte ASCII,
ASCII bitmap in two Int words, Unicode), so no fast path of Go's
makeCutsetFunc, trimLeftByte, trimRightByte, trimLeftASCII,
trimRightASCII, trimLeftUnicode or trimRightUnicode is unported.

Next experiments, ranked, each with the row that proves it:

1. The const-access guard (27%): `xo dev codegen --backend=arm64` on
   `index_all` in bench/perf/trimchars/main.xo; if each access really
   loads through cabi.const_once and consts.table, a once-initialized
   const should lower to a plain load (compiler/consts or cabi). Proof:
   the `index` row of this benchmark.
2. The bounds checks (22%): a slice of a Str whose range was just checked
   by the same expression need not check again (compiler/ssa). Proof: the
   trim rows, whose net cost is mostly slicing.
3. The trim's own 42%: ascii_set is rebuilt on every call (5 ms) though
   the cutset is a constant in most callers; a first-byte compare before
   find_short (Go's IndexByte does this in one pass) is the next step
   inside scan.xo. Proof: the trim row alone, `--ab` before and after.
