# std built apart for LLVM: three things that looked like other things

Status: Observed (2026-11-21, task #607, decision 0738)

Symptom 1: a program linked against std's LLVM objects failed with
`undefined hidden symbol: x.prelude.llrt_println` and every other std
function it called. Cause: the objects came from an XO_STD_ROOTS build
with dead function removal on. The removal's roots are main, the C
exports and the entries (compiler/arm64 reach.xo), not the std roots
the SSA builder declared, so it kept only what the runtime reaches
(6,368 functions, 662,055 lines of IR, which I first reported as "all
of std"). The native store's build turns the removal off (`xo dev
stdobj`: XO_DCE=off). Fix: std's own LLVM build roots every plain std
function (drop_unreached_store).

Symptom 2: a program that uses no time zone and no sort failed to link
with `undefined symbol: xo_tz_table` and `qsort`, referenced from
`x.tztable.data` and `x.internal.rt_probe`. Cause: std's functions are
in the named section `xo_text` (the runtime's code range for
preemption), which `-ffunction-sections` does not divide, and the
runtime's `__start_xo_text` and `__stop_xo_text` keep every input
section of that name alive, so `--gc-sections` stripped nothing. `ld
-r` of the modules into one object merges sections and does the same.
Fix: `-fseparate-named-sections` on std's objects and `-Wl,-z,start-stop-gc`
on the link; the module objects go to the link as they are.

Symptom 3: clang rejected some of std's modules with `use of undefined
value '@xo_entry'`, and the link then lacked their functions, which
read as a linkage problem. Cause: std's own build left the program
entry out without declaring it, and the runtime takes its address. The
last two lines of clang's output held only the caret. Fix: declared in
std's build; read the first `error:` line of a failed compile.

Avoid: when a build made for one purpose (the native store's) is
reused for another, list what its options were for before dropping or
keeping one; and print a failing tool's first error line, not its tail.
