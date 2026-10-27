# 0387: Uuid as two Ints instead of its canonical text

Status: Proposed (Xo Lang 2, 2026-10-27, for the owner; the lead put it in
the morning summary). Changes decision 0282's representation of
`uuid.Uuid`, a public std type.

## Context

Decision 0282 made `uuid.Uuid` a newtype over its canonical text,
`type Uuid struct(Str)`: equality, hashing, order and JSON are the text's,
spec/std.md section 24 documents `u.0` (order with `a.0 < b.0`, store `u.0`
in a `derive Json` struct until newtype JSON lands, `sql.str(u.0)` for a
Postgres uuid column), and std/uuid.xo says the same.

Milestone 0.33.0's criterion 4 asks every std module to time within 1.2x
of Go. bench/perf/uuid against github.com/google/uuid (whose UUID is a
`[16]byte` value) measured, after the table driven text hooks of
0c9d56283 (bench/results/std-go-timing-audit-2026-10-26.md, quiet `xo
bench --ab`, 15 rounds, Xo / Go wall):

| workload | Xo / Go |
|---|---|
| parse (five accepted forms, one bad text) | 1.23 |
| bytes (`u.bytes()` then `uuid.from_bytes`) | 4.24 |
| string (`u.display()`) | 0.35 |
| none (the loop alone) | 0.88 |

`bytes` is the worst row left in std. A CPU profile shows what is left is
allocation: `bytes()` makes 16 bytes, `from_bytes` makes a Result and the
36 byte text, where google/uuid's FromBytes copies into a value array and
allocates nothing. The text representation is also why `string` is 0.35:
display returns the stored text, where Go formats.

## Options

**Break now.** `type Uuid struct { hi: Int, lo: Int }` (private fields)
in 0.33.0; the text only from `display()` / `"${u}"`; `u < v` and
equality on (hi, lo) as unsigned (the bytes' order, which is the text's);
JSON and sql through display and parse.

A throwaway prototype (bench/perf/uuid's workloads against a two Int
struct in plain Xo, a builder a byte, no native hook; the same checksums as
the Go twin), quiet `--ab`, 15 rounds, Xo / Go wall:

| workload | text (today) | two Ints (prototype) |
|---|---|---|
| parse | 1.23 | 1.07 |
| bytes | 4.24 | 2.60 |
| string | 0.35 | 2.17 |
| none | 0.88 | 0.87 |

Estimate, not measured: with the native hooks of 0c9d56283 (uuid_text's
table writer for display, a 16 byte store for bytes) instead of the
prototype's builders, string and bytes should land near parse, about 1.0 to
1.3; bytes still allocates the Bytes it returns, as Go's MarshalBinary does.

Breaks every `u.0` in user code and in the repository (golden uuid_basic,
bench/perf/uuid, parse_edge_formats, gen's go_uuid golden, the corpus), the
order and JSON definitions in spec section 24, and decision 0282's text.

**(a) Keep the text and add accessors.** `u.hi()`, `u.lo()` or
`u.byte(i)` for byte level users. No break; does not move the bytes row
(from_bytes still makes the text).

**(b) Keep everything as it is.** bytes stays at 4.24x, a known row over
the bar with this decision as its path.

**(c) Deprecate first, switch next release** (recommended). In 0.33.0:
`Uuid` gets `u < v` and `u.display()` as the documented ways to order and
print, a warning on `u.0` (a vet rule or XO code naming display()), spec
section 24 and the std change log say the representation changes in the
next minor. In the next release: the two Int representation as in "break
now", with its timings.

## Recommendation

(c). It gets the bytes row under the bar one release later without
breaking code without notice, and costs one deprecation warning now. The
owner may prefer "break now" before 1.0 (0.x allows it); then this is
about a day: the representation, the four lookups, display through
uuid_text, the spec section, the goldens and the corpus.

## Consequences

- (c) or break now: `Uuid` stops being a Str; code that stored `u.0`
  keeps working through `u.display()`; JSON keeps the text form.
- Equality and hashing become two word compares: maps keyed by Uuid get
  cheaper.
- display allocates the text on each call (Go's String does too); code that
  prints a Uuid in a loop pays what parse saves.
