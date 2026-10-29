# 0409: an edit's store link reuses std's laid-out half

Status: Proposed (Xo Lang 4, 2026-10-29; ceiling check first)

## Context

A separately compiled darwin edit (decision 0388) links the program's objects against std's
store. After cfd303004 (the DWARF cut), 225fb8410 (def_of memo) and e7675ecb0 (the prepare
cache) the HTTP server's edit link is 23.7 ms (fib 9.1): prepare 8.4, resolve 4.4, finish 10.9.
The in-flight items (parallel page hashes, eh_live's index, one-lookup add_globals) are worth
about 3 ms together. A line profile of layout and synthesize shows no line above 0.5% of the
link. The 0.35 target is an HTTP edit link under 15 ms, so the remaining time has to come from
not doing std's share of the link at all on an edit.

On an edit of a function body, the prepare cache hits: std's live set, its kept atoms, their
order and sizes are the same as the previous link's. Only the program's objects change. Yet
every link rebuilds std's objects, lays out std's sections, applies std's internal
relocations, synthesizes std's stubs, GOT entries, unwind info and DWARF, and writes them.

## Decision (proposed)

On a prepare cache hit, the link reuses std's laid-out half from the previous link with the same
key:

1. Layout order: std's sections first in each output section (__TEXT,__text: std then the
   program; likewise data), so std's addresses do not depend on the program's sizes. The
   program's sections follow at an address the cached half records. Only the first link of a key
   (a miss) lays out std.
2. The cached half (in <backend cache>/stdhalf/<prep key + layout inputs>): each std output
   section's bytes with every std-to-std relocation applied, the std symbols the program may
   reference (name, address), the relocations in std that name a program symbol (main, the
   callbacks a program defines and std calls; few), std's share of the GOT, stubs, chained
   fixups, unwind entries and __XODW tables, and the layout's end addresses.
3. A hit link: parse the program's objects, place them after std's half, resolve the program's
   references against the cached symbol table, apply the program's relocations and the few
   std-to-program ones, append the program's GOT, stub, unwind and DWARF entries, and write the
   load commands, __LINKEDIT and signature.
4. Anything the cached half cannot answer (a new import std needs, a weak definition both define,
   a section kind std's half has no room for) falls back to the full link: the output is then the
   full link's, byte for byte.

The output must not depend on whether the half came from the cache: proof by identity with the
half cache off, miss and hit, as for the prepare cache.

## Ceiling check (before any of the above)

In a never-pushed branch, time an HTTP and a fib edit link where std's half is taken as given:
- XO_CEIL=half: the link runs as today, but skips store_prepare's std objects past the first
  link of the process (XO_STD_STORE_REPEAT keeps the first link's std ISects, layout and
  synthesized std parts in memory and reuses them), with the program's relocations still
  applied. Wrong output is fine; the timing bounds the gain.
- Interleaved n=80 against the same binary without it, plus the one-binary A/A control.
- Go on only if the ceiling puts the HTTP edit link at or under 15 ms with margin for the cache's
  own read (a std half is roughly the image size: ~6.8 MB for HTTP, a read of about 1 ms warm).

## Consequences

- An edit's link becomes proportional to the program, not to std: the expected HTTP edit link
  is the program's share plus __LINKEDIT and the signature (an estimate to confirm by the
  ceiling: 8 to 12 ms).
- Layout changes: std's sections first. Identity against the parent changes (addresses move)
  in separately compiled builds, once; whole builds are untouched.
- A new cache kind (stdhalf) in the back-end cache, keyed by the prepare key plus the layout
  inputs (page size, section order, opts), pruned with it, classified in
  TestCodegenInputsClassified if it adds an XO_* switch.
- More code in compiler/macho: the half's encoding and the hit path, with a full-link fallback
  for anything unusual.

## Alternatives

- Keep shaving the full link (the in-flight items): about 20 ms at best, short of the target.
- Prelink std as a dylib: changes the binary's shape (a second image, dyld load cost at every
  start); decision 0388 chose a static link.
- Run the link in a resident process (xo watch) with std's half in memory: the same reuse
  without a file, but only for watch users; the cache serves every build. The two compose
  later.
