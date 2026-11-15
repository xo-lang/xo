# 0654: `var` aggregates past the argument registers by reference (native backends)

Status: Accepted (Xo Lang 1, 2026-11-15; extends 0151 and 0564 on the native backends, within the lead's approved std Proven work)

## Context

Decision 0151 passes a struct or enum parameter of WIDE_MIN (256) bytes
or more by address on the native backends, with a home for a `var` one
that its updates write in place (0148's dataflow, byref.xo
compute_held); 0564 made that the one rule for wide values. A `var`
parameter below 256 bytes stayed by value: its words went in as
arguments and came back as results at every call.

The std Proven survey (2026-11-15, linux/arm64 host 1; notes in the lead's
lang1/stdperf-causes.md) found this to be the largest single cause of
the parsers' gap to Go. xml.Dec.getc, a three line method on the
decoder's 20 word state, moved those words through the stack twice
per byte, and 16 stack slots were shuffled per call. Go passes a
pointer (`d *Decoder`).

## Decision

On arm64 and x86-64 (compiler/arm64 byref.xo by_ref_of), a `var`
parameter whose type is a struct, tuple or enum of more than 8 words
(VAR_BY_REF_WORDS, the argument registers) is passed by reference as a
wide one is, with the same home and held dataflow. The plain convention
stays wherever 0151 keeps it: functions called through function values,
entries, functions whose address is taken, `main`, and exported
functions (a `use c` boundary or a C export). Caller and callee decide
from the same facts: the parameter types and `var_params`, which the
std facts carry (ssa stdfacts.xo) and check, so separate compilation
and the std store agree. The function cache version is bumped (v12),
since the convention of cached functions changes.

LLVM is unchanged: 0148 already keeps `var` parameters in memory there.

## Consequences

`xo bench --ab` on linux/arm64 host 1, main (b88f637be) against the
change ((private commit)), 3 rounds, wall time:

| Program | main ms | change ms | change / main | 95% interval |
|---|---|---|---|---|
| bench/perf/xml stream | 238.3 | 99.4 | 0.417 | 0.414 to 0.419 |
| bench/perf/xmlcodec decode | 649.8 | 353.1 | 0.543 | 0.541 to 0.546 |
| bench/perf/htmltok next | 121.6 | 69.2 | 0.569 | 0.568 to 0.573 |
| bench/perf/template render | 256.6 | 140.4 | 0.547 | 0.521 to 0.547 |
| bench/perf/tabwriter pad | 44.2 | 32.8 | 0.741 | 0.735 to 0.746 |
| bench/perf/htmlparse ent | 436.3 | 407.3 | 0.934 | 0.934 to 0.938 |
| bench/perf/regexp match | 54.8 | 54.5 | 0.995 | 0.982 to 1.004 (noise) |
| bench/perf/toml decode | 51.9 | 51.9 | 0.999 | 0.987 to 1.014 (noise) |

Outputs unchanged. Gate (linux/arm64 host 2): repo checks, the arm64
suite, codegen (48 programs, 372 patterns), cross, xounit (619 passed),
the self build (26 s, peak 4101 MB) and the seed build with 0.50.1; the
amd64 sample on the linux/amd64 host (53 ok).

Caches and keys: the function cache's version is bumped (v12). The
part cache keys on the functions' fn keys, and the std store, the part
and every backend cache entry key on XO_CODEGEN_KEY (compiler/xo
bcache.xo cache_id), which byref.xo feeds, so no entry made under the
old convention is read. The 0.50.1 seed builds it (no new intrinsic,
no seed needed).

A callee now reads and writes such a parameter through memory, so a
hot loop over its fields loads where it used registers. The held
dataflow keeps loaded fields across reads as it does for wide values;
no measured case got slower (regexp and toml within noise).

## Alternatives

- Inline the small methods instead (getc is three lines). The inliner
  already declines them for size or landing pads in several modules,
  and every call that stays a call would still copy the words.
- A lower threshold (any `var` aggregate). Values of up to 8 words fit
  the argument registers, where passing them in and out costs moves,
  not memory traffic; kept by value.
- Only the parsers' types. A rule by size covers every module.
