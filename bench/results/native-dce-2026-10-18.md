# Dead function removal in the native backends (2026-10-18)

Owner request through the lead: a hello world was 603 KB native (arm64)
against 359 KB with LLVM. Measured on the development Mac (darwin/arm64),
main 717eefab plus compiler/arm64 reach.xo.

## Diagnosis

Hello, before the change (570 KB native, 359 KB LLVM), by section:

| Section | arm64 | LLVM |
|---|---|---|
| __text | 358,008 | 243,568 |
| __cstring | 35,649 | 18,830 |
| __const | 42,980 | 22,208 |
| __eh_frame + __unwind_info | 32,052 | 11,224 |

A reachability walk over the arm64 binary's own disassembly (calls,
branches, adrp+add addresses, data words that hold a function's address;
every non-Xo symbol a root) found 578 of 1,222 functions (87,488 bytes,
24.4% of __text) unreachable. They are runtime and std code
(std/native/llrt): parts of the thread pool inlined into their callers,
the Windows poll loops on darwin, unwinder and profiler paths the program
does not use, Map instances. Unreachable after inlining is not dead
source: llrt-coverage-2026-10-18.md has that survey.
LLVM's globaldce drops them; the arm64 backend emitted every
SSA function and compiler/macho does no dead stripping. User code was
already pruned by the checker (lazy checking).

## Change

compiler/arm64 reach.xo drop_unreached, run by the driver after inlining
and before code generation (arm64 and amd64). Roots: main, the C ABI
exports (Func.export: the runtime's xo_x_ entry points) and Func.entry.
A function reaches every function an operation in its blocks names (Call,
Closure, FuncAddr, Rebind), which is every way the emitter refers to a
function symbol. Values left outside blocks (inlined calls) do not count:
counting them kept 1,009 of 1,015 hello functions. XO_DCE=off keeps every
function, XO_DCE=debug prints the counts, XO_DCE=list the dropped names.

## Results

Bytes and cold build time (XO_BACKEND_CACHE=off XO_BIN_CACHE=off):

| Program | arm64 before | arm64 after | change | LLVM |
|---|---|---|---|---|
| hello | 583,942 | 428,917 | -26.5% | 367,448 |
| JSON server (examples/notes-service) | 1,414,245 | 1,178,068 | -16.7% | 1,278,120 |
| HTTP server (bench/perf/http/xo) | 2,476,117 | 2,112,036 | -14.7% | 2,792,152 |
| TLS client (bench/perf/tls) | 2,176,500 | 1,882,723 | -13.5% | 2,371,880 |

Functions kept, darwin/arm64 (linux/arm64 and linux/amd64 alike):
hello 441 of 1,015 (linux 511 of 1,172), JSON 817 of 1,741, HTTP 1,191 of
2,430, TLS 1,058 of 2,215.

Build time: the emit phase of the HTTP server fell from 490 to 438 ms
(-11%); whole builds were within noise (1.9 s), since checking and SSA
lowering, which run before the removal, dominate.

Checked: TestDeadFunctionsDropped, the golden dce_values (functions
reached only as values), 46 fault, defer, task and HTTP goldens on arm64,
and internal/llvm's TestProf*, TestTrace and TestDebugInfo* (XO_PROF,
XO_TRACE, DWARF on arm64) before the gate.

## Open

- Hello stays 17% above LLVM: string constants of dropped functions that
  live code still shares, per function code size, __data that LLVM puts in
  bss, and the LINKEDIT symbol table.
- Running the removal before RC insertion and inlining would also cut
  their time, once each SSA pass that adds calls by name (conc.xo,
  budget.xo, build.xo's rt helpers) is audited.
- Source dead on every target: llrt-coverage-2026-10-18.md (20 functions
  deleted, 140 to cover with tests).
