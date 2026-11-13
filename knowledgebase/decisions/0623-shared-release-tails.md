# 0623: Shared release tails for nested error exits

Status: Accepted (lead, 2026-11-12: an internal codegen change with no
semantics change, recorded with its numbers; Xo Lang 3).

## Context

aws-43's gcp models (decision 0013 there) grew bigquery's sample from
4.44 to 5.43 MB of __text. With every derived decoder kept out of the
inliner (XO_INLINE_DENY='xo.json.dec*') the growth stays (-28 KB of
996 KB), so inlining is not the cause. Each field of a derived Json
decoder has two error returns (a duplicate key, the member itself), and
the reference counting pass puts, on each, a release of every field
value decoded before it: O(n^2) releases. A 60 field struct of Str
fields has a 65 KB decoder; the releases between fields grow 0, 0, 1,
1, ..., 14, 14 in its disassembly. Notes:
~/xo-lead/notes/lang3/jsondec-size-2026-11-12.md.

Fixes in the JSON generator alone do not work: a struct accumulator is
flattened (every error releases all n slots); an error ladder of
blocks loses its carried values (unused block parameters are removed
before the RC pass, so the releases stay on the edges).

## Decision (proposed)

1. A pass at the end of insert_rc (compiler/ssa/reltail.xo,
   share_release_tails; XO_PASSES=-reltail turns it off) groups the
   exits that end at one place (a return, or a jump to one block) and
   holds, of each exit's releases, the values another exit of the group
   also releases. Sorted by set size, each exit jumps to a tail that
   releases what its set adds to the largest tail below it and jumps on
   to that tail (or the exit's target). Each shared value is released in
   one place. A tail's values are released by every exit reaching it, so
   their definitions dominate it.
2. An exit is left alone when, after its first moved release, it holds
   an operation that can fault (a self-faulting op, or a call of
   anything but a runtime `rt.` function: a user callee's faults are the
   module's table, which the pass does not see), or when its terminator
   passes a value that may be immortal (a constant, a literal
   aggregate, a parameter): insert_rc proves immortality through only 24
   parameter hops (imm_at), and a ladder adds one a tail.
3. Derived Json decoders (compiler/ssa json.xo dec_fields_in) send every
   member's error to one error block per struct, with the segment as two
   Int offsets into one constant (prelude llrt_json_path_push_at), so a
   field's error exit is a run of releases and a jump.

## Results

darwin/arm64 __text, every build with XO_INLINE_STRICT=1 (a module that
fails to verify otherwise builds without inlining and looks smaller):

Every row is a whole-program link: a split link against the prebuilt std
store puts the program's own code in _ptext (ELF .text.xoprog) as well,
but these xo builds had no store, so _ptext was 0 and __text is all the
code. Re-measured on 2026-11-12 as __text + _ptext, with xo at the
parent (5910cb428) and at 267e8bdb4, gcp 991353d: calendar 2,107,744
to 2,011,240 (-4.6%), bigquery 5,449,972 to 4,815,172 (-11.6%), aws
presign 2,414,996 to 2,412,996. Those are the table's changes; the
absolute sizes drifted with main between the two runs.

| binary | before | after | change |
|---|---|---|---|
| gcp calendar sample (aws-43's kit, gcp 991353d) | 2,095,656 | 1,998,912 | -4.6% |
| gcp bigquery sample | 5,434,292 | 4,799,276 | -11.7% |
| aws presign sample | 2,395,748 | 2,393,420 | -0.1% |
| xo itself (the pass off against on, same sources) | 42,535,628 | 42,001,668 | -1.3% |
| a 60 Str field struct's dec[T] | 65,296 | 22,704 | -65% |

The bigquery growth of aws-43's 0013 models (from 4,437,856) falls from
+996 KB to +361 KB.

Speed (linux/arm64 host 1, `xo bench --ab`, 3 rounds):
- xo's self build (stage2 self, base 3d7578e84): cpu 0.977 (0.977 to
  0.997), max RSS 0.968: better.
- bench/perf/json `invalid` (base c3637aae0, the change's parent): wall
  0.952 (0.926 to 0.98), better; `small`: wall 1.008 (1.008 to 1.012).
- Callgrind (linux/arm64 in Docker, same parent): `small` 241,188,143
  to 240,793,143 instructions (-0.16%), `invalid` 171,953,571 to
  171,958,101 (+0.003%): the `small` wall difference is not more
  instructions (13 ms runs; layout).

Correctness: arm64 TestGoldenRun (all), xo dev codegen arm64 and llvm
(334 patterns), llvm json and derive goldens, wasm json goldens, the
json, jsontoken, cbor, yaml and protojson corpora on go, arm64 and llvm,
xo unit tests (with reltail_test.xo), the amd64 sample on the linux/amd64
host (52 ok), and the offload gate with the fixed point.

## Consequences

- Error paths with many live values are smaller everywhere (xo -534 KB);
  the error path runs a few more jumps, the success path is unchanged.
- An exit with a faulting call after its releases, or a constant passed
  on, keeps its releases; a decoder generator that wants the gain keeps
  calls off its error exits (json.xo does).
- Every dump of an affected function shows the tails as new blocks at
  the end.

## Alternatives

- Generator-only fixes (above): no effect.
- A heap scratch object for the decoded values: one allocation per
  decode.
- A runtime helper that releases a list of slots: still O(n^2) data.
