# Size-aware inlining: the few sites rule (2026-10-17 to 2026-10-18)

Xo Lang 2, for the lead's queue item "size-aware inlining limit".
Landed as variant F with the owner's acceptance (2026-10-18, through the
lead): about 3% smaller binaries for iface +0.79% and TLS handshakes
+0.2 to 0.3%.

## The rule before

compiler/arm64 inline.xo inlines a function within INLINE_MAX_INSTRS (40)
and INLINE_MAX_BLOCKS (12) everywhere. A larger one was still inlined at
every site when it had at most INLINE_FEW_SITES (2) sites, within
INLINE_FEW_BUDGET (500 instructions over its sites) and INLINE_FEW_BLOCKS
(80). Most of std's mid sized helpers (60 to 180 instructions) have one
or two sites, so they were copied into their callers, and a copy outside
a loop saves only a call.

## The ledger

`XO_INLINE_DENY=name[,name...]` (a prefix ends in `*`) leaves functions
out of inlining: one build per callee measures its text against its
cost. TLS client (bench/perf/tls measure.sh, Mac, N=100; text of
c-arm64-xo, x25519 and p256 instructions retired). Base: text 1,653,816,
x25519 674.8M, p256 789.1M; base noise x25519 0.03%, p256 0.28%.

| denied | text saved | x25519 | p256 |
|---|---|---|---|
| x509verify.parents | 12.2 KB | +0.10% | +0.17% |
| asn1.cat | 7.3 KB | +0.02% | +0.03% |
| raw.str_slice | 7.0 KB | +0.33% | +0.33% |
| prelude.llrt_bytes_slice_at | 14.3 KB | +0.34% | +0.35% |
| tasks.h_release | 3.3 KB | +0.09% | +0.27% |
| cbyte.bytes | 3.1 KB | -0.06% | -0.03% |
| pool.unlock | 2.2 KB | +0.09% | +0.11% |
| nistec.add4p | 4.1 KB | +1.62% | +1.77% |
| raw.data | 3.2 KB | +0.72% | +0.82% |
| http1.fail | 1.1 KB | +0.97% | +1.04% |
| tlsio.load | -6.3 KB (grows) | -0.13% | -0.12% |

Many copies cost text for nothing; a few are hot. A rule must tell them
apart from what it sees statically.

## Variants (TLS client; Mac)

| variant | rule for a callee over the limits with 2 sites | text | x25519 | p256 | notes |
|---|---|---|---|---|---|
| A | not inlined (1 site still is) | -3.3% | +0.22% | +0.13% | corpus: sortpt +3.2%, stack +3.5%, grid +1.5% |
| B | inlined at loop sites, in the loop budget | -3.7% | +0.78% | +1.0% | |
| C | inlined at loop sites, own pass | -3.7% | +0.85% | +0.6% | |
| D | C, callers count the callee as a call | -1.4% | +0.65% | +0.6% | |
| E | D, up to 100 instructions (larger: not inlined) | -3.1% | +0.12% | +0.33% | Linux: maxby +1.2%, iface +0.8% |
| F | E, but a callee over only the block limit (instructions within 40) stays inlined at both sites | -2.8% | +0.21% | +0.31% | landed |

F's refinement came from per function callgrind of the HTTP server
under E: the callees that lost were over only the block limit (joins of
their tests, 40 instructions or fewer): libcstr.memcmp (37, 26 blocks),
xo.map.scan (27, 13), State.email_taken (31, 15).

## F, measured

- Text: TLS client 1,653,816 to 1,607,652 bytes (-2.8%); HTTP server
  (bench/perf/http) 5.7 MB to 5.5 MB; compiler -0.6%; the self build's
  time unchanged.
- HTTP callgrind (Docker linux/arm64, 3 reps; the counts spread about
  ±150 a request that day): plain 31,525 to 31,467, tls 88,266 to
  88,342: parity.
- bench/perf/xo, Linux cachegrind (deterministic): get -3.0%, hof -0.62%,
  maxby +0.11%, iface +0.79%, the rest within ±0.03%.
- TLS handshakes (Mac, 2 runs): x25519 +0.21%, p256 +0.31%.

## Open

iface +0.79% is exactly one instruction an element (2,000,000). Its hot
functions (iface_loop, total_area[Rect]) disassemble identically under
both rules, and denying in the base build each callee F stops inlining
moves nothing, so the instruction is elsewhere. Not found yet.
