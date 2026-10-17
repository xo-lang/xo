# rt.mem_copy_block: the runtime's bulk copy as vector loops (2026-10-17)

Decision 0207's amendment (owner, 2026-10-17). bench/perf/xo `branch`
copies a 50,000 entry map on write 2000 times (`var c = m; c.put(..)`,
about 1.9 MB a copy) and `copyset` a list: their time is
std/native/llrt/libcstr.xo copy_words, the no-libc memcpy. Its 32 byte
loop of words was 18 arm64 instructions per 32 bytes (each `i + k`
offset rematerialized per load and store, no ldp or stp), 9 after Xo
lang 4's address fold (e3b7139c); LLVM vectorizes the same source.
copy_words now copies the multiple of 32 with rt.mem_copy_block:

- arm64: `ldp q30, q31, [x16], #32` / `stp q30, q31, [x17], #32` twice,
  `subs`, `b.ge`: 64 bytes in 6 instructions, then one 32 byte pair.
  The in-house assembler gained ldp and stp of q registers.
- amd64: four `movups` through xmm14 and xmm15 at index r11, 32 bytes
  an iteration (the emitter has only those two vector scratch
  registers).
- LLVM: the old loop, expanded inline (two words loaded then stored,
  twice per 32 bytes); llvm.memcpy would call memcpy, its own caller.

## Method

linux/amd64 and linux/arm64 in Docker (scripts/docker-linux.sh),
`valgrind --tool=cachegrind --cache-sim=no` I refs of bench/perf/xo
built by a linux xo cross built with the Go backend from main f37b6592
(base) and from the branch (new). Output agrees but for the printed
time. Goldens: 186 of 186 on darwin/arm64, linux/arm64 (arm64 and LLVM
backends), and linux/amd64 (no C toolchain).

## Results (I refs)

| workload | arm64 base | arm64 new | LLVM arm64 | amd64 base | amd64 new | LLVM amd64 |
|---|---|---|---|---|---|---|
| branch | 1116637119 | 385018523 (0.345) | 382507764 | 1365531243 | 877773727 (0.643) | 686864921 |
| copyset | 40709479 | 24691009 (0.607) | 22190320 | 50542778 | 39862946 (0.789) | 28239816 |
| strings | 17344768 | 16424176 (0.947) | 13926369 | 21986678 | 21373162 (0.972) | 13789439 |
| putget | 424757382 | 422916256 (0.996) | 329370067 | 554594173 | 553366409 (0.998) | 373782520 |

LLVM's builds are unchanged (the same loop). arm64 branch is now 1.01
times LLVM's instructions (was 2.92 on main, 5.47 before e3b7139c);
amd64 1.28. amd64's next step is 64 bytes an iteration, which needs two
more vector scratch registers or saving them around the loop.
