# Nested element updates in place (decision 0169), 2026-10-11

Apple M4 Pro, macOS, about ten agents working on the machine (the runs
went through the machine-wide heavy lock, but other light work ran):
wall clock is noisy, allocation counts are exact. `before` is the
compiler at 2cdbb29 (main before the decision), `after` the branch with
`list.take` (b1fe391).

## Micro benchmark

`bench/perf/nested` (`nested <workload>`, N = 20,000 updates each),
built with `--backend=go|arm64|llvm`, three runs per cell; times are the
median of the program's own clock in microseconds; allocations are
`XO_LEAKCHECK=1`'s total (no allocation live at exit in any run). The
checksums are the same on every backend before and after.

| workload | backend | before (us) | after (us) | allocations before | after |
|---|---|---|---|---|---|
| push: `g.blocks[g.cur].insts.push(x)` (var receiver) | arm64 | 63,580 | 104 | 39,993 | 18 |
| | llvm | 69,770 | 84 | 39,993 | 18 |
| | go | 173 | 104 | | |
| setnested: `m[0][j] = v` | arm64 | 53,237 | 68 | 20,019 | 19 |
| | llvm | 48,818 | 59 | 20,019 | 19 |
| | go | 180 | 85 | | |
| setdeep: `rows[0].cells[j].g = v` | arm64 | 1,477,240 | 292 | 20,021 | 21 |
| | llvm | 1,277,496 | 217 | 20,021 | 21 |
| | go | 354 | 169 | | |
| mapput: `rows[0].m[k] = k` | arm64 | 190,224 | 803 | 140,194 | 411 |
| | llvm | 181,952 | 706 | 140,194 | 411 |
| | go | 802 | 806 | | |
| strappend: `rows[0].text += "x"` | arm64 | 2,455 | 172 | 20,009 | 20 |
| | llvm | 2,184 | 151 | 20,009 | 20 |
| | go | 17,138 | 10,177 | | |
| method: `cs[0].add(i)` (var receiver method on an element) | arm64 | 59,436 | 65 | 39,993 | 17 |
| | llvm | 67,519 | 61 | 39,993 | 17 |
| | go | 237 | 228 | | |
| shared: 2,000 pushes, a copy of the outer list kept at each | arm64 | 940 | 369 | 5,998 | 5,998 |
| | llvm | 921 | 375 | 5,998 | 5,998 |
| | go | 281 | 107 | | |

The go backend's code is unchanged; its column shows the noise. The
shared workload still copies at every update (5,998 allocations before
and after: value semantics), and each copy kept its value (checksum).
After the change the native backends are at or below the go backend on
every workload but setdeep (about 1.3x to 1.7x of go's 169 us).

The golden program `internal/llvm/testdata/golden/nested_update` makes
159 allocations on each native backend after, 2,162 before
(TestNestedUpdateInPlace requires at most 400).

## By value receivers on the native backends

A related trap on the go backend (a by value method on a state struct
makes its next list writes copy, std/rt shared marks) does not happen
on the native backends: borrow inference passes a by value receiver
without a count. A scratch program with a state struct of two lists
(`struct Gen { names: List[Str], lazy: List[Int] }`), a by value
`fn (g: Gen) peek(i)` and a `var` `fn (var g: Gen) put(i)` writing
`g.lazy[i] = i` and pushing onto `g.names`, alternated 20,000 times
after 20,000 pushes: 28 allocations on arm64 and on LLVM, with and
without decision 0169.

## The switch-over driver without the hand workaround

See the next section (filled by the experiment run).
