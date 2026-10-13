# Raw memory intrinsics and C ABI exports (decision 0207, stage 1)

Date: 2026-10-13. Branch of the rawmem agent; base aea0c65a (the ADR
stub), merged with origin/main at f0ab329b (atomics, decision 0200;
scan intrinsics, decision 0201). Instruction counts are whole process
instructions retired, minimum of 5 runs, `/usr/bin/time -l`, on the
development Mac with other agents running (counts, not times, are
reported for that reason).

## What landed

- **C ABI exports** (decision 0204's mechanism): every top level
  function of `std/native/llrt/cabi.xo` is called by the C runtime as
  `xo_x_<name>`; `cabi_<feature>.xo` modules (`cabi_tls.xo`,
  `cabi_time.xo`) hold exports only one feature's C calls. Int and Float
  parameters (an address is an Int), an Int, Float, Str, or no result.
  The SSA builders (internal/ssa `declareExports`, selfhost/ssa
  `declare_exports`) build them as roots after the program's code: cabi
  always, a feature's when code of a module importing it was built. The
  emitters give each a global symbol: selfhost/arm64 and internal/arm64
  a branch to the function (Xo functions follow AAPCS64 for Int and
  Float words) or, for a Str result, a frame that stores the length
  through the pointer after the parameters; selfhost/llvm and
  internal/llvm an external C convention function that calls the
  internal one. The wasm backend builds none (`Builder.wasm`).
- **Raw memory intrinsics** in `std/native/rt.xo`: `mem_load`,
  `mem_store` (constant width 1, 2, 4, 8, checked by both SSA builders),
  `mem_copy`, `mem_move`, `mem_set`, `mem_cmp`, `layout` (folded by the
  SSA builders from one table, internal/ssa `Layout` and selfhost/ssa
  `layout_value`, native and wasm columns), the generic `obj_addr[T]`
  (Str, List, handle) and `obj_at[T]` (List, handle; one retain), and the
  address form of the atomics of decision 0200 (`slot_*[Int](a, i, ...)`
  is the word at a + 8 * i). Lowered inline in selfhost/arm64,
  selfhost/llvm, selfhost/wasm (rt2.xo functions and `memory.copy`,
  `memory.fill`), internal/arm64, internal/llvm; memcpy, memmove,
  memset, memcmp are platform library calls on arm64 and LLVM intrinsics
  (memcmp a call) on LLVM. `rs_set_len` is shared with decision 0201.
- Used at run time by the moved code: mem_load (widths 1, 2, 4, 8),
  mem_store (1, 2, 4, 8), mem_copy, mem_move (fmt_int), mem_cmp
  (raw.mem_eq), layout, obj_addr, rs_set_len. mem_set (raw.fill),
  obj_at, and the atomics at an address are lowered by every emitter,
  but no runtime module calls them
  yet, so no test runs them; stage 2 is their first user.
- `std/native/llrt/raw.xo` (text to and from C memory),
  `netpolicy.xo` (Net.only's address policy), `tlsname.xo` (x509 name
  checks, net.ParseIP and IP.String, PEM decoding, alert text).

## The "(0) held by C callers" group (audit: 1,806 lines)

| Audit group | Lines | Now |
|---|---|---|
| xo_rt.c fmt_int, fmt_go_g | 123 | Xo (cabi `fmt_int`, `fmt_float`; num.xo `go_g_text`) |
| xo_rt.c fault messages | 75 | texts in Xo (cabi `fault_*`); the C keeps the entry points that raise |
| xo_rt.c host:port split, address policy | 215 | Xo (netpolicy.xo); std/native/http.xo calls it directly, `rt.net_admits` deleted |
| xo_std.c nstr, nbuf, UTF-8, is_space | 164 | UTF-8 decode and encode and IsSpace in Xo; nstr and nbuf stay (C's own buffers, they go with their last C caller) |
| xo_std.c n_parse_int | 29 | Xo (cabi `parse_int`) |
| xo_std.c n_fmt_duration | 127 | the runtime's callers use Xo (cabi `fmt_duration`); the C copy only under `XO_TEXT_C` (the C path of text, decision 0194) |
| xo_std.c n_fmt_time | 47 | likewise (cabi `fmt_time`; C only under `XO_TEXT_C`) |
| xo_std.c b64_val, n_days_from_civil | 32 | Xo |
| xo_bytes.c fault_bytes_index | 16 | text in Xo |
| xo_time.c zip directory reader | 57 | Xo (cabi_time `tz_zip_entry`) |
| xo_tls.c PEM decode | 160 | Xo (tlsname `pem_next`); ends_with and types_list (16 lines of X509KeyPair's error text) stay |
| xo_tls.c hostname and IP matching | 189 | Xo (tlsname `host_ok`, `parse_ip`); names_load stays (it reads Mbed TLS) |
| xo_tls.c certificate time and hostname messages | 102 | hostname_msg in Xo; rfc3339 and x509_time stay (localtime_r, timegm: (3)) |
| xo_tls.c alert text | 58 | Xo (tlsname `alert_text`) |
| xo_testing.c test output, env, exit code, log records | 185 | not moved: C state under C locks (see below) |
| xo_testing.c fakes, records, memfs | 151 | not moved: likewise |
| xo_proc.c fake processes | 76 | not moved: likewise |

`git diff --numstat aea0c65a bcde613c -- std/llrt`: 883 lines deleted,
151 added (the export declarations and the C shells that call them), so
std/llrt is 732 lines shorter; another 176 (n_fmt_duration 127 and
n_fmt_time 49 with their comments) are compiled only with
`XO_LLRT_C=text`. The three groups not moved (412 lines) are stores of
the test world (records, fakes, memfs, fake processes) that C reads
and writes under its own mutexes: they hold state, which Xo cannot keep
outside a value until (2)'s allocator work gives it a home and (1)'s
atomics guard it, so the audit's "(0)" is (1) and (2) for them.

## Behavior

- Fault messages are the same byte for byte: TestCABIDiff (new,
  internal/llvm cabi_diff_test.go) runs 1,690 input lines (Int faults at
  the bounds and random, Float to Int faults over Go's shortest texts,
  Float32 text, UTF-8 of random valid and invalid bytes, white space,
  Net.only lists and dials of loopback literals) on the Go backend, arm64
  and LLVM: 8,760 output lines, identical on both native backends.
- TLS name checks now follow Go exactly: net.ParseIP's rules (no leading
  zeros in IPv4 fields; the C used the platform's inet_pton) and no 256
  byte limit on a name (the C copied names into 256 byte buffers).
- Rune.is_space is Xo (unicode.is_space) instead of a C call;
  `rt.rune_is_space` and `xo_rune_is_space` are deleted.

## The bar (decision 0193 item 5)

bench/perf/text and bench/perf/httpparse, base f0ab329b (origin/main
before the merge) against this branch, instructions retired, new over
base:

| Workload | arm64 | LLVM |
|---|---|---|
| text none (setup) | 1.000 | 1.000 |
| rune_count | 0.996 | 0.995 |
| trim (after unicode.is_space tests the ranges first; first version 1.040 and 1.037) | 0.978 | 0.985 |
| split | 1.000 | 0.999 |
| lines | 1.000 | 1.000 |
| utf8 | 0.999 | 1.000 |
| int_fmt | 0.998 | 0.997 |
| float_fmt | 1.000 | 1.000 |
| time_fmt | 1.000 | 1.001 |
| dur_fmt | 0.999 | 1.002 |
| parse_int | 0.999 | 1.001 |
| httpparse none | 0.997 | 1.003 |
| h1_get | 1.006 | 1.008 |
| h1_chunked | 0.997 | 1.000 |
| h2_get | 1.000 | 1.000 |

Every workload is within 5 percent; checksums equal.

## Costs

- Every native program carries cabi.xo's closure (Dragonbox Float text
  for the Float to Int fault and Float32 text, Duration and Time text,
  Net.only's policy). A hello world on arm64: 363,720 to 384,440 bytes
  (+20,720; text segment 262,144 to 278,528), base aea0c65a; 403,032
  bytes before the TLS and zone exports moved to feature modules. On
  LLVM 106,344 to 106,456 (+112: LTO drops what the C runtime's reachable
  code does not call).
- The SSA golden of fib has 84 functions (9 before); `go test -short
  ./internal/ssa` took about 90 s instead of about 9 s on the loaded Mac
  (gotchas/exports-grow-every-program.md).

## Tests run

- `go test -short` over the repository (exclude runs/): pass.
- EC2 (r8g, on demand, instance i-EXAMPLE27, terminated), commit
  cb9207d3: internal/llvm TestGolden, TestGoldenWant, TestTLSDiff,
  TestTZDiff, TestRuntime*, TestCryptoDiff, TestHTTPParseDiff,
  TestHTTPFuzz, TestLeakCheck, TestH2Diff, TestHTTP1ParseDiff,
  TestJSONTextDiff, TestIntsDiff, TestNumDiff, TestDigestDiff,
  TestProcSpecDiff: ok (171 s); internal/arm64 TestGoldenRun, TestRun,
  TestBench: ok (34 s), linux/arm64.
- TestSelfhostWasm: 132 pass, 19 unsupported, 0 fail of 151 goldens; after merging origin/main at 28cc2052, 132 pass, 20 unsupported, 0 fail of 152.
- TestCABIDiff on the Mac: pass (arm64, LLVM). TestTLSDiff and
  TestTZDiff on the Mac: pass.
- TestSelfhostSSA, TestSelfhostArm64, TestSelfhostLLVM with
  `XO_SELFHOST_DIRS=net_only,http_tls,tz_table,fault` (30 runs, go
  backend): the Go SSA builder's 3,728 functions identical, the Go arm64
  emitter's 3,731 functions identical, every LLVM run byte identical.
- Selfhost parity by hand (the self hosted emitters' output against
  `xo asmdump`/`xo llvmdump`): identical on hello, the fault program,
  the text program, http_tls, net_only.

## Next, ranked

1. Make the always compiled C callers of the heaviest exports follow use:
   the Float to Int fault and Float32 text called by generated code
   directly (the SSA builders already route Float text to num.xo), so
   Dragonbox leaves programs without Floats; then the http log time.
2. The test world stores (412 lines) after (1) and (2): a runtime state
   record in Xo reached by address (obj_at of a handle the C allocates
   once), guarded by slot_* atomics.
3. Decision 0207's design: reference counts, then the allocator, in Xo.
