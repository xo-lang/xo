# 0237: Static words: 256 process words, two steps around a seed

Status: Accepted (lead, 2026-10-16). Step 1 implemented 2026-10-16;
step 2 (std using words past 63) after the next seed.

## Context

rt.static_word(k) (decision 0207, work item B02) is the address of word
k of a table of process globals, zero at start: one adrp/add (arm64), a
constant GEP (LLVM), or a constant address (wasm), with no load before
the word itself. The table had 64 words and all were claimed in
std/native/rt.xo's table, which blocked new users (the lazy P-521 and
P-384 fixed-base tables) and pushed several fields into odd homes.

## Decision

1. The table grows to 256 words (2 KiB) on every backend: selfhost/ssa
   accepts a constant 0 to 255, the arm64 emitter writes 256 `.quad 0` (selfhost/macho has no `.zero`)
   (an add immediate of 8 * k still fits, 2040 at most), the LLVM
   emitter a weak `[256 x i64]`, the wasm emitter a 256 word static
   table. The cost per access is unchanged.
2. Two steps around a seed: the 0.16.0 seed's SSA builder, which builds
   xo from this std, rejects a word past 63, so std claims only 0 to 63
   until a seed whose builder accepts more; internal/repotest
   TestStaticWords enforces it (seedWords = 64; raise it to 256 then).
3. Words that no code uses any more are freed now: 12 and 13 (obj.xo's
   C function addresses, gone with the C runtime) and 44 to 47 (poll.xo's
   words for the C's park_signal and services record; its event loop
   record moves from 45 to 43 to keep one range): six free words before
   the seed, listed as free in rt.xo's table.

## Consequences

- New process globals cost nothing per access and need no start-up
  allocation; after the next seed there are 192 more words.
- Until then, six words are free (12-13, 44-47).

## Alternatives

- One services-style record (one static word pointing to a record made
  at start, fields by offset): every access pays a load of the record's
  address first, and the record must exist before its first user runs
  (the heap and fault setup run before anything else); growing the
  table costs nothing.
- Moving cold users into ctx.xo's services record now: churn in
  working code for words the seed step frees anyway.
