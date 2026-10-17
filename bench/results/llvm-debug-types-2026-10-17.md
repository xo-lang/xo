# LLVM debug types (decision 0252 stage 4 on LLVM), 2026-10-17

Xo Lang 3. Main ecf73775 against branch llvmdi (compiler/llvm debug.xo:
DWARF types for Str, List, Box, Map and Set (structs), enums, function
values, handles; fragment dbg.values of a value's parts from the block's
insertvalue chains; poison where a part is not known). Mac (arm64), the
http_server golden of internal/llvm, `xo build --backend=llvm` with its
default debug information.

| | main | branch | change |
|---|---|---|---|
| IR, --release | 12 MB | 12 MB | byte identical (cmp) |
| IR, debug | 31 MB | 31 MB | |
| llvm.dbg.value calls | 35,703 | 32,472 | -9% (inferred: a variable with two binding records of one value is now described once; not counted per cause) |
| fragment dbg.values | 0 | 15,625 | |
| clang -cc1 -O2 -g, instructions retired (3 runs) | 225.94, 225.94, 225.92 G | 228.71, 229.40, 227.98 G | +1.2% |
| clang -cc1 peak footprint (median of 3) | 1.248 GB | 1.267 GB | +1.5% |
| xo build, xo's own instructions (2 runs) | 20.55, 20.15 G | 20.22, 20.75 G | within noise |
| xo build, xo's peak footprint | 777 MB | 779 MB | +0.2% |
| object __text | 2,364,864 B | 2,364,904 B | +40 B |
| object __debug_info | 790,073 B | 892,584 B | +13% |
| object __debug_loclists | 793,447 B | 832,914 B | +5% |

Method: each xo wrote the debug and the --release IR of the same program
(`--asm`); `clang -###` gave the -cc1 command of `clang -O2 -g -c`, run
on each IR under a proc_pid_rusage (RUSAGE_INFO_V4) reader that takes
the child's own ri_instructions and ri_lifetime_max_phys_footprint before
reaping it; wall and user time were too noisy with other sessions
running. The 0252 option (1) budget holds: -s and --release builds
change nothing (identical IR), and a debug build costs clang 1.2% more
instructions.

## Addendum: the code's own extractvalues (dbg_note_ext)

Main 6c40366c against branch llvmext (a fragment dbg.value after the
code's own extractvalue of a value whose variable is bound to it only),
the same program and method:

| | main | branch | change |
|---|---|---|---|
| IR, --release | | | byte identical (cmp) |
| llvm.dbg.value calls | 32,472 | 37,610 | +16% |
| clang -cc1 -O2 -g, instructions retired (3 runs) | 228.54, 228.18, 227.77 G | 231.37, 230.94, 230.66 G | +1.2% |
| clang -cc1 peak footprint (median of 3) | 1.285 GB | 1.331 GB | +3.6% |
| object __text | 2,364,972 B | 2,364,972 B | same |
| object __debug_info | 892,584 B | 1,108,844 B | +24% |
| object __debug_loclists | 832,914 B | 872,293 B | +5% |
