# Go map iteration order is random

Status: Observed (2026-10-07, P1 runtime)

Xo `Map` and `Set` iterate in insertion order (determinism matters for replay
and tests), so they are not Go maps. Since 2026-10-08 `std/rt/map.go` is
an append only entry log plus an open addressing hash index (see
`map-copy-claims-log.md`); each entry records the log position that gives
its key its place (its origin), iteration walks the log and yields each
live key at its origin with its newest value, and the log is flattened when
it holds more garbage than live keys. `put` on an existing key keeps its
position; remove then put moves the key to the end. (Before 2026-10-08 it
was a persistent hash array mapped trie plus a List of keys.) Golden tests: `collections`, `hash_keys`.

JSON objects are parsed with the token API so a decoded `Map[Str, V]` keeps
the key order of the text (`encoding/json` into `map[string]any` loses it).
