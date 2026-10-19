# 0272: A warm standard library cache with seeds and releases

Status: Proposed (owner, 2026-10-19, through the lead; Xo Lang 3)

## Context

Owner request: ship the standard library's build cache entries with
seeds and releases, so a user's first build of a program costs about
what an edit build costs. Measured first (2026-10-19, the Mac, main
990ef859): a cold build of the HTTP CRUD server takes 4.4 s of CPU and
an edit build 1.2 s; a cache filled by building another program
(notes-service) saved nothing but the reference counting phase, because
the front end, inline, and function keys of std's functions depend on
the whole program.

## Decision

(Draft; filled in as the steps land.)

1. The inline cache key names callees and their order relative to the
   function instead of function indices (compiler/arm64 inlcache.xo).
2. The front end's program independent keys and relocatable entries come
   with decision 0271 (Xo lang 5); the shipped cache builds on them.
3. The shipped cache: which entries, the key's validity, where it lives
   in the archive, how xo reads it (read only), its size per target.

## Consequences

To be written.

## Alternatives

To be written.
