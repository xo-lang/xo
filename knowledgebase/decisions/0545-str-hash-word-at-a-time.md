# 0545: Str keys hashed a word at a time

Status: Accepted (the owner, 2026-11-07: (a) with the seed; Xo Lang 4).
Landed as l4-strhash-final's (a) v2 with the allocation-free seed; (b)
became decision 0552, accepted the same day.

## Context

Decision 0072 hashes a `Str` Map key or Set element with FNV-1a over its
bytes and a final mix: `ints.str_hash` in std/native/llrt/ints.xo for
the native backends (arm64 and LLVM, when the native prelude provides
the sized integer helpers; otherwise `rt.str_hash`), and `$str_hash` in
compiler/wasm/rt.xo for wasm. FNV-1a does one multiply per byte, each
depending on the last, so a 64 byte key (every compiler cache key is 64
hex digits) is 64 serial multiplies; the native version reads a word at
a time but still mixes byte by byte.

A 10 kHz profile of 20 warm whole builds of http_server_request on the
linux/arm64 host (main e5ca9d617, 2026-11-07; ADR 0529 step 3b):
`ints.str_hash` was 6.8% of the CPU (1.10 s of 16.3 s), and a further
4.0% was `xo.map.rebuildk[Str]`, which rehashes every live key each time
a map grows (about 1.6 points of it in the hash). The compiler is a
large user of Str keyed maps, and so are the programs Xo is for: HTTP
headers, routes, JSON objects.

## Decision

(a) first, (b) after, each measured on its own:

(a) **A word at a time.** `str_hash` mixes eight bytes per multiply
(`rt.bytes_word`), with the length folded into the start and no loop over
single bytes: a key under 8 bytes is one word of fixed loads (4 to 7
bytes: its first four and its last four; 1 to 3: its first, middle and
last), a longer key's 1 to 7 byte tail is its last 8 bytes shifted down
past the ones already mixed; then decision 0072's final mix:

    h = C0 ^ (len << 56)
    for each 8 byte word w:  h = (h ^ w) * K;  h ^= h >> 32
    a short key or the tail: the same, once
    final mix (as today):    h ^= h >> 31;  h *= K;  h ^= h >> 29

(C0 = the FNV offset basis, K = 0x9E3779B97F4A7C15, the constant today's
final mix already uses). The same function replaces compiler/wasm/rt.xo
`$str_hash`, so the backends stay alike (no value has to match between
them; see below).

(b) **The hash kept with each entry**, as Go's maps keep a hash byte: a
map's growth (`xo.map.rebuildk`) moves entries by their kept hash
instead of rehashing every key, and a lookup compares hashes before
`str_eq`. Sketch: no new list; a slot (an Int holding the entry's index
plus one) keeps the index in its low 32 bits and the hash's low 32 bits
in its high ones. A lookup then compares the slot's hash bits with the
key's before it reads the live flag and the key (a collision costs no
`str_eq`), and a growth reads each live entry's hash from the old table
(one pass over the old slots fills a hash per index) instead of hashing
the key again, for key types whose hash costs more than that pass (Str,
structs; an Int key's hash is one multiply). Only compiler/ssa reads
slots (map.xo, mapcore.xo, maplog.xo, maphyb.xo); the backends see
lists of Int, so the step is one front of the compiler for every native
backend and wasm. A map past 2^32 - 1 entries keeps today's slot (a
flag bit says the hash bits are absent). Its own step, after (a)'s
numbers. Its likely gain is small: rebuildk was 4.0% of the profile and
about 1.6 points of that was the hash, which (a) roughly halves for 64
byte keys, so (b) saves about 1% of a build at most. Against that, it
touches every slot reader (insert_slot, map_find, the removals and
decision 0291's log map: about a hundred sites in map.xo, maplog.xo and
maphyb.xo). It is prototyped only if a profile after (a) still shows
rebuildk's hashing above 1%.

**Seeding (hash flooding).** Str keyed maps hold untrusted input
(header names, JSON object keys, form fields). Neither today's FNV-1a
nor (a) is keyed: anyone can compute colliding keys offline and make
each insert walk a long probe chain (quadratic time for n keys). Go
seeds its map hash per process (and per map) for this reason. Proposed
with (a): a per-process seed, read once at start from the operating
system's generator (the one std/native/llrt/rand.xo already reads:
getrandom on Linux, getentropy on darwin, BCryptGenRandom on Windows),
folded into C0; the cost is one load of a global per hash, measured
before it lands. `XO_HASH_SEED=N` fixes it for reproducing a run
(tests, profiles, callgrind, whose instruction counts would otherwise
vary a little from run to run with the probe lengths). A keyed hash of
this shape resists casual flooding, not an attacker who can observe
timing at scale; the owner decides whether that is enough or a keyed
hash with a stronger mix (wyhash's 64x64 to 128 bit multiply,
`cpu.mul_hi`) is wanted, and whether the seed is part of (a) or a step
of its own.

**Every backend.** The four backends hash a Str key as follows, and
with this decision they all hash it a word at a time and seeded:

- **arm64 and amd64 (native)** and **LLVM** call the same function:
  compiler/ssa map.xo lowers a Str key's hash to the native prelude's
  `llrt_str_hash`, which is `ints.str_hash` in std/native/llrt (the
  `rt.str_hash` arm is for a prelude without the sized integer helpers,
  which no backend builds today). One change covers all three, seed
  included (entry.xo start, which every native and LLVM program runs).
- **wasm** has its own `$str_hash` in compiler/wasm/rt.xo, the same
  algorithm written in WAT (the prototype's wasm port passes the map,
  set, str and json goldens). The prototype does not seed it yet: the
  wasm runtime's start reads 8 bytes from WASI `random_get` (already
  imported for rand, rt2.xo) into a global that `$str_hash` folds into
  C0, with the same XO_HASH_SEED override. That lands with the native
  seed, so no backend is left unkeyed.
- **Go** (std/rt) uses Go's own maps, whose hash Go already seeds per
  process and per map (aeshash or memhash with a random seed); nothing
  changes there, and it already resists precomputed collisions.

The hash values differ between backends (word reads differ in wasm's
fixed loads, and the seeds are random per process), which is fine:
no value is stored, compared across processes or visible to a program
(below).

## Where a hash value could leak (none found)

- **Iteration order.** Map and Set iterate in insertion order (decision
  0072's keys list; decision 0291's log map by each entry's `org`), not
  by slot, so no program output depends on hash values. scripts/identity
  checks the compiler side: an xo built with (a) must build the same
  bytes as one without it.
- **derive Hash.** `Eq` and `Hash` derived for a type make it usable as
  a key; there is no method that returns the value. std's hash digests
  (decision 0279) are unrelated.
- **Stored keys.** Every cache key the compiler writes (front end,
  reference counting, cleanup, inline, parts, std records, packs) is a
  SHA-256; none is built from `str_hash`. The native emitter's own
  short hashes in symbol and part names (compiler/arm64/parallel.xo
  `fnv`) are a separate function, so neither (a) nor the seed can change
  what the compiler writes. A map's slot layout is never
  written to disk and is not exposed to programs.
- **The Go backend** uses Go's maps (std/rt), unchanged.
- **Generated hashes** (`xo.hash[T]` for structs, tuples, lists, enums)
  combine their parts' hashes; a Str part changes the value, and only
  the table reads it.

## Consequences

- Every native and wasm program's Str keyed maps hash faster; the
  compiler's builds most of all (numbers below).
- The runtime's `str_hash` is compiled into every program, so binaries
  differ from the last release's; outputs do not.
- With the seed: a map's slot layout differs from run to run; anything
  that measures instruction counts sets `XO_HASH_SEED`.

## Results

Prototypes on branches, none pushed (2026-11-07, the second linux/arm64
host):

- (a) native, l4-strhash a0f1cf358 against f5af1bf2b: scripts/identity
  311 builds (darwin/arm64 and linux/arm64) 0 differing; internal/arm64
  TestGoldenRun and the LLVM map and set goldens pass.
- (a) wasm, l4-strhash-wasm 438ce3e32: TestSelfhostWasm over the map,
  set, str, strings, json, conc and atomic goldens passes.
- (a) with the seed, l4-strhash-seed 44f5011ca (rt.static_word 100,
  set in entry.xo start from XO_HASH_SEED or rand.system_u64) against
  (a) without it: scripts/identity 310 builds 0 differing (the
  compiler's output does not depend on its maps' seed); TestGoldenRun,
  the LLVM map goldens and TestStaticWords pass.
- Speed, (a) l4-strhash 3fd4c5596 against d2f036886 (main plus
  bench/perf/strkeys), `xo bench --ab` on the first linux/arm64 host,
  wall time B/A: strkeys hex (64 byte keys) 0.455, hdr (HTTP header
  names) 0.926, short (2 to 6 byte keys) 1.047 (worse: the 1 to 7 byte
  tail is a byte loop); the self build 0.995 (cpu 0.994; 2 of 3 rounds
  loaded). A second version with no byte loop (keys under 8 bytes by
  fixed loads, a longer key's tail by an overlapping last word, the
  length folded without a multiply; l4-strhash-v2 52dd7ce5d, wasm
  52c5ae6d3) against the same base: hex 0.472, hdr 0.799, short 1.001
  (within noise): the regression is gone and header names gain more.
  Its first build faulted: `<<` is checked in Xo, so `n << 56` (a key of
  256 bytes) and a byte over 127 shifted by 56 overflowed; the shifts
  are `wrapping_shl` (gotchas/xo-checked-word-arithmetic.md). Gates (the
  second linux/arm64 host): TestSelfhostWasm over the map, set, str,
  strings, conc, atomic and json goldens; LLVM map goldens; the fixed
  point (s2 = s3) and the self build (35 s, 4729 MB).
- The seed's cost, l4-strhash-seed 44f5011ca against (a) 3fd4c5596, the
  same host: strkeys hdr wall 1.024 (1.006 to 1.035, worse), short 1.016
  (within noise); short's max RSS 3768 to 4944 KB (1.31x), a fixed cost of
  reading the system's generator at start (rand.system_u64 and what it
  brings in) that a small program pays. A second version (l4-strhash-seed2
  37b9bdbcf) reads the seed with no allocation, straight into its word:
  Linux's AT_RANDOM bytes from the auxiliary vector (else one getrandom),
  darwin's getentropy; against (a): short's max RSS 3768 KB on both sides,
  wall 1.045 and hdr 1.053, both within noise (n=3, spreads to 4.4%); a
  larger run decides the hash's one extra load.
- The whole proposal, (a) as v2 with the allocation-free seed
  (l4-strhash-final fc6ea208f) against FNV-1a (d2f036886), wall B/A:
  strkeys short 0.942, hex 0.458, hdr 0.875 (v2 alone 0.799: the seed's
  load costs header names about 9%), the self build 0.990 (CPU 0.998,
  within noise). Gate: scripts/identity f5af1bf2b against it, 311 builds
  (darwin/arm64, linux/arm64), 0 differing; internal/arm64 TestGoldenRun;
  the LLVM map goldens; TestSelfhostWasm's map, set, str, json and conc
  goldens; the fixed point (s2 = s3); the self build 35 s, 4686 MB.
- Owner's choices: (a) alone, or (a) with the seed (the 9% on short keys
  above against resistance to precomputed collisions); a stronger keyed
  hash instead; (b) only if a later profile asks for it.

## Alternatives

- **Keep FNV-1a.** 6.8% of a build, and every Str keyed map, for no
  benefit but the status quo.
- **SipHash (Rust's default).** Keyed and flooding resistant, about
  three times slower than (a) per word; the owner may prefer it for
  maps that hold untrusted input.
- **A hardware CRC32 or AES round per word.** Fast, but arm64 and amd64
  differ, and wasm has neither.
- **Presizing maps** (a `Map.with_capacity`): helps only sites that know
  their size and leaves the per-byte hash; a separate, smaller change.
