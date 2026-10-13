# Runtime port, step 6: HPACK, HTTP/1.1 parsing, Fs paths, process specs (2026-10-13)

Decision 0193 step 6. Machine: the shared 12 core Apple M series Mac
(darwin arm64), several agents running; every run through the heavy
lock, one at a time. Figures are instructions retired (`/usr/bin/time
-l`, the minimum of 5 runs of each workload) of the server process
(bench/perf/httpparse) or of the program (bench/perf/fsproc). Native
builds used the Go driver (`XO_SELF_DRIVER=0`). "C" is the same
compiler with `XO_LLRT_C=http1,hpack` (or `fspath,procspec`).

## What was ported

- **HPACK of the HTTP/2 server** (std/native/llrt/hpack.xo, C path
  hpack_c.xo): the decoder (integers, string literals, Huffman codes by a
  nibble state machine generated from RFC 7541 Appendix B, the dynamic
  table and its size updates) and the encoder (hpack.Encoder's choices,
  the peer's table size settings). C never calls compiled Xo, so the
  connection's fiber (`hpack.Conn.next` in std/native/http.xo's
  serve_conn) calls `rthttp.h2_next`, which stops at each HEADERS or
  CONTINUATION fragment (2: Xo decodes `h2_block`, `h2_fields` goes on
  with the frame) and before writing response heads (3: the
  connection's lock stays held while Xo encodes `h2_enc_job`, so heads go
  out in the order and with the table they were encoded with). The frame
  loop, flow control, and stream state stay C (stream tasks share them
  under the connection's lock). The client's HPACK stays C (cl_do is one
  C function).
- **HTTP/1.1 request parsing** (http1.xo, C path http1_c.xo): the request
  line, the MIME header with continuation lines and every check of
  readRequest and conn.readRequest, the request target (ParseRequestURI
  with IPv6 hosts by BIND's inet_pton rules), the query (ParseQuery,
  keys sorted), chunked bodies and their trailers, and std/http's URL and
  text helpers (url_parts, url_error, path_unescape, query_escape,
  go_quote, canonical_key, equal_fold). `rthttp.hc_read` stops at a head
  (3) or a chunked body (4) and takes the result (`hc_head`,
  `hc_chunked`); the parsed head stays in Xo and becomes the Request (no
  C accessor per field). The client's response parsing stays C.
- **Fs path policy** (fspath.xo, C path fspath_c.xo): os.MkdirAll's and
  os.RemoveAll's walks, os.CreateTemp's names, the working directory with
  / on Windows; the system calls are `rt.fs_raw_*`. The C moved under
  `#ifdef XO_FSPATH_C` with an IOFlags entry (build.IOFlags and xo1's
  io_flags). The program path's / on Windows is in xo_bytes.c (step 5's
  file) and readlink's in xo_win.c's POSIX layer, which C uses; both
  stay.
- **Process specs** (procspec.xo, C path procspec_c.xo): a command's
  environment (the program's or the one set, PWD for a directory, the
  added variables, dedupEnv with Windows' case folding, addCriticalEnv's
  SYSTEMROOT) and the Windows command line (EscapeArg plus every space
  character quoted). xo_proc.c's pr_parse takes them as built (spec flag
  8); its own merging stays for the C path.

## Final (the committed code)

bench/perf/httpparse: 20,000 requests a workload from load/ (a Go
client in another process); "less setup" subtracts each path's `none`
(the server started and stopped). Per request: (workload - none) /
20,000.

| Workload | arm64 Xo | arm64 C | less setup | LLVM Xo | LLVM C | less setup |
|---|---|---|---|---|---|---|
| `none` | 28,533,191 | 28,434,172 |  | 27,457,560 | 27,487,753 |  |
| `h1_get` | 2,193,829,521 | 2,502,533,977 | 0.875 | 2,063,793,292 | 2,464,832,826 | 0.835 |
| `h1_chunked` | 1,289,098,250 | 1,285,308,124 | 1.003 | 1,193,182,697 | 1,221,608,883 | 0.976 |
| `h1_url` | 2,945,235,046 | 3,151,594,539 | 0.934 | 2,866,033,795 | 3,098,951,041 | 0.924 |
| `h2_get` | 4,961,302,807 | 4,870,549,723 | 1.019 | 4,871,097,436 | 4,822,021,043 | 1.010 |
| `h2_varied` | 5,137,144,029 | 5,087,102,363 | 1.010 | 5,016,076,958 | 5,034,249,126 | 0.996 |

Per request on arm64: h1_get 108,265 against 123,705 instructions,
h2_get 246,638 against 242,106. The HTTP/2 figures include TLS (both
directions) and the server's whole stream handling; two more rounds of
h2 with 10,000 requests gave h2_get 1.021 and 1.021 on arm64, 1.006 and
1.014 on LLVM, h2_varied 0.997 and 1.009, 0.987 and 0.994.

bench/perf/fsproc ("less setup": mkdir, temp, cwd less `none`; remove
less `tree`, the tree it removes; proc less `proc_plain`, the same
processes without a directory or variables):

| Workload | arm64 Xo | arm64 C | less setup | LLVM Xo | LLVM C | less setup |
|---|---|---|---|---|---|---|
| `none` | 18,926,827 | 18,939,212 |  | 16,973,202 | 16,863,621 |  |
| `mkdir` | 1,448,679,836 | 1,451,931,363 | 0.998 | 1,446,640,370 | 1,448,428,649 | 0.999 |
| `tree` | 1,156,857,760 | 1,158,591,481 | 0.998 | 1,157,778,764 | 1,155,315,343 | 1.002 |
| `remove` | 2,261,587,970 | 2,242,976,197 | 1.019 | 2,254,672,653 | 2,245,062,024 | 1.007 |
| `temp` | 1,325,400,914 | 1,328,070,230 | 0.998 | 1,316,430,860 | 1,317,884,461 | 0.999 |
| `cwd` | 3,570,627,460 | 3,562,350,449 | 1.002 | 3,563,337,879 | 3,561,348,514 | 1.001 |
| `proc_plain` | 236,892,431 | 236,845,817 |  | 234,731,361 | 234,730,716 |  |
| `proc` | 264,183,189 | 279,673,148 | 0.637 | 257,997,527 | 277,509,581 | 0.544 |

The Fs workloads are system call bound: macOS counts the kernel's
instructions too (400 mkdir walks retire 1.4 billion), so they show
that the port adds nothing measurable, not the policy's own cost.

**The bar (1.05) holds on both backends for every workload**, so the
Xo paths are the default; the C paths stay selectable until the lead's
full tier passes, then a later change deletes them (decision 0198).

## How it got there

- First version (hpack.xo with names and values in two lists, the
  decoded fields' lengths in a Str, the encoder slicing every name and
  value of the job; http1.xo unescaping a byte a C call, the query in a
  Map then sorted): h1_url 1.165 (arm64) and 1.134 (LLVM), h2_get 1.065
  and 1.039, h1_chunked 1.049 and 1.010. Then: an entry is one Str (name
  then value), the lengths a List[Int] (pushes inline), the encoder reads
  the job in place and finds static names among those of the same
  length (STATIC_ORDER); unescape copies runs and packs the rest seven
  bytes a word; query keys are kept sorted as they come (binary search):
  h1_url 0.943 and 0.922, h2_get 1.002 and 1.004.
- h1_chunked moved between 1.015 and 1.041 on arm64 between rounds;
  hc_input copied all the unread input for each parse (50 pipelined
  requests at a time). It now gives a head up to its first empty line
  (the rest cannot change the parse), a chunked body a 4 KiB window first
  (all of the input when the window ends first or an error shows: a line
  cut by the window looks too long), and a trailer is parsed up to its
  empty line: 1.003 and 0.976.
- The HTTP/2 workloads first ran 8 client workers on one connection;
  their instruction counts moved by 2 to 3 percent between sessions (the
  C path too), so they send one request at a time now.

## Correctness

- TestH2Diff (Go backend, LLVM, arm64) with new probes (probeH2HPACK):
  RFC 7541 C.3 and C.4 request sequences, C.4 with each block cut across
  a CONTINUATION, 60 indexed fields evicting past the 4096 byte table
  and read back, every allowed byte Huffman coded, the encoder's blocks
  over several responses traced representation by representation (with
  table sizes 0, 50 then 300, and over 4096 from the client), a 100
  Continue, and 15 decoding edge cases (Huffman padding of 8 bits or
  with a zero, EOS, empty and long codes, integer overflow, lengths past
  the block and of 2^63, size updates, never indexed fields): identical to the Go backend on both paths. TestH2Sanitizers,
  TestTLSDiff, TestTLSLeaks pass.
- TestHTTP1ParseDiff (new): the same server program built with every
  backend answers each input on a connection of its own (sending side
  closed): the corpus of TestHTTPParseDiff, header limits (1 MiB fields,
  5,000 fields, a 70,000 byte target), chunked bodies (40 chunks of 4
  KiB, 200 chunk extensions, sizes of 2^64 - 1 and 17 digits, trailers
  good and bad, bare LF), IPv6 and userinfo hosts, CONNECT, queries, and
  mutated random requests: 30,130 inputs (seed 11, at def13dee) and
  10,130 on the final code (seed 7) identical to the Go backend on LLVM
  and arm64 (Xo path); the C path differed on two inputs, both C defects
  (below): the chunk size is fixed, the IPv6 host the test leaves out on
  the C path (2,129 inputs pass there).
  TestHTTPParseDiff and TestHTTPFuzz (the C, sanitizers) pass.
- TestProcSpecDiff (new): env(1) and printf(1) children with inherited,
  set, and added environments, relative and absolute directories, and
  quoting-sensitive arguments: identical to the Go backend on both
  paths. Goldens http_*, fs_*, proc_cmd, env_dirs, unix_kill on LLVM
  (also under AddressSanitizer) and arm64, both paths.
- TestHPACKTables, TestHTTP1Tables (the generated tables are current),
  TestIOFlagsFspathC, TestXo1IOFlagsSymbols, TestLLRTCPath.
- Windows: scripts/remote-windows.sh --batch on a201bea4 (EC2
  c7i.4xlarge on demand, i-EXAMPLE53, terminated): xo1 built;
  windows/amd64 goldens 148 pass of 148; windows/arm64 148 built of 148;
  the LLVM runtime tests ok; xo test LLVM against Go 16 same, 0 differ.

## Found on the way

- The C parser read a chunk size of 2^63 or more as negative (a negative
  copy length) and took an IPv4 part with a leading zero in an IPv6 host
  through the platform's inet_pton; http1.xo matches the Go backend, the
  C size compare is fixed (gotchas/http-chunk-size-sign-and-ipv6-zeros.md).
- A request body cut short with 256 KiB or more unread gets `Connection:
  close` from the native server (and from plain net/http) but not from
  the Go backend's server; TestHTTP1ParseDiff leaves such inputs out
  (gotchas/http-short-body-close-differs.md).
- The arm64 backend fails ("reads virtual register N before it is
  written") on a scan loop that sets a flag and breaks on an `||` of
  `&&` chains; repro in gotchas/arm64-vreg-read-before-write-break-flag.md.
  Fixed 2026-10-13 (see below). http1.xo keeps `plain_end`: it is not
  only the workaround, unescape's main loop also calls it to copy runs
  of 8 bytes or more at once.

## After the arm64 join fix (2026-10-13)

The fix of gotchas/arm64-vreg-read-before-write-break-flag.md: a block
entered through a Bool join (`a && b`, `a || b`, a flag) takes the
registers of its dominator only, and the join source's facts. It
changes 7 functions of the httpparse server's http1 module on arm64 (39
more instructions in all, `xo asmdump`); LLVM is unchanged. The h1
workloads again on arm64, the same way (Go driver, minimum of 5 runs,
20,000 requests), with the compiler before the fix ("old") and after
("new") in one session; several agents running, so the old/new spread
(up to 1.6 percent on `h1_url`, the old compiler lower; up to 0.9
percent on `h1_get`, the new one lower) is the noise level:

| Workload | new Xo | new C | less setup | old Xo | old C | less setup |
|---|---|---|---|---|---|---|
| `none` | 28,624,456 | 28,547,274 |  | 28,531,858 | 28,752,887 |  |
| `h1_get` | 2,148,186,194 | 2,458,636,560 | 0.872 | 2,159,070,058 | 2,445,814,249 | 0.881 |
| `h1_chunked` | 1,250,756,441 | 1,241,951,524 | 1.007 | 1,261,795,420 | 1,228,299,382 | 1.028 |
| `h1_url` | 2,930,899,607 | 3,140,541,007 | 0.933 | 2,884,326,281 | 3,108,722,399 | 0.927 |

The bar (1.05) holds; the ratios match the final table's (0.875, 1.003,
0.934).
