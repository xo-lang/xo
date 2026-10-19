# 0272: A warm standard library cache with seeds and releases

Status: Proposed (owner, 2026-10-19, through the lead; Xo Lang 3). Step 1
implemented (19e374ec).

## Context

Owner request: ship the standard library's build cache entries with
seeds and releases, so a user's first build of a program costs about
what an edit build costs.

Measured first (2026-10-19, the Mac, main 990ef859's xo, arm64, a fresh
XO_BACKEND_CACHE_DIR and XO_BUILD_DIR per build, two rounds, user CPU):

| program | cold | after another program's build | edit build |
|---|---|---|---|
| hello | 0.20 to 0.24 s | 0.18 to 0.22 s | 0.18 to 0.19 s |
| the HTTP CRUD server (2,484 functions) | 4.39 to 4.50 s | 4.28 to 4.49 s | 1.20 to 1.22 s |
| examples/notes-service (1,793) | 2.64 to 2.89 s | 2.81 to 2.90 s | 0.84 to 0.89 s |

hello is below the 256 function split, where no cache applies. For the
others an edit build costs about a quarter of a cold one, but a cache
another program filled (what a shipped std cache would be) saved only
the reference counting phase: the CRUD server reused 1,511 of 2,484
reference counting entries, and 0 of 2,330 front end bodies, 0 of 2,484
inline entries, and 0 of 1,209 functions.

Why, per key (a scratch xo that dumped each function key's text, and
the code):

- Front end (compiler/ssa fecache.xo): the flags hold the program's
  directory (`root=`); the standard library's part of every key hashes
  the std, prelude, and native modules this program loads (std_sum,
  shared with the checker's keys, because the import walk misses the
  prelude and the native runtime); and an entry replays only into the
  builder state it was recorded in (fe_state: every name built so far
  and the table sizes).
- Inline (compiler/arm64 inlcache.xo): the key held the function's own
  index and each reachable callee's.
- Function (compiler/arm64 fcache.xo): of the 1,536 functions both
  programs have, every key differed, and in 1,475 the only differing
  input was the inline key the function key embeds; the SSA after
  inlining, the types, and the target were the same text.

## Decision

1. The inline key names each callee and its place before or after the
   function (`<`, `>`, `=`, `-` for none) instead of indices: the inliner
   reads a callee's index only to fetch its body as it is at that moment
   (inline_one's `m.funcs[ci]`), which the key already describes by the
   version it picks (the callee's inline key when it comes before, its
   reference counting key after, "self" for itself), and its facts
   (indirect, catches, result count) are in the key from its summary.
   The cache version is v2.
2. The front end keys and entries become program independent with
   decision 0271's relocatable body entries (Xo lang 5): the root only
   for modules outside std, the std part the body module's own import
   closure plus the prelude and native closures, and entries that name
   the types and functions they make, interned by a serial commit step,
   instead of the fe_state fingerprint. The shipped cache builds on that
   format.
3. The shipped cache itself (which entries, how the key stays valid
   across releases, where it lives in the archive, how xo reads it read
   only, its size per target, seed-cut.sh and disttar): designed after 2.

## Consequences

Step 1 measured (the same setup, ik's xo): the CRUD server after a
notes-service build reuses 1,279 of 2,484 inline entries and 495 of
1,209 functions (both 0 before); inline 2.25 s to about 0.5 s, emit
1.03 to 0.37 s, user CPU 4.48 to 3.81 to 3.86 s. The gate: -short,
internal/arm64, the fixed point, arm64-identity.sh on arm64 and amd64
(402 compared, 0 differing each), and XO_BACKEND_CACHE=verify (which
fails on any entry that differs from a recomputation) for notes-service
then the CRUD server, the reverse, and the compiler's self build: all
pass. Old inline entries are not read (v2).

## Alternatives

- Ship today's entries: saves only reference counting (about 1.4 s of
  CPU of the CRUD server's 4.4).
- Prebuilt std objects (a std library linked as an archive): the native
  backends compile the whole program at once (inlining across modules,
  dead function removal), so a separately compiled std would give up
  that code quality; the cache keeps whole program compilation and
  reuses work only where it is provably the same.
