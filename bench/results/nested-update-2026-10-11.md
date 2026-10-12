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

Run 2026-10-12 under the heavy lock. Three builds of the switch-over
driver selfhost/xo with `--backend=arm64` (each about 140 s):

- A-before: the branch's selfhost sources built by the compiler before
  decision 0169 (2cdbb29);
- A-new: the same sources built by the compiler with `list.take`;
- A-rev: the same sources with selfhost/arm64's take_insts and
  put_insts rewritten as direct nested updates
  (`g.blocks[g.cur].insts.push(x)`, `g.blocks[b].insts[i].early = true`;
  11 sites in isel.xo, opt.xo, closures.xo, jumptab.xo, a scratch copy,
  not committed), built with `list.take`.

Each compiled selfhost/xo to assembly (`xo1 asm selfhost/xo`, no link;
`/usr/bin/time -l`, one run each):

| compiler | wall clock | instructions retired | peak RSS |
|---|---|---|---|
| A-before | 271.8 s | 2,584.5 billion | 5.78 GB |
| A-new | 276.4 s | 2,586.5 billion | 5.25 GB |
| A-rev | 273.1 s | 2,555.3 billion | 5.16 GB |

The three outputs are byte identical (8,707,747 lines), and so are
their outputs for bench/perf/xo (about 0.06 s each; 686.5, 674.2, and
673.3 million instructions for A-before, A-new, A-rev, the second and
third of three runs).

Result: without the hand workaround the compiler is as fast as with it
(1.2% fewer instructions, wall clock within noise): the workaround is
no longer needed. Before decision 0169 the same rewrite ran over 20
minutes on this input (the gotcha). The decision does not make the
compiler that keeps its workarounds faster (A-new and A-before within
0.1% in instructions): the remaining factor of about 2 between the
self compiled compiler and the Go built one (selfhost-arm64-2026-10-11)
does not come from nested element updates. Next experiment: profile
A-new on the same input (`sample <pid> 30` during `xo1 asm
selfhost/xo`) and rank the Xo functions by self time against the Go
emitter's pprof profile of `xo asmdump selfhost/xo`.
