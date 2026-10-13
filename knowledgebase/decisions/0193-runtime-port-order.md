# 0193: Porting the rest of the native runtime (std/llrt) to Xo: order, what stays C, and the bar per step

Status: Accepted (2026-10-12), steps 1 to 6 implemented (step 3:
item 8 below; step 4, std/json: item 9, 2026-10-13; step 5: item 10,
2026-10-13; step 6, the I/O policy: item 11, 2026-10-13):
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
7. **Steps (a) to (c) for Duration text** (decision 0194, results
   `bench/results/const-div-2026-10-12.md`): (a) is done (both arm64
   emitters lower a division by a constant to a multiply high), (b) is
   done in text.xo's source (masks the ranges use), and (c) was tried
   two ways and not kept. Duration text is now **1.171x on arm64 and
   1.087x on LLVM**: (a) costs instructions on arm64 (`sdiv` is one
   instruction) while saving time, so the bar is still missed and
   `XO_LLRT_C=text` stays. The next experiments are decision 0194's
   ranked list (wide constants as logical immediates, ranges of call
   results for the checked shifts).

8. **Step 3, Float and Int text** (results
   `bench/results/runtime-port-step3-2026-10-12.md`, "Final"):
   `std/native/llrt/num.xo` (and its C path `num_c.xo`, `XO_LLRT_C=num`)
   does the decimal text of an Int (eight digits a word by lane
   multiplies), a Float's text (Dragonbox, Go 1.26's ftoadbox.go, with
   Go's 'g' layout and FloatStr's `.0`), Float.parse (Go 1.26's atof.go
   whole: special values, decimal and hex text with underscores, the
   exact path, Eisel-Lemire, the 800 digit decimal, atofHex; subnormal
   results rounded in Eisel-Lemire instead of the big decimal), and
   Bytes.from_hex, to_base64, from_base64; the 128 bit powers of ten and
   the decode tables are generated (`num_tables.xo`, TestNumTables).
   - The SSA builders (internal/ssa `numText`, selfhost/ssa `num_text`)
     route every interpolation, Display, and Debug of an Int or Float and
     std/json's Int to the prelude's `llrt_concat_int` and
     `llrt_concat_float`; with `XO_LLRT_C=num` (or without the native
     prelude) they emit `str.concat_int` and friends as before. A Float32
     and the cold fault messages keep the C call.
   - **Intrinsics**, lowered inline by both arm64 emitters, both LLVM
     emitters, and the wasm runtime (C versions in xo_bytes.c for any
     other call): `rt.int_wrap` with a constant op (add, sub, mul, and two
     new ops, 3 shift left and 4 logical shift right by the amount mod
     64), `rt.u64_mul_hi` (umulh; an i128 multiply on LLVM; four 32 bit
     products in WAT; the 64 by 64 to 128 multiply step 3 needed),
     `rt.float_bits`, `rt.float_from_bits`, and `rt.bytes_word` (an
     unchecked 8 byte load; the callers check the range).
   - arm64 takes every positive logical immediate (repeating patterns)
     in `and`, `orr`, `eor`; `rs_push_word` and `rs_push_words` make a
     new string with room when appending to the literal `""`.
   - Float.parse on the native backends now parses what the Go backend
     parses (the C took neither underscores nor hex floats nor texts
     over 4096 bytes; gotchas/native-float-parse-differed-from-go.md).
   - The bar holds on both backends (instructions less setup, Xo over
     C, arm64 and LLVM): Int text 0.800x and 0.734x, Float text 0.019x
     and 0.014x (the C ran snprintf up to 17 times a Float), Float.parse
     0.282x and 0.273x, base64 0.385x and 0.310x, hex (from_hex in Xo,
     to_hex in C) 1.003x and 0.942x. The C Float.parse, hex decode and
     check, and base64 functions moved under `#ifdef XO_NUM_C`, with
     their IOFlags entry (and xo1's `io_flags`).
   - **What stays C**: Bytes.to_hex (a loop that decides nothing per
     byte: the Xo version, 16 bytes a call, retired 1.09x the C's
     instructions on arm64); fmt_int and fmt_go_g in xo_rt.c, which the C
     runtime itself uses (fault texts, collections' Debug) until step 5
     moves those callers; std/json's Float text and number parse
     (`rs_append_json_float`, `n_parse_float`, step 4); Float32 text;
     std/json's base64 of Bytes (`rs_append_b64_quoted`, step 4).
   - Correctness: TestNumDiff (336,386 lines, Go's strconv and encoding
     packages as the reference) on arm64 and LLVM, both paths; the LLVM
     and arm64 goldens; TestSelfhostSSA, Arm64, LLVM, and Wasm.
   - Next, ranked: (a) std/json's Float text and number parse on num.xo
     (step 4, the same Dragonbox digits in Go's json layout); (b) delete
     the C path of num once the lead's full tier passes (num_c.xo, the
     XO_NUM_C C, the IOFlags entry, `numText`'s C branch; done by
     decision 0198, 2026-10-13); (c) a Float32
     text through `float_bits` of the widened double; (d) to_hex again
     with a 32 byte push primitive, so a call covers 16 input bytes
     without the tail call.

9. **Step 4, std/json** (results
   `bench/results/runtime-port-step4-2026-10-13.md`, "Final"):
   `std/native/llrt/jsontext.xo` (and its C path `jsontext_c.xo`,
   `XO_LLRT_C=jsontext`) replaces std/llrt/xo_json.c: the parser
   (Decoder.Token's state machine and the scanner, one function, string
   bodies scanned eight bytes a word; the same tape, with an escape flag
   and a high byte flag in a string node's length word that only
   jsontext reads), the tape readers the generated decoders call
   (`json_str`, `json_find`, `json_members`, `json_int_ok`, `json_int`,
   `json_float_ok`, `json_float`, `json_next`), and the encoders' text
   (`rs_append_json_str`, `rs_append_json_float` as num.xo's
   `append_json_float`, std/json's Bytes as `append_base64` in quotes and
   `from_base64`). The SSA builders call the native prelude's
   `llrt_<name>` for each (internal/ssa `jsonRT`, selfhost/ssa
   `json_rt`); std/native/json.xo calls `jsontext.parse`. num.xo gained
   `parse_float_at` (a number read in place), `append_json_float`, and
   `append_base64`.
   - The bar holds on both backends (instructions less setup, Xo over C,
     arm64 and LLVM): small objects 0.622x and 0.568x, Int arrays 0.946x
     and 0.900x, Float arrays 0.660x and 0.614x, strings 0.937x and
     0.827x, Map objects 0.956x and 0.925x, the parser alone 0.865x and
     0.725x, invalid texts 0.439x and 0.408x, encoding structs 0.124x
     and 0.112x, strings 0.938x and 0.900x, Floats 0.021x and 0.016x
     (the C ran snprintf and strtod up to 17 times a Float), Bytes
     0.704x and 0.557x. So **nothing of std/json stays C** on the default
     path: xo_json.c and xo_rs_append_b64_quoted moved under `#ifdef
     XO_JSONTEXT_C`, with their IOFlags entry (and xo1's `io_flags`); the
     pack is 4.0 percent smaller. Int arrays first measured 1.111x and
     1.052x (the digits read twice through an `Int?` helper); `int_ok`
     and an unchecked `int_at` fixed it.
   - Correctness: TestJSONTextDiff (27,900 inputs decoded into 15 types
     and encoded again, the Go backend's output the reference, both
     native backends and both paths, the LLVM build leak free), golden
     `json_text`, TestJSONParseDiff (the C path), TestSelfhostSSA over
     the json programs.
   - wasm: native_json, text_table, codex_gaps, and json_text now run
     (128 of 145 goldens); codex_gaps exposed a wasm runtime result type
     mismatch, fixed (gotchas/wasm-runtime-result-type-mismatch.md).
   - IOFlags: the XO_NUM_C scan (step 3) used a plain substring search,
     so xo1's own `io_flags` table (string data naming xo_float_parse)
     would turn it on for xo1's link; it counts only references now, as
     the others do.
   - Next, ranked: (a) delete the C path of jsontext once the lead's full
     tier passes (jsontext_c.xo, xo_json.c, the guarded
     xo_rs_append_b64_quoted, the IOFlags entries, `jsonRT`'s C branch,
     TestJSONParseDiff and std/llrt/testdata/json_diff.c, http_fuzz.c's
     JSON part, which test only the C; done by decision 0198,
     2026-10-13); (b) one call per Int or Float
     element instead of `_ok` then the value (an `llrt_json_int` that
     returns a flag word, the builders' Int and Float arms), the largest
     remaining cost of `ints` and `floats`; (c) step 5 (collections' Debug
     text, the test runner's report, zone rules, sized integer text).

10. **Step 5, collection and test text, zone rules** (2026-10-13, results
    `bench/results/runtime-port-step5-2026-10-13.md`, "Final"):
    - The Debug text of collections was already generated code; its C
      leaves, a Rune's Display and Debug and a UInt64's text (sized
      integer text: the narrower types print as Ints through num.xo), are
      `std/native/llrt/show.xo` (C path `show_c.xo`, `XO_LLRT_C=show`,
      `-DXO_SHOW_C`); the SSA builders call `llrt_append_rune`,
      `llrt_append_rune_quote`, `llrt_append_u64` (`fn.showText`,
      selfhost `show_text`).
    - std/time's zone rules are `std/native/llrt/tz.xo` (C path `tz_c.xo`,
      `XO_LLRT_C=tz`, `-DXO_TZ_C`): TZif parsing, the POSIX TZ footer
      parsed once per zone, Go's lookup and cache. A parsed zone is one
      List[Int]; xo_time.c keeps the zone data (`tz_data`: constant names'
      TZif data, the embedded zoneinfo.zip, the host's files and a
      $ZONEINFO zip, unchanged format and order) and the table of loaded
      zones (`tz_find`, `tz_keep`: the list held shared, released at exit
      for the leak check). time.Zone carries `tz.Rules{id, w}`.
    - A property test case's report (choices, input lines, the shortlex
      check of XO_PROP_BEST) is `std/native/llrt/report.xo` (C path
      `report_c.xo`, `XO_LLRT_C=report`, `-DXO_REPORT_C`) over the
      primitives `gen_choices`, `test_write`, `prop_best`, `test_quit`
      (std/native/rttest.xo).
    - The bar holds on both backends (bench/perf/text, less setup, arm64
      and LLVM): debug 0.663x and 0.622x, zone_offset 0.729x and 0.651x,
      zone_fmt 0.987x and 0.983x, zone_civil 0.864x and 0.821x, zone_load
      1.007x and 0.966x. bench/perf/report (a process per case): 0.989x
      to 1.006x, within the 0.4 percent noise of a test process; the
      report itself is too small to see.
    - **What stays C**: the zone data and table (files, mutex, readlink);
      the fault texts of sized integer arithmetic (C `xo_int_op` raises
      them; they move with sized integer operations); `xo_rs_append_rune`
      (BytesBuilder.write_rune); the literal pieces and cold fault
      `str.concat_int` of Debug text; XO-TEST-FAIL and XO-TEST-LOG
      framing (writes of text the generated code made); the property
      generator. The pass and fail lines are the harness's, not the
      runtime's.
    - Correctness: TestTZDiff (every zone of Go's database, around its
      transitions from 1700 to 2250, embedded, system, and $ZONEINFO
      data, both paths); goldens show_table and tz_table on LLVM and
      arm64 with either path; cmd/xo TestNoGCTests prop, same,
      test_logs with either path; the pack built twice byte identical.
    - Next, ranked: (a) delete the C paths of show, tz, and report once
      the lead's full tier passes (show_c.xo, tz_c.xo, report_c.xo, the
      `#ifdef` C, the IOFlags entries, `showText`'s and the testmode C
      branches); (b) sized integer operations in Xo (`int_op`,
      `int_conv`, `rune_of` as llrt functions the SSA builder inlines with
      constant bits), which would take their fault texts along; (c)
      XO0520 leaving scalar bindings alone
      (gotchas/xo0520-scalar-copy-reassigned.md), which cost tz.xo two
      workarounds; (d) TestGoldenThreads with the zone data
      (gotchas/threads-golden-needs-zone-data.md).

11. **Step 6, I/O policy** (2026-10-13, results
    `bench/results/runtime-port-step6-2026-10-13.md`, "Final"):
    - **C never calls Xo, so the C loops stop for Xo.** The I/O loops
      (the HTTP/1.1 reader, the HTTP/2 connection's frame loop) stay C,
      because they hold sockets, buffers, and state that other fibers
      share under C locks; where they parsed, they now return to the
      connection's Xo code with a code and take the result back.
      `rthttp.hc_read` returns 3 (a request head to parse at
      `hc_input`; the result to `hc_head`) and 4 (a chunked body; the
      result to `hc_chunked`), resuming where it stopped
      (`hc->rd_phase`). `rthttp.h2_next` returns 2 after each HEADERS or
      CONTINUATION fragment (Xo decodes `h2_block`; `h2_fields` goes on
      with the frame, as `hp_decode_part` did) and 3 before response
      heads go out: the connection's lock stays held from the 3 until
      `h2_encoded` (Xo code that cannot block in between), so heads go
      out in the order and with the dynamic table they were encoded
      with, and no stream can be reset between the encoding and the
      write. A 100 Continue and the peer's SETTINGS_HEADER_TABLE_SIZE
      pass through the same job (`h2_enc_job`).
    - `std/native/llrt/hpack.xo` (C path `hpack_c.xo`, `XO_LLRT_C=hpack`):
      the server's HPACK decoder and encoder; the Huffman decoder is a
      nibble state machine (`hpack_tables.xo`, generated by
      TestHPACKTables from RFC 7541's tables), an entry of the dynamic
      table is one Str, the decoded fields go to C as one Str and a
      List[Int] of lengths.
    - `std/native/llrt/http1.xo` (C path `http1_c.xo`, `XO_LLRT_C=http1`):
      request heads, targets, queries, chunked bodies and trailers, and
      std/http's URL and text helpers (`http1_tables.xo`, generated by
      TestHTTP1Tables). The parsed head becomes the Request in Xo; C
      keeps only what reading the body and answering need (`hc_head`'s
      method, version and flags, Content-Length, a status error's text).
    - `std/native/llrt/fspath.xo` (C path `fspath_c.xo`,
      `XO_LLRT_C=fspath`, `-DXO_FSPATH_C`): MkdirAll's and RemoveAll's
      walks, CreateTemp's names, the working directory with / on Windows,
      over `rt.fs_raw_*` system calls.
    - `std/native/llrt/procspec.xo` (C path `procspec_c.xo`,
      `XO_LLRT_C=procspec`): a command's environment (dedupEnv, PWD,
      SYSTEMROOT) and the Windows command line (EscapeArg); xo_proc.c's
      pr_parse takes them as built (spec flag 8).
    - The bar holds on both backends (instructions less setup, arm64 and
      LLVM): HTTP/1.1 GETs 0.875x and 0.835x, chunked bodies 1.003x and
      0.976x, escaped targets with 20 query parameters 0.934x and 0.924x,
      HTTP/2 GETs 1.019x and 1.010x, HTTP/2 with changing fields 1.010x
      and 0.996x (bench/perf/httpparse); the Fs walks 0.998x to 1.019x
      (system call bound) and process environments 0.637x and 0.544x
      (bench/perf/fsproc). The Xo paths are the default.
    - **What stays C, and why**: the frame loop, flow control, and the
      stream table of HTTP/2 (shared with stream tasks under the
      connection's lock; a trampoline per DATA frame would cost more than
      it moves); the HTTP/2 request checks after decoding (h2_collect,
      h2_request; small, and they build the C request the C response code
      reads); the client (`cl_do`: connection, TLS, redirects, response
      parsing, its HPACK) as one C function; response writing
      (hw_response: Date, sniffing, chunking into the socket buffer);
      sockets, TLS, and every system call; xo_win.c's readlink and
      xo_bytes.c's program path separators (C uses the first; the second
      is step 5's file). The C copies of HPACK and the request parser
      stay compiled while the client and the C paths use them.
    - Correctness: TestH2Diff with probeH2HPACK (RFC 7541 C.3 and C.4,
      cut blocks, eviction, every byte Huffman coded, encoder traces with
      table sizes, 15 decoding edge cases), TestHTTP1ParseDiff (new:
      the Go backend as the reference over real connections; 30,130
      inputs), TestProcSpecDiff (new), TestH2Sanitizers, TestTLSDiff,
      TestHTTPParseDiff, TestHTTPFuzz, the http, fs, and proc goldens on
      LLVM (and AddressSanitizer) and arm64, both paths; the Windows
      batch (148 of 148 windows/amd64 goldens pass, 148 build for
      windows/arm64).
    - Found: two C parser defects (a chunk size of 2^63 or more read as
      negative, now fixed; IPv4 leading zeros in IPv6 hosts accepted by
      the platform's inet_pton), the Go backend not closing after a body
      cut short where native and plain net/http do, and an arm64 backend
      failure on a flag-and-break scan loop (gotchas in the results).
    - Next, ranked: (a) delete the C paths of hpack, http1, fspath, and
      procspec once the lead's full tier passes (decision 0198's list);
      (b) the client in Xo: `cl_do` split at its socket and TLS calls the
      way hc_read was, so its response parsing and HPACK use http1.xo and
      hpack.xo and the C copies go; (c) the HTTP/2 request checks
      (h2_collect, h2_pseudos_ok, h2_request) in Xo, with the request
      built in Xo as HTTP/1.1's is; (d) done 2026-10-13: the arm64
      emitter bug is fixed in both emitters
      (gotchas/arm64-vreg-read-before-write-break-flag.md; h1 figures
      again in the step 6 results file); (e) find why the Go
      backend's server keeps a connection open after a body cut short
      (gotchas/http-short-body-close-differs.md) and make TestHTTPParseDiff
      compare with the Go backend instead of plain net/http.

## Consequences

- The wasm backend runs the ported code: str_ops_table, native_display,
  unicode_table, regexp_table, std_error_display, and misc pass under
  Node with the Xo text (Duration Display and Str Debug through
  text.xo); text_table and native_json, which stopped at std/json's C
  only primitives (`json_parse`, `rs_append_json_str`), run since step 4
  (item 9).
- The C runtime loses the ported code from every pack (338 lines of
  xo_std.c under `XO_TEXT_C`, 74 under `XO_STRINGS_C`); sizes and keys in
  the results file.
- Decision 0198 (2026-10-13) deleted the C paths of strings (step 2),
  num (step 3), and jsontext (step 4) with decision 0180's unicode path:
  3,373 lines of std/llrt (xo_unicode.c 1,783, xo_json.c 983, xo_std.c
  421, xo_bytes.c 186), their `<name>_c.xo`, IOFlags entries, and the SSA
  builders' C branches. `XO_LLRT_C` keeps text, show, tz, and report.
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
