# 0454: XO_THP=off, MADV_NOHUGEPAGE on the heap's mappings

Status: Accepted (owner, 2026-11-02: option 3 now, a default that turns huge pages off for servers as the goal; proposed by Xo Lang 3)

## Context

On a Linux host with transparent huge pages `always` (both offload hosts,
defrag `madvise`), the kernel backs a heap's touched 2 MiB regions with
huge pages, so a sparse heap is resident in whole 2 MiB steps. Measured
2026-11-01/02 on the arm64 offload hosts (4 interleaved rounds, scratchpad
thp.out; experiment branch hkq, madvise after both heap mmaps):

| 2,000 held connections, RssAnon | default | no huge pages | Go |
|---|---|---|---|
| TLS | 95 to 108 MB | 40 to 56 MB | 79 to 83 MB |
| plain | 55 to 94 MB | 21 to 24 MB | 14 to 15 MB |

HTTP CRUD peak RSS 71 to 99 MB against 21 to 28 MB (Go 19 to 21), requests
per second 149k to 153k against 153k to 156k (Go 61k to 67k). The non-huge
part of a TLS run was a steady 30 MB; AnonHugePages swung 39 to 92 MB, which
was the 37 to 110 MB spread of RssAnon between runs. Lang 2: coll's edit
build 121 to 125 MB against 88 MB with THP off.

The speed cost (`xo bench --ab`, linux/arm64, the parent against the
variant): full variant, self build wall 1.085, CPU 1.075, RSS 0.956, alloc
mixed 1.029; chunk mappings alone cost the same (self CPU 1.066, mixed 1.028)
and large blocks alone cost little (self CPU 1.010, RSS 0.969, mixed 1.0).
The huge pages on the small object chunks save TLB misses and faults, which a
compute heavy program like the self build uses.

## Decision

`XO_THP=off` (read at start, entry.xo; heapos.xo thp_off) makes every Linux
mapping the heap makes (map_os, map_aligned) advise MADV_NOHUGEPAGE. The
default is unchanged: no env, no advice. Other values do nothing. A service on
a THP=always host sets it to hold its working set to the small page footprint.

## Consequences

- Servers and builds can cut RSS by half or more on THP=always hosts at the
  price in the table above for compute heavy programs; the choice is theirs.
- The goal (owner): a default that turns huge pages off for servers, for
  example from the first accepted connection, covering memory already mapped
  (prototype and numbers to come back to the owner).
- A mapping made before the heaps exist (their own records) is not advised.
- Why Go stays low (bench/results/thp-density-2026-11-02.md, 2026-11-02):
  at 2,000 TLS connections Xo's touched 2 MiB regions are 19 to 23% full
  (each thread's own 4 MiB chunks use about 260 KiB), Go's 71 to 75%; the
  pagemap prediction from XO_THP=off's layout, 96 to 111 MB, matches the
  default's measured 95 to 108 MB.
- XO_THP=off still left 12 to 14 MB of huge pages on the always host. Open
  sources: the first heap record's mappings (made before entry.xo reads
  XO_THP), thread stacks, the TLS block and cabi_nolibc's no-reserve stack.
  Follow-up: advise those too.
- Windows, Darwin and Wasi are unaffected.

## Alternatives

- Always advise: 7.5% self build CPU. Rejected as a default.
- PR_SET_THP_DISABLE at start: process wide (stacks too), not undoable per
  mapping. Not chosen.
- Leave it to the administrator (THP=madvise): documented, but a deployed
  service does not control the host.
