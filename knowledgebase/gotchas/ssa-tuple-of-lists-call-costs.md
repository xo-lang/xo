# Moving a hot path into a generated function that returns a tuple of lists costs instructions and memory

Status: Observed (2026-10-24, decision 0372 stage A).

Symptom: putting the lists Map's put into one shared function per key
type (`xo.map.putk[K]`, returning `(keys, live, slots, n, index, state,
old live)`) made bench/perf/xo putget 1.37 times main's instructions
(+52 a put). Building a 50k entry map cost +62 instructions an insert.
A 200k entry map's peak RSS was 1.15x (Str keys) to 1.2x (Int keys).
Wall time was within noise in one round of runs, so only instruction
counts and max RSS showed it.

Cause:
- The tuple of lists goes through memory, and each list in it is
  counted on the way out and back.
- The caller still held the map (it read the values list and the log
  field after the call), so the map's old key lists stayed alive through
  the callee's rebuild.
- An old live flags list was returned only so the values could be
  compacted after the rebuild, so it lived through the rebuild too.

Fix:
- Keep the hot paths (the key already there, room in the table) in line
  in the per-type function, and call the shared one only on the rare
  path (growth).
- Read every field of the struct before the call, so its last use is
  before it.
- Compact the values first (by the old flags), then call the rebuild,
  so nothing old is returned.
- Check every such change with `/usr/bin/time -l` instructions and max
  RSS against the parent's xo before any quiet window.
