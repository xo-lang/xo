# 0193: Porting the rest of the native runtime (std/llrt) to Xo: order, what stays C, and the bar per step

Status: Accepted (2026-10-12), steps 1 and 2 implemented:
`std/native/llrt/text.xo` and `text_c.xo` (step 1),
`std/native/llrt/strings.xo` and `strings_c.xo` (step 2),
`std/native/prelude.xo` (`llrt_*` and the Str methods),
`std/native/rt.xo` (`rs_push_word`, `rs_push_words`),
`std/llrt/xo_bytes.c` (the two primitives), `std/llrt/xo_std.c` (the
ported C under `#ifdef XO_TEXT_C` and `XO_STRINGS_C`),
`internal/check/load.go` (`LLRTUsesC`, `XO_LLRT_C`),
`internal/build/tzdata.go` and its mirror `selfhost/xo/link.xo`
(`IOFlags`), `internal/ssa/build_conc.go`, `build_show.go`,
`build_json.go` and their mirrors `selfhost/ssa/build.xo`, `show.xo`,
`json.xo` (`llrt`), `selfhost/wasm/rt.xo` and `call.xo` (the two
primitives in WAT). Tests: goldens `text_table` and `str_ops_table` (Go,
LLVM, arm64, both paths), `TestLLRTCPath`, `TestIOFlagsTextC`,
`TestIOFlagsStringsC`, `TestXo1IOFlagsSymbols`, `TestFarOutSlot`.
Evidence: `bench/results/runtime-port-2026-10-12.md`
(`bench/perf/text`).

## Context

Decision 0180's bar (every workload of a port within 5 percent of the C
path in instructions, on both native backends) holds for the Unicode
pilot since decisions 0184 (arm64) and 0192 (LLVM). The C runtime
std/llrt is 27,223 lines (2026-10-12): xo_rt.c 6,664, xo_http.c 3,927,
xo_h2.c 3,335, xo_tls.c 1,658, xo_testing.c 1,590, xo_crypto.c 1,540,
xo_proc.c 1,490, xo_std.c 1,353, xo_json.c 983, xo_time.c 969,
xo_fs.c 855, xo_bytes.c 739, xo_stdin.c 337 (xo_unicode.c's 1,783 are
the pilot's C path). The wasm backend (decision 0179) has its own WAT
runtime and no C; runtime code in Xo is compiled with the program, so
each port is code the wasm backend gets for free once it lowers the
primitives the port uses. Decision 0180 item 3 still holds: C never
calls compiled Xo, so a C function used by other C keeps a C copy until
its callers move too.

## Decision

1. **Order** (by risk, size, and benefit to the wasm backend; each step
   lands only behind its C path):
   1. **Number and time text** (xo_std.c, done): Int.parse, a
      Duration's text and Duration.parse, a Time's RFC 3339 text and
      std/json's Time parse, the Debug quoting of Str and Bytes.
   2. **Str operations that decide per rune or per match** (xo_std.c,
      done): Str.replace and the trims (Unicode white space).
   3. **Float and Int text**: Float.parse (Eisel-Lemire with a big
      decimal fallback, which needs a 64 by 64 to 128 bit multiply:
      an intrinsic or 32 bit halves), Float and Int formatting (fmt_int
      and fmt_go_g in xo_rt.c, shortest round trip), base64 and hex
      encode/decode (the wasm runtime has its own WAT versions of most).
   4. **std/json's parser** (xo_json.c, 983 lines): policy over bytes
      into a tape, once List[Int] pushes are inline (they are, decision
      0184 era) and the scans use bytes_span-like primitives.
   5. **Collections and formatting policy in xo_rt.c and xo_testing.c**:
      the Debug text of collections, the test runner's report text,
      xo_time.c's zone rules (969 lines, data plus policy), xo_bytes.c's
      sized integer text.
   6. **I/O policy**: xo_http.c's request and header parsing and
      xo_h2.c's framing and HPACK over byte primitives, xo_fs.c's path
      policy, xo_proc.c's argument building; the system calls stay C.
   Crypto (xo_crypto.c: SHA-2, Ed25519, Argon2) comes after a wrapping
   UInt64 arithmetic story in the native backends (sized integer
   operations are C calls today), whatever its wasm benefit.
2. **What stays C, and why.** The parts of xo_rt.c below what Xo can
   express: reference counts and the allocator (raw memory), lists and
   maps (element layout, growth), fibers and the scheduler (stack
   switching), atomics, faults and unwinding, signals; every system call
   wrapper (sockets, files, processes, clocks, terminals); TLS
   (xo_tls.c over the platform library, constant time code); and the
   bulk byte primitives that decide nothing, because a C call per piece
   from Xo costs more than the C loop: memchr and memcmp searches
   (`str_find`, `str_contains`, `bytes_span`), copies (`str_slice`,
   `rs_append`, `rs_append_bytes`, `str_repeat`, `str_join`), and the
   operations that are only such scans: **Str.split, Str.lines, and
   Str.rune_count stay C**. Their Xo versions (a find call and a copy
   call per piece; rune_count a branchless byte loop) measured 1.02 to
   1.32 times the C's instructions (split 1.09x, lines 1.13x on LLVM,
   rune_count 1.13x on arm64, after a memchr primitive) and were taken
   out; the wasm backend keeps its WAT versions of them.
3. **Mechanism.** A step is a runtime module `std/native/llrt/<name>.xo`
   (decision 0180) and its C path `<name>_c.xo` (the same API over the C
   functions, which move under `#ifdef XO_<NAME>_C` so no pack contains
   them). `XO_LLRT_C=<name>[,<name>]` or `all` at build time selects C
   paths (the Go checker; `XO_UNICODE=c` still selects unicode's);
   `IOFlags` (and xo1's `io_flags`) add `-DXO_<NAME>_C` when the
   generated code calls them, so such builds compile the runtime
   locally. Where the SSA builder lowered a construct to an `rt.` call
   (Display and Debug of Duration, Time, Str, Bytes; std/json's Duration
   and Time), it calls the native prelude's `llrt_<name>` instead
   (`fn.llrt`, mirrored as `Builder.llrt` in selfhost/ssa), unchecked
   like the C call it replaces. The self hosted compiler has no switch.
4. **Primitives added.** `rt.rs_push_word(s, w, n)` (s plus the n low
   bytes of an Int, the lowest first) and `rt.rs_push_words(s, w0, w1,
   w2, w3, lens)` (four such pieces, lengths in nibbles): short text is
   assembled in Ints by Xo and appended with one C call instead of one
   per byte. Both are in the WAT runtime too.
5. **The bar for every step** is decision 0180's: on arm64 and LLVM,
   each workload of the step's bench/perf program (instructions retired,
   minimum of 5 runs, `/usr/bin/time -l`) within 5 percent of the C
   path; goldens identical on the Go backend, arm64, and LLVM with
   either path; LLVM goldens free every allocation; TestSelfhostSSA
   identical over the goldens; the pack (decision 0157) rebuilt twice
   byte identical. The Xo path is the default once its goldens pass; the
   C path stays selectable until the bar holds on both backends, and a
   later change deletes the C path (`<name>_c.xo`, the `#ifdef` C, the
   IOFlags entry) once it does.
6. **Results of steps 1 and 2** (bench/perf/text, whole process
   instructions, Xo over C, arm64 and LLVM; results file, "Final"):
   Int.parse 0.70x and 0.71x, Duration.parse 0.80x and 0.67x, Time text
   0.21x and 0.18x (the C used snprintf), std/json Time 1.03x and
   0.76x, Debug quoting 0.51x and 0.49x, replace 0.76x and 0.68x, trims
   1.01x and 1.01x: the bar holds. **Duration text misses it: 1.16x on
   arm64 and 1.12x on LLVM** (from 1.30x and 1.43x in the first
   version). The rest of the cost is the Xo digit loop (an `sdiv` and
   `msub`, a checked shift, and the loop compare per digit on arm64)
   against C's unchecked loop into a stack buffer. So `XO_LLRT_C=text`
   stays. Next experiments, ranked: (a) the arm64 backend lowering a
   division by a constant to a multiply high (internal/arm64 isel and
   selfhost/arm64; it helps every Xo integer formatting loop), then
   remeasure with `bench/perf/text/measure.sh`; (b) shift checks the
   range analysis proves (`w` stays below 2^56 while it has at most 7
   bytes: a fact from the loop count); (c) the 9 digit fraction as two
   halves of 4 and 5 digits, halving the loop trips. A 200 byte digit
   pair table read from a `Bytes` constant was tried and was slower
   (1.39x and 1.40x).

## Consequences

- The wasm backend runs the ported code: str_ops_table, native_display,
  unicode_table, regexp_table, std_error_display, and misc pass under
  Node with the Xo text (Duration Display and Str Debug through
  text.xo); text_table and native_json still stop at std/json's C only
  primitives (`json_parse`, `rs_append_json_str`).
- The C runtime loses the ported code from every pack (338 lines of
  xo_std.c under `XO_TEXT_C`, 74 under `XO_STRINGS_C`); sizes and keys in
  the results file.
- A Display or Debug of these types is a call of a compiled function
  instead of a C call, so callers get landing checks for faults the
  code cannot raise (the SSA goldens grew); measured within the
  workloads above.
- `bench/perf/text` (12 workloads and the setup alone) is the bench for
  these steps; `measure.sh` takes `XO_LLRT_C` and `WORKLOADS`.

## Alternatives

- **Port xo_rt.c next, as decision 0180 item 7 first listed.** Most of
  it needs raw memory or stack switching; its text helpers (fmt_int,
  fmt_go_g) are step 3 here.
- **Port split, lines, and rune_count too, for the wasm backend.** Tried
  (with a memchr primitive for one byte separators): 1.02 to 1.32x; the
  wasm runtime already has them in WAT.
- **One switch per module (`XO_TEXT=c`, ...)** like `XO_UNICODE`.
  Rejected for one list variable; `XO_UNICODE=c` kept for scripts.
- **Make the Xo default only after the bar.** The pilot's rule (Xo
  default, C selectable) is kept: the goldens prove the semantics, and a
  default C path would leave the Xo untested by every other test.
