# 0179: A WebAssembly backend written only in Xo, from the self hosted SSA to WAT assembled in Xo, with a runtime of its own over WASI

Status: Accepted (2026-10-12), stages 1 and 2 and stage 3 (size, frames, structured control flow, rt.fs_write) implemented in `selfhost/wasm/`
(gen.xo, func.xo, call.xo, home.xo, coalesce.xo, layout.xo, rt.xo, rt2.xo, rt3.xo, rt4.xo, asm.xo, main.xo, run.mjs), the driver
path `xo1 build --backend=wasm` (`selfhost/xo/wasm.xo`), and
`internal/selfhost/wasm_test.go` (`TestSelfhostWasm`). Figures:
`bench/results/wasm-2026-10-12.md`. Item (5) of decision 0150's amended
order; serves the browser playground of decision 0144.

## Context

Decision 0144's playground runs `cmd/xoplay` (the Go implementation
built for wasip1) and then the Go compiler and linker, also built for
wasip1, in the visitor's browser: about 23 MB of toolchain, and a build
and run of a tour program in about 2 s. Decision 0150 (amended
2026-10-11) asks for a WebAssembly backend written only in Xo (no Go
twin, so no double maintenance), so the playground can eventually run
the self hosted compiler itself as wasm and emit wasm directly, with no
Go toolchain in the page. Its first experiment: the core subset lowered
to WAT, assembled with `wat2wasm`, run under wazero, output diffed
against the goldens. It also leaves the port of std/llrt's xo_rt.c to be
decided with this backend.

What the machine has (2026-10-12): Node 22.22.1 (WebAssembly with
multi-value, bulk memory, and `node:wasi` preview1), wazero 1.8.2 and
1.12.0 in the Go module cache (not a dependency of this repository), no
`wat2wasm`, `wasmtime`, or `wasmer`, and Apple clang without the
WebAssembly target (no `--target=wasm32-wasi`).

The self hosted pipeline already ends in an SSA module after the passes
and InsertRC (selfhost/ssa, decision 0150), which selfhost/arm64
(decision 0158) and selfhost/llvm (decision 0166) consume; the LLVM
emitter (internal/llvm fromssa.go) is the closest model: values are
SSA values, memory is reference counted, faults unwind through landing
pads (decision 0085).

## Decision

**Where.** `selfhost/wasm`, an Xo package (`wasm.generate(m)`: the WAT
text and the binary module, or why the program is not supported) and a
command line (`xo run selfhost/wasm -- [--std=DIR] [--wd=DIR] [--each]
[--out=DIR] <dirs...>`; `--asm=IN.wat -o OUT.wasm` assembles a file).
`xo1 build --backend=wasm [-o OUT.wasm] <dir>` (selfhost/xo) writes the
module and its `.wat` beside it. There is no Go implementation of the
backend and none is planned.

**WAT text first, assembled in Xo.** The emitter writes WAT (the flat
form of the standard syntax, so `wat2wasm` takes it too when installed)
and `asm.xo` (about 900 lines) assembles it to the binary format: no
external tool in the build, the text is there to read and diff, and the
playground needs nothing but the compiler. The assembler takes the
subset the backend writes (no folded expressions; Float constants as
`i64.const` bits and `f64.reinterpret_i64`), and writes a `name` section
so traps name the function. Emitting the binary directly was rejected
for stage 1: the text is what is debugged, and assembling costs little
next to checking.

**Values.** Every SSA value is a sequence of wasm values, its leaves,
held in locals, passed as parameters, and returned as multiple results
(multi-value): Int and every sized integer i64, Float f64, Bool i32,
unit and capabilities nothing, Str and Bytes an object pointer and a
length (two i32), List and Box one i32, a function value its table
index and environment (two i32), structs and tuples the leaves of their
fields, enums an i64 tag and as many i64 words as the largest variant
has leaves. Aggregates are never in memory except as list elements, box
contents, and captures, where every leaf takes an 8 byte slot. This is
the LLVM emitter's model (values are SSA values, retain and release go
to the heap leaves) without its byte layout.

**Control flow.** One wasm function per SSA function; its blocks are a
dispatch loop: a `loop` around one `block` per SSA block, entered by a
`br_table` on the block number, a block's code after the end of its
wasm block, so a jump to the next block falls through and any other
sets the number and branches to the loop. Block parameters are locals
the edges assign (all arguments pushed, then stored: a parallel copy).
Simple and correct for any graph; a structured lowering (a stackifier)
is a later stage.

**Memory and allocator.** One linear memory, no threads at first (the
playground runs a module per worker). A size class allocator in the
runtime: power of two blocks of at least 16 bytes with an 8 byte header,
a free list per class, the heap grown by `memory.grow` (doubling). Live
and total allocation counts are exported (`xo_live`, `xo_total`) so the
runner prints the native runtime's leak check line under XO_LEAKCHECK.

**Reference counting as on the native backends.** The SSA after
InsertRC is lowered as the LLVM emitter lowers it: `retain` and
`release` call generated helpers per type (`$dup<T>`, `$drop<T>`: a
count increment, or a decrement that frees at zero after releasing the
children), `reuse` copies a list whose count is not 1 (`$lcopy<T>`),
equality is a generated helper per type, closures carry an environment
with its drop function in the table. Counts are i32; the i32 minimum is
immortal (literals, the empty list, list.take's holes, decision 0169);
with no threads there are no shared counts, so `share` does nothing.

**Faults without exception handling.** A fault records its message and
position and sets a flag; the code after every call that can fault
tests the flag and goes to the value's landing block (the pads of
decision 0085: the first successor of the check's block) or returns to
its caller; `rt.landing` is false and `rt.fault_continue` does nothing,
so the cleanup paths the SSA built run unchanged. When the fault leaves
main, `_start` prints `fault: <message> at <position>` to standard error
and exits with status 70, the messages those of std/llrt. The wasm
exception handling proposal was not used: Node supports it, but the flag
works in every runtime, and the SSA already has the checks.

**The runtime: written for this backend, in WAT inside Xo** (`rt.xo`,
about 5,100 lines), assembled with every program; not a port of
xo_rt.c (its messages and algorithms are std/llrt's, function by
function). Decision 0150
asks to decide the port of xo_rt.c with this backend: **xo_rt.c is not
ported to wasm.** Compiling it with `clang --target=wasm32-wasi` needs a
WebAssembly C toolchain and wasi-libc that the self hosted compiler
would then depend on (the playground would ship a C runtime archive
again), and xo_rt.c is built around what WebAssembly lacks (threads and
fibers, unwinding through the platform unwinder, signals, sockets). The
runtime grows with the stages, in WAT text for the primitives and in Xo
compiled by this backend for what can be written in Xo once the backend
covers it (formatting Floats, Unicode, the std implementations), which
is also the path of decision 0150's item (6), the runtime pilot.

**WASI.** The module's only imports are `wasi_snapshot_preview1`
`fd_write`, `proc_exit`, `args_sizes_get`, `args_get`,
`environ_sizes_get`, and `environ_get` (stage 1), the playground's sandbox rule
(decision 0144: only WASI preview1 imports, checked before
instantiating). `selfhost/wasm/run.mjs` runs a module under Node's WASI
in a worker with a 1 GiB stack (the goldens recurse a million deep),
as the playground's worker will, with the single threaded garbage
collector (gotchas/node-wasm-concurrent-gc-crash.md).

**Unsupported means no module.** A construct the stage does not cover
(a runtime call not ported, handles, `strconv` quoting, case mapping,
tasks, channels, scopes, network, files, time) makes the build fail
with `the wasm backend does not support ...`; no module is written, so a
program is never run with a construct lowered wrongly.

**Stage 1** (this decision, 2026-10-12): integers with their checks and
fault messages, the sized integers and Rune (std/llrt's xo_int_op,
xo_int_conv, xo_rune_of), Float arithmetic, order, `%` (an exact fmod),
and text (Go's shortest `%v`, by the free-format algorithm of Burger and
Dybvig on 1,280 bit integers), Float32 rounding, Bool, control flow,
calls, `var` parameters (multiple results), structs, tuples, enums,
match, boxes and recursive types (freed through a worklist, as
xo_rc_defer), closures and method values, interfaces through the SSA's
generated helpers, Str concatenation, interpolation, equality and order,
slices, find, split, lines, trim, repeat, join, contains, quote and
Debug text, Duration text, List (new, len, index, get, set, push, pop,
take, reuse, equality), Map and Set through the SSA's generated map
helpers and xo_str_hash, `fault`, contracts, `defer`, the fault records
of `task.isolate` style catching (fault_take), `env.args` and `env.get`
(WASI args and environ), Stdio output, and the leak check. Figures:
71 of the 133 LLVM goldens print the native output and free every
allocation; 62 fail the build as unsupported; none differs.

**Next stages**, in the order of the goldens they unlock: (2) the std
runtime the remaining single threaded goldens call (strconv quoting and
case mapping need Unicode tables: generated into Xo from Go's unicode
package and compiled by this backend rather than written in WAT; time
and the clock through WASI clock_time_get; random through random_get;
base64, JSON text); (3) a structured control flow lowering instead of
the dispatch loop, and output buffering (figures in the results file);
(4) the self hosted compiler built by this backend (selfhost/xo as
wasm), which needs the file system through WASI (`path_open`,
`fd_read`), then the playground switch (decision 0144): the compiler as
wasm emitting wasm, no Go toolchain in the page; (5) tasks, channels,
and scopes on one thread (fibers do not exist in wasm: a cooperative
scheduler over a CPS lowering, or the stack switching proposal);
network and std/http stay unsupported in the browser.

## Stage 2 (2026-10-12)

**Unicode and strings: Xo, not WAT.** Since decision 0180 the Unicode
operations (case mapping, quoting, the Rune classes, the lower compare)
and their tables are std/native/llrt/unicode.xo, compiled with the
program by the ordinary SSA builder; this backend compiles them like
any Xo code, so no Unicode table is written in WAT. The backend adds
only the byte level primitives that code and the native prelude call,
with std/llrt's semantics (`selfhost/wasm/rt2.xo`): `str_byte`,
`bytes_span`, `rs_append_bytes`, `rs_append`, `rune_is_space`,
`utf8_valid`, `bytes_decode`, `bytes_rune_count`, `int_parse_ok` and
`int_parse`, `str_of_byte`, `str_replace`, base64 (`b64_encode`,
`b64_ok`, `b64_decode`), `duration_parse_ok`/`duration_parse`, hex decoding
(`bytes_hex_ok`, `bytes_from_hex`), and the text of a Time
(`rs_append_time`).
Figures: `bench/results/wasm-2026-10-12.md`, "Stage 2".

**Tasks without threads (the old stage 5, moved here).** Scopes,
spawn, await, channels, select, Mutex, Atomic, cancellation, shields,
timers, and budgets run on one thread under a cooperative scheduler in
the runtime (`selfhost/wasm/rt3.xo`, assembled only into programs that
use it), with xo_rt.c's semantics function by function and without the
worker pool:

- *Suspension by stack unwinding and rewinding* (Asyncify's technique,
  generated by the backend; no stack switching proposal, no JS
  Promise Integration, so any WASI preview1 runtime runs it). A
  function that can reach a wait (a blocking runtime call, a call
  through a function value, or a call of such a function; a fixpoint
  over the call graph) gets a resume point before each such call: a
  dispatch label of its own in the function's `br_table`. When the
  callee suspends (`$sstate` 1), the function stores its locals and the
  resume point in the task's save area and returns; the scheduler later
  calls the task's entry function again with `$sstate` 2, and each
  function reloads its frame and jumps back into the call, down to the
  waiting runtime function, which tests its condition again. Functions
  that cannot wait are unchanged, and a program without tasks gets no
  scheduler at all.
- *The scheduler* runs main as the first task, then every task in spawn
  order, each until it waits or ends; a waiting task runs again when an
  event (a channel hand off, a task's end, a cancellation, an unlock)
  happened since it waited, or its deadline passed. Nothing runnable
  and no deadline: `fault: XO-F001: deadlock: every task is blocked`,
  status 70, as std/llrt. The output of a program is the same on every
  run (the Go backend's deterministic scheduler, core 7.4, without its
  seeded choices). Atomic operations and `task.is_canceled()` are
  scheduling points (every 64th yields), so a loop that polls the other
  tasks makes progress (gotchas/wasm-busy-loop-crashes-node.md).
- *Per task context*: the fault flags, the cancellation node, the
  shield depth, the unwinding flag, the held Mutexes (XO-F004), and the
  budget are globals swapped with the task. A fault that passes through
  frames is a flag of its own (`$prop`), cleared by a landing, so
  cleanup code runs normally (gotchas/wasm-landing-stops-fault.md).
- *Time*: WASI `clock_time_get` (monotonic for deadlines and budgets,
  realtime for `clock.now`); when every task waits for a deadline the
  scheduler waits in `poll_oneoff` on a clock subscription. Imports
  added: `clock_time_get` and `poll_oneoff` (task programs only), and
  `random_get` (`rand`, every module), all WASI preview1.
- *Not covered*: XO-F005 (a shielded defer longer than 5 s) and XO-F008
  (a budget passed twice in shielded code) are not checked;
  `task.stopping()`, `task.stop_requested()`, and the network stay
  unsupported (WASI preview1 has no sockets: the build fails with
  `the wasm backend does not support the network`).

**Toward the playground: the front end as wasm.** Reading files goes
through WASI preview1 (`selfhost/wasm/rt4.xo`: `path_open`, `fd_read`,
`path_filestat_get`, `fd_readdir`; a relative path resolves against
the first preopened directory, an absolute one against the second, as
run.mjs preopens the working directory and `/`; the playground will
preopen its virtual tree the same way), with `const_once`, Time, and
SHA-256. The self hosted lexer and parser built by this backend print
what their go backend builds print. Two build changes for large
programs: only the runtime functions a program reaches are assembled
(a call graph walk over the runtime text from the program's calls, the
table, and the exports), and the assembler's tokens are offsets into
the text (memory, not output, limited the first builds of
selfhost/check). `selfhost/wasm --why` lists every unsupported
construct; `--lax` makes an unsupported runtime call fault where it
runs, for experiments only (a shipped module never has one).

**Values share locals.** The value model stays (leaves in locals), but a
value no longer gets locals of its own: values whose live ranges do not
overlap share them (SSA liveness widened to one interval per value, a
linear scan per leaf signature), a field update of a struct that dies
there writes only the field, and a field read is its struct's locals at
an offset. Without it the self hosted checker (its `Checker` struct is
253 leaves) was a 1.67 GB module with functions of millions of locals
(V8 takes 50,000; gotchas/wasm-locals-per-value.md). What is left of
that cost is code: a `var` call still passes and returns every leaf of
a large struct. The next step for it is to pass large aggregates (or
`var` parameters) by reference in linear memory, as the LLVM emitter
passes `var` parameters by reference. With the sharing, the self hosted
checker builds into a 40 MB module that runs under Node and prints
what its go backend build prints (run.mjs compiles eagerly and turns
off incremental marking: two V8 crashes, see the gotchas); 40 MB and
about 5 GB to compile are what keep it out of the playground for now.

## Stage 3: the size of a large module (2026-10-12)

The self hosted checker's module was 43.2 MB (main on 2026-10-12) and
took 10.7 s and 5.9 GB under Node for seven small goldens. Measured by
construct (`selfhost/wasm --annotate` comments every SSA instruction's
code in the WAT with its opcode, so a script attributes the bytes), the
first quarter was suspension frames, then whole `Checker` copies: as
`var` arguments and results (253 to 274 leaves each way), field by field
releases in landing pads, edge copies into block parameters, and
unwind exits building a placeholder `Checker`. The changes, in the
order they were made (figures: `bench/results/wasm-2026-10-12.md`,
"Stage 3"):

- **No frames without waiting.** A program whose functions never call
  a blocking runtime function (`rt3_blocking`) never suspends, so calls
  through function values get no resume points and no frame saves
  (the checker uses the clock, which pulled in the task runtime).
- **One zero exit.** Every fault exit of a function branches to one
  block that zeroes the homes and pushes the zero results (`$Z`), and an
  unwind exit that only hands the fault on (`rt.fault_continue`) and
  returns placeholders is that exit (in a program where nothing waits,
  unwinding is always a fault and the caller reads no result).
- **Large `var` parameters by reference** (`home.xo`, the LLVM
  emitter's decisions 0145 and 0148 in this value model). A `var`
  parameter of a struct or tuple of 16 leaves or more (`HOME_MIN`), of a
  function only called directly, is the address of its home: the
  callee's chain of values stays there (the parameter, field updates of
  a value that dies there, the `var` result of a call that was given
  the home, block parameters every edge passes such a value), fields are
  loaded where they are read, and the updated value is not returned. The
  chain is the greatest set closed under those rules (loops), with exact
  liveness (not plan_locals' intervals), the SSA's repeated reads of
  one call result counted as one value, and the field reads that follow
  a give up served by snapshots taken where the value is defined
  (gotchas/wasm-home-chain-ssa-shapes.md). A forward dataflow checks
  that every use sees its value in the home; if one does not, that home
  falls back (loaded at entry, stored at each return). A caller passes
  its own home when it gives the value up, else a temporary block
  (`memory.copy` from memory, stores from locals). A fault exit zeroes
  the homes, as it returned zero leaves before.
- **Read only homes.** A parameter that is not `var`, of a type some
  function takes as a `var` home (`fn (c: Checker)` methods), is passed
  by reference too: callers mostly hold the value in their home and pass
  its address, and the callee never writes it (a copy when the same
  call also passes that home for a `var` parameter).
- **Releases as one call.** A run of field releases of one struct (the
  landing pads of `Checker` methods released 220 fields one by one) is
  one call of a helper per type and field set, given the struct's
  leaves, or its address when it is in memory.
- **Locals.** Field reads of block parameters alias them as other field
  reads do; temporaries of one instruction (element reads, copies,
  values loaded from memory) share per function scratch locals; a call
  tuple has no locals for the parts its homes hold; and locals are
  renumbered so the most used ones have the one byte indexes.

Results: the module is **8,903,113 bytes** (from 43,193,163), the
goldens still give 120 pass, 21 unsupported, 0 fail, and the checker
prints its 3,398 lines identically; under Node it compiles in 0.17 s
(0.92 s) and runs the seven goldens in 1.9 s and 2.5 GB (10.7 s,
5.9 GB), with a 1 MB worker stack where 4 MB was needed. Most of the
earlier run cost was V8's optimizing tier on huge functions, not the
module's compile (gotchas/wasm-turbofan-tier-up-memory.md). In headless
Chrome 154 the module compiles lazily in about 5 ms, and the checker
runs in a worker on the structs golden with the go backend build's
output; on deeper programs (the recursion golden) it overflows the
worker's stack (`RangeError`), where Node needs between 0.75 and 1 MB.
That, not the size, is now the first blocker for the playground: the
next steps are smaller frames on the checker's recursion (fewer locals
per function), then stage 3's structured control flow.

The build of the checker is slower (14 s and 4.0 GB before, about
37 s and 3.1 GB with `--annotate`), the price of the liveness walks per
value. TestSelfhostWasm still gives 120 pass, 21 unsupported, 0 fail of
141 (121 of 143 after merging main); `wasmMinPass` stays 116.

## Stage 3, continued: frames, structured control flow, writing files (2026-10-12)

Figures: `bench/results/wasm-2026-10-12.md`, "Stage 3, continued".

- **Locals coalesced by liveness** (`coalesce.xo`). V8's baseline
  compiler gives every declared local a stack slot, and a Chrome
  dedicated Worker allows about 0.63 of a 1 MB Node worker
  (gotchas/wasm-browser-stack-locals.md), so the checker overflowed the
  worker on 81 of the 144 goldens. After a function (or helper) is
  lowered, its locals are merged on the emitted wasm code: control flow
  of block, loop, if, br, br_if, br_table (the dispatch loop's
  `i32.const K; local.set PC; br $L` read as a jump to the table's K-th
  target), live locals per basic block, an interference graph (a copy's
  source does not interfere), a greedy coloring per type; parameters
  keep their indexes, unused locals go. The checker's expression cycle
  went from 14,837 parameters and locals to 2,224, and it checks all
  144 goldens in the Chrome worker as its go backend build does.
  plan_locals stays (it decides aliasing and in place updates); the
  coloring only renames.
- **Structured control flow** (`layout.xo`), Ramsey's algorithm ("Beyond
  Relooper"): the emitter writes jumps as markers, then a function
  whose graph is reducible and that cannot suspend is laid out along its
  dominator tree, children in reverse postorder each inside a `block`,
  loop headers inside a `loop`, forward jumps `br $BK`, back jumps
  `br $LK`, a jump to the code that follows dropped; the walk uses an
  explicit stack. Functions that can suspend (their resume points enter
  the middle of the function) and irreducible graphs keep the dispatch
  loop. Every function of the checker is structured; the module is
  8,291,907 bytes (8,616,940 with the dispatch loop), and the seven
  goldens run in about 0.55 s and 340 MB under Node (0.7 s and 520 MB
  with the dispatch loop, 2.3 s and 2.1 GB before either).
- **rt.fs_write**: `path_open` with O_CREAT and O_TRUNC, then `fd_write`
  until every byte is written, errors mapped as rt.fs_read maps them
  (TestSelfhostWasmFsWrite). With it the backend itself builds as wasm
  except for `rt.float_parse_ok` and `rt.float_parse`; built with
  `--lax`, the 17.5 MB module under Node writes modules byte identical
  to the go backend build's for goldens without Float literals.

Next: `rt.float_parse` (exact decimal to binary, in Xo compiled by this
backend), then the backend as wasm without `--lax`, then the playground
page running it in a worker on the virtual tree.

## Consequences

- The playground's path no longer needs the Go compiler and linker in
  the page once stage 5 lands; until then nothing in the playground
  changes.
- The backend has no Go twin: a change to the SSA that the backend does
  not follow shows in TestSelfhostWasm (merge and full tiers), not in a
  differential comparison.
- Node is needed to run the goldens (the test skips without it); the
  modules are standard and run under any WASI preview1 runtime.
- The dispatch loop costs speed (figures in the results file); since
  stage 3 it remains only for functions that can suspend and
  irreducible graphs.
- A module carries its runtime: about 12 KB for the smallest golden,
  16 KB the median (a native arm64 build of the same golden is about
  380 KB with its C runtime).

## Alternatives

- **wat2wasm or wazero as the assembler and runner**: not installed
  (wat2wasm) or not a dependency (wazero); an assembler in Xo keeps the
  compiler self contained, which the playground needs anyway.
- **Binary wasm directly**: smaller and faster to write out, but the
  text is the debugging surface; the assembler can be bypassed later by
  emitting its instruction lists without the text.
- **Large `var` parameters loaded at entry and stored at return**
  (decision 0145's first form): measured as stage 3's fallback, a
  `Checker` copied into and out of memory around each call made the
  module larger (29.9 MB to 40.0 MB at the first attempt); only keeping
  the chain in memory pays.
- **Aggregates in memory** (every struct a pointer to an immutable
  record): simpler calls, but a record per value would need its own
  counting and would not be the LLVM emitter's model of retains on heap
  leaves.
- **Port xo_rt.c with clang --target=wasm32-wasi**: rejected above.
- **WebAssembly exception handling for faults**: rejected above; it may
  return as an optimization once runtimes agree on the final proposal.
