# Size class waste above 1 KiB (2026-11-04)

Measurement for the proposal of finer classes (ADR 0505 finding 3). Probe:
bench/perf/sizewaste (Xo) with its Go twin: COUNT byte buffers of SIZE bytes
(about 195 MB of payload each), every byte written, held; RSS growth read
from /proc/self/status. linux/arm64 host 2, one process per row, one run
each (RSS is deterministic here). Ratio is RSS growth over payload; Go is
about 1.00 to 1.08 everywhere.

| payload bytes | blocks | payload KB | Xo RSS KB | Xo ratio | Go RSS KB | Go ratio |
|---|---|---|---|---|---|---|
| 8,192 | 24,414 | 195,312 | 393,420 | 2.01 | 202,004 | 1.03 |
| 9,000 | 22,222 | 195,310 | 358,568 | 1.84 | 210,388 | 1.08 |
| 12,288 | 16,276 | 195,312 | 262,144 | 1.34 | 198,924 | 1.02 |
| 16,000 | 12,500 | 195,312 | 202,752 | 1.04 | 201,744 | 1.03 |
| 16,376 | 12,212 | 195,296 | 394,508 | 2.02 | 197,552 | 1.01 |
| 16,384 | 12,207 | 195,312 | 394,508 | 2.02 | 199,676 | 1.02 |
| 24,576 | 8,138 | 195,312 | 263,940 | 1.35 | 198,872 | 1.02 |
| 32,768 | 6,103 | 195,296 | 396,432 | 2.03 | 196,436 | 1.01 |
| 32,760 | 6,105 | 195,312 | 396,432 | 2.03 | 196,460 | 1.01 |
| 49,152 | 4,069 | 195,312 | 265,868 | 1.36 | 196,104 | 1.00 |
| 65,536 | 3,051 | 195,264 | 402,512 | 2.06 | 197,928 | 1.01 |
| 65,528 | 3,052 | 195,304 | 402,512 | 2.06 | 195,928 | 1.00 |
| 98,304 | 2,034 | 195,264 | 269,904 | 1.38 | 195,664 | 1.00 |
| 131,072 | 1,525 | 195,200 | 416,816 | 2.14 | 195,652 | 1.00 |
| 131,064 | 1,525 | 195,188 | 416,816 | 2.14 | 195,636 | 1.00 |
| 196,608 | 1,017 | 195,264 | 278,064 | 1.42 | 197,616 | 1.01 |
| 262,144 | 762 | 195,072 | 198,148 | 1.02 | 197,576 | 1.01 |

Reading: the classes above 1 KiB are 2040, 4088, 8184, 16376, 32760, 65528,
131064 (heapos.xo class_bytes). A buffer of 8, 16, 32, 64 or 128 KiB (the
usual power of two) plus its header does not fit the class below, so it
takes the next one and holds 2.0 to 2.14 times its payload. 16,376 bytes
(the class size itself) also lands in the next class (2.02): a Bytes of n
bytes needs n plus 32 bytes (24 of object header, 8 of block prefix). A size between two classes
wastes its distance to the class above (12,288 and 24,576 and 49,152 bytes:
1.34 to 1.36; 98,304: 1.38; 196,608: 1.42). Only a payload just under a
class (16,000: 1.04) is close to Go. 262,144 bytes (over the classes' 256 KiB limit) is a large mapping and costs 1.02, like Go (the first run read 0.02 because the probe let the blocks die before the sample; the probe now uses every block after the sample, rerun 2026-11-04 on linux/arm64 host 2, same table values for every other size within 0.01).

## The self build (xo building compiler/xo, --release)

linux/arm64 host 2, XO_HEAPSTATS=1, /usr/bin/time -v: 31.9 s wall, maximum
resident set 3,857,148 KB, 1,030,924,348 allocations. At exit the heap held
3,553 MB in retained free blocks (not resident memory by itself: darwin
advises page class blocks as they are freed, and 0505's trim will on linux),
of which the classes above 1 KiB held 1,850 MB (52%); the table's sizes are
block bytes including the 8 byte prefix:

| class (block bytes) | free blocks | MB |
|---|---|---|
| 2048 | 40,684 | 84 |
| 4096 | 22,460 | 92 |
| 8192 | 23,562 | 193 |
| 16384 | 16,356 | 268 |
| 32768 | 9,311 | 305 |
| 65536 | 3,686 | 242 |
| 131072 | 3,346 | 439 |
| 262144 | 868 | 228 |

This is the free memory the heap keeps, not the waste inside live blocks
(heapstats does not count requests per class), so it bounds what a finer
table could reclaim from the cache but does not measure it; the live-block
waste needs the heap profile census of Xo Lang 2's plan (step 1). It does show
that xo's own memory is mostly in the classes the doubling coarsens.

## std buffers that are a power of two

std/native/http.xo:4788 `raw.bytes_buf(32768)` (the proxy pass-through
buffer, one per tunnelled connection) takes the 65,528 byte class: 64 KiB
mapped for 32 KiB. Others found by grep: linux.xo `bytes_buf(16384)` (two
sites, 32 KiB class), pg.xo `with_capacity(65536 + 1024)` (a 128 KiB class
for 65 KiB). A measured row for each is still to do.

## Live bytes by request size (self build, 2026-11-04)

The heap in use at "building, past phase inline.verify" of `xo build
--release compiler/xo` (XO_PROF=heap, XO_PROF_HEAP_AT, rate 1 MiB, linux/arm64
host 2, 2,097 MB in use). Each sample's size is its in use bytes over its
in use objects (the sampling estimate, so the figures are rounded):

| request size | live MB | share | objects | mean size |
|---|---|---|---|---|
| up to 64 | 535.0 | 25.5% | 10,350,674 | 54 |
| 65 to 256 | 98.0 | 4.7% | 969,929 | 106 |
| 257 to 1,024 | 37.0 | 1.8% | 78,080 | 497 |
| 1,025 to 4,096 | 130.2 | 6.2% | 52,943 | 2,578 |
| 4,097 to 16,384 | 247.4 | 11.8% | 24,958 | 10,394 |
| 16,385 to 65,536 | 311.0 | 14.8% | 8,032 | 40,599 |
| 65,537 to 262,144 | 439.2 | 20.9% | 3,162 | 145,660 |
| over 262,144 | 299.1 | 14.3% | 505 | 621,140 |

Requests over 1 KiB hold 68% of the live bytes; those between 4 KiB and
256 KiB, the sizes the probe wastes up to half of, hold 47.5%. The classes
now round each of them to the next power of two minus the header, so a
request spread evenly inside a class wastes 25% of it on average and up to
50%: on this build that is about 250 to 450 MB of the 2,097 MB (not
measured directly: the profile has the request size, not the block size).
Script: the census steps of `$S/census2.sh` (heap profile, `go tool pprof
-raw`, an awk histogram of in use bytes over in use objects).
