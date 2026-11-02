# Why Go stays low under THP=always: heap density (2026-11-02)

Question (decision 0454 follow-up, item (d) from Xo Lang 3's THP queue): with
transparent huge pages `always`, 2,000 held idle TLS connections cost Xo
95-108 MB RssAnon and Go 79-83 MB, while Xo with `XO_THP=off` held 40-56 MB.

Method: bench/perf/idleconn (Xo server, Go server, Go client) at main
9b4db4a2c, cross-built on the Mac; 2,000 TLS connections held 25 s, and 15 s
in, `density.py` reads the server's /proc/PID/pagemap present bits over its
private writable anonymous mappings. For each 2 MiB aligned region inside a
mapping it counts present 4 KiB pages. Under `always` a touched region
inside its mapping faults in as one huge page, so touched regions x 2 MiB
plus the pages outside aligned regions predict the RSS under `always`.
Scripts and logs: thp-density-2026-11-02/ (density.py, thp-job.sh, three logs).

## linux/amd64 (the WSL host, THP madvise: no heap huge pages, so the small page layout)

| | RssAnon | touched 2 MiB regions | mean fill | predicted under always |
|---|---|---|---|---|
| xo | 32-35 MB | 69-89 | 19-23% | 151-190 MB |
| xo XO_THP=off | 31-33 MB | 54-72 | 22-28% | 118-157 MB |
| go | 75-77 MB | 50-51 | 71-75% | 101-104 MB |

By mapping (one round): Xo has 39 separate 4 MiB chunk mappings, each with
exactly one touched region at 13% fill (about 260 KiB), plus 158 larger
merged anonymous mappings holding 14 MB over 50 regions at 15%; 31 threads.
Go has 4 heap mappings holding 72 MB over 50 regions at 72%.

## linux/arm64 (second offload host, THP always, defrag madvise; it hands out few huge pages now)

| | RssAnon | AnonHugePages | touched regions | mean fill | predicted under always |
|---|---|---|---|---|---|
| xo | 65-71 MB | 34-38 MB | 38-41 | 83-85% | 88-94 MB |
| xo XO_THP=off | 44-45 MB | 12-14 MB | 41-48 | 46-53% | 96-111 MB |
| go | 80-81 MB | 70-74 MB | 44-45 | 88-89% | 88-91 MB |

The prediction from Xo's small page layout (96-111 MB) matches Lang 3's
measured default under `always` on this host (95-108 MB, 2026-11-01).
Today's default got only part of its regions as huge pages (34-38 MB), so
it read 65-71 MB. `XO_THP=off` still left 12-14 MB of huge pages: mappings
the knob does not advise (the first heap record's mappings come before
entry.xo reads XO_THP; thread stacks, the TLS block and the no-reserve
stack mapping of cabi_nolibc.xo are never advised).

## Reading

Go does not avoid huge pages (go1.26.1 runtime: sysMapOS advises
MADV_NOHUGEPAGE only with GODEBUG=disablethp). It stays low because its heap
is dense: one first fit, address ordered page heap for every thread, so a
touched 2 MiB region is mostly used (71-89% fill), and its scavenger
releases only chunks not dense for a GC cycle, widened to whole huge pages
(mgcscavenge.go:957-984). Xo gives every thread its own 4 MiB bump chunk
and 4 MiB page chunk; each is lightly used (13-21%), and under `always`
each touched half becomes a whole huge page: about 5x its small page RSS
on amd64, against about 1.35x for Go.

What Xo could copy, ranked by likely effect for the cost:
1. Go's density rule for huge pages: map a chunk MADV_NOHUGEPAGE and advise
   MADV_HUGEPAGE on a 2 MiB half once the bump cursor has filled it, so
   only dense regions take huge pages (one madvise per filled half).
2. Small first chunks on Linux too (decision 0455, darwin only so far):
   the PROT_NONE tail splits the mapping, so a sparse first chunk cannot
   become a huge page; Linux paid 2-3% wall for it with whole chunks.
3. A shared, address ordered page heap across threads (Go's mheap): the
   densest layout, and the largest change.
