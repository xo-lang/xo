# 0620: Pack a struct's Bool fields into shared words on the native backends

Status: Proposed (2026-11-12, Xo Lang 2; stub, the lead asked for it with
the measured gain; the owner decides)

## Context

On the native backends (arm64, amd64) and LLVM, every struct field takes at
least one 8 byte word (compiler/ssa cost.xo `type_words`, compiler/arm64
gen.xo's layout: Bool, Int, List and Map 1 word, Str 2). A Bool field is
one word for one bit. A list of structs stores each element's words inline
(decision 0369), so a struct with many Bools makes every element of every
list of it, and every copy of it, larger.

The REPL's warm session (bench/results/browser-repl-page-2026-11-12.md,
"The warm session by owner") is 43% std's object list, and the
checker's `Obj` has 23 Bools among about 82 words (28% of every object).
In std, 96 structs have two or more Bool fields (329 Bool words); in
compiler/, 106 (521). Those with the most are per connection or per parse
state: yaml's Emitter (15 Bools of 40 fields), http1's H2X (13 of 37) and
Head (7 of 18), x509's Cert (9 of 53), tls13's Server (8 of 38).

## Decision (proposed)

The native backends' layout packs a struct's Bool fields into shared words
(64 to a word, after its other fields), read with a mask and written with
a read-modify-write of the word. Field order in the source does not
change; the Go backend keeps Go's own layout. `Option[Bool]` and Bools
inside enum payloads are not packed in the first step.

## Measured gain

The hand version on the checker's `Obj` alone (decision 0583 work, branch
l2-objflags2: 23 Bools in one `ObjFlags` word, 82 to 60 words an object)
measures what packing one Bool-heavy struct gives:
- the REPL's compiler peak (scripts/browser-memory.sh): PENDING;
- xo's self build, linux/arm64 `xo bench --ab`, wall and RSS: PENDING.

The language version applies the same packing to all of those structs
without hand-written accessors. Its gain on std-heavy programs (the HTTP
server bench/perf/http, JSON decode bench/perf/jsontoken, yaml, x509 pool
loads) needs the prototype: a compiler/arm64 layout behind
XO_PACK_BOOLS=1, then `xo bench --ab` with RSS on the linux/arm64 host.

## Consequences

- Every native backend's struct layout changes, so does the ABI between a
  program and its separately compiled std half, and every cache keyed on
  layout: the std caches' keys change once.
- `raw` field access, `unsafe` offsets, DWARF variable locations
  (decision 0252) and `xo dbg` must know a Bool's bit.
- A write is a read-modify-write of a shared word. Two tasks writing two
  Bools of one struct value at once would race where two separate words
  did not. Structs have value semantics (spec/core.md: every user
  struct, enum and tuple), and tasks share mutable state only through a
  Mutex or similar, so this matters only for raw shared memory.
- Each Bool read gains an AND; each write a load, an OR or AND, and a
  store.

## Alternatives

- Hand-pack only the hot structs (this ADR's measurement on Obj): no
  language change, but each struct needs accessors and every user
  rewritten (Obj: 23 fields, about 450 references).
- Pack to bytes, not bits: simpler masks, 8 Bools a word.
- Do nothing: the cost stays one word per Bool field.
