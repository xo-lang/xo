# Stability soaks, 2026-11-05 to 2026-11-06 (decision 0520 phase 1)

Last verified: 2026-11-06

What: `stability/harness soak` of stability/notes on the arm64 native
backend, linux/arm64 offload host 2, open loop 500 requests/s, with
XO_LEAKCHECK=1 and XO_CRASH_FILE set. Signals: RSS, fds (split into
sockets and files; the client's own sockets as client_socks), latency
(p50/p99/p99.9), heap_live, timers_pending, CPU per request, and
AnonHugePages from smaps_rollup (anon_huge_kib, from dc72da981).

## Verdict

No leak found. Every run: 0 errors, 0 drops, leakcheck 0 live blocks at
exit, no crash report, heap_live without a trend, timers flat.

- fds: the growth is the load client's connection pool (the harness's
  http.Transport kept idle connections forever); the server's socket
  count equals the client's (client_socks) and plateaus. Not a leak.
  The fd alert now reads the second half's slope (dc72da981).
- RSS: grows without a matching live heap: neither connections (flat
  sockets with -client-idle, still 9,994 KiB/hour) nor THP
  (AnonHugePages flat at 2 MiB) is the cause. What grows is the heap's
  free lists: freed blocks whose touched pages stay resident (9.5 MB at
  20 minutes, 11.2 MB at 1 h, chunks unchanged at 14). 6 h (long6h, dc72da981, XO_HEAPSTATS=1, 500 req/s, 10.79M requests, 0 errors): leakcheck 0 live at exit; RSS 25.1 to 41.7 MB, slope 3,540 KiB/h, of which outside huge pages 1,280 KiB/h (8 to 10 MB/h at 1 h, so the growth slows but does not stop within 6 h) and AnonHugePages 6 to 14 MB; heap_live 935 to 1,860 blocks while the client held connections rose 24 to 83 (fds flat over the second half), so the live growth follows the harness's connections, not a leak; p99 1,332 to 611 us; verdict DRIFT.

## Runs

| run | rev | length | requests | RSS warmup end -> end | fds | verdict |
|-----|-----|--------|----------|-----------------------|-----|---------|
| night | 5ad22091e | 2 h | 3,596,490 | 19,520 -> 41,484 KiB | 24 -> 31 | DRIFT (RSS, fds) |
| rerun (0528 + fd split) | 9edc455eb | 2 h | 3,596,924 | 16,804 -> 36,268 KiB | 19 -> 36 sockets, files 9 | DRIFT (RSS, fds) |
| thpoff (XO_THP=off) | 08772c9da | 1 h | 1,797,893 | 3,412 -> 19,168 KiB | app 32, client 31 | DRIFT (RSS, fds) |
| thpon (anon_huge_kib) | dc72da981 | 1 h | 1,797,824 | 8,740 -> 22,172 KiB (AnonHugePages flat 2,048) | 13, flat | DRIFT (RSS) |
| idle30 (-client-idle 30s) | b6ec20423 | 2 h | 3,596,630 | 15,652 -> 31,704 KiB | 3 to 13, flat | DRIFT (RSS, heap_live noise) |

## Findings

1. Hypothesis disproved: the std/sql memory driver's per write table
   copies (removed by 0528, 75093177d) were not the RSS growth; the
   rerun with 0528 grew at the same rate (10,330 KiB/hour against
   10,134).
2. fd growth is the client pool: sockets went 19 -> 36 and stayed at 36
   from t=1810 s; client_socks matched the app's sockets (31 and 32).
   The harness gained -client-idle (b6ec20423) to close idle client
   connections.
3. RSS is the heap's free list retention, not connections: idle30 kept
   sockets flat at 3 to 13 and still grew 9,994 KiB/hour to 31.7 MB in
   plateaus and jumps; at exit (thpoff, 1 h) 10 MB of 19 MB RSS was free
   blocks on class free lists, and the 20 minute run had 9.5 MB of them
   with the same 14 chunks (56 MB mapped): no new mappings, more touched
   pages of free blocks. Classes under 32 KiB never return pages
   (alloc.xo SCAVENGE_FROM, decisions 0383 and 0425); classes from 32 KiB
   keep 4 magazines with their pages.
4. THP is not it: with smaps' AnonHugePages recorded (thpon, 1 h) huge
   pages stayed at 2,048 KiB the whole run while RSS outside them grew
   8,247 KiB/hour. The 30.0 against 19.0 MB at 1 h between two earlier
   runs was run to run variance.

## Follow-up items

- Bounded retention: the 6 h soak (long6h) decides whether the free lists' growth stops; a
  time based scavenger is parked for the owner with that answer.
- Withdrawn: the "0.6 MB per kept-alive connection" reading. A worker sweep (8 to 128) shows
  memory follows stored data, not connections.

## Reproduce

stability/README.md has the harness command; the lead's scripts run it
on host 2 with the run's flags (-client-idle 30s, -env XO_THP=off).
