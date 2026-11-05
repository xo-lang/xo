# 0506: size classes above 1 KiB, four per doubling

Status: Proposed (Xo Lang 3, 2026-11-04, at the lead's request; the class
change and its --ab are Xo Lang 2's, who owns the heap code and 0505)

## Context

heapos.xo `class_bytes`: 16 byte classes to 1 KiB, then one class per
doubling: 2040, 4088, 8184, 16376, 32760, 65528, 131064 and 262136 bytes
after the 8 byte prefix. A buffer whose payload is a power of two (the usual
size of a read buffer, a body buffer or a page) plus its header does not
fit the class below it and takes the next, so it maps twice its size.
Go's classes are about four per doubling.

Measured (bench/results/size-class-waste-2026-11-04.md, bench/perf/sizewaste,
linux/arm64 host 2, about 195 MB of payload held per row, RSS growth over
payload; Go is 1.00 to 1.08 in every row):

| payload | Xo | Go |
|---|---|---|
| 8, 16, 32, 64 KiB | 2.01, 2.02, 2.03, 2.06 | 1.01 to 1.03 |
| 128 KiB | 2.14 | 1.00 |
| 12, 24, 48 KiB | 1.34, 1.35, 1.36 | 1.00 to 1.02 |
| 16,000 bytes | 1.04 | 1.03 |

Xo's own build (xo building compiler/xo on host 2): maximum RSS 3.86 GB, and
at exit the classes above 1 KiB hold 1.85 GB of the 3.55 GB of retained free
blocks (context only: free blocks are not the doubling's waste; that needs
live requests per class, still open). std buffers of a power of two: http.xo's 32,768 byte
pass-through buffer (a 64 KiB class), linux.xo's 16,384 byte buffers (a
32 KiB class), pg.xo's 65,536 + 1,024 byte one (a 128 KiB class).

## Decision

The live request census (bench/results/size-class-waste-2026-11-04.md "Live bytes by request size", the self build at inline.verify, 2,097 MB in use): requests over 1 KiB hold 68% of the live bytes, 4 KiB to 256 KiB hold 47.5%. Shape (Xo Lang 2's review): below 16 KiB
classes at about 1.25x (1280, 1536, 1792, 2048, 2560 ... in 256 B, 512 B and
1 KiB granules, each a step minus the 8 byte prefix); from 16 KiB the page
classes (runs of 16 KiB pages) in multiples of 16 KiB: 32, 48, 64, 80, 96,
112, 128, 160, 192, 224, 256 KiB. The 16 byte path to 1 KiB stays a shift;
a small table serves requests above 1 KiB. Bar: --ab self (max RSS, wall),
bench/perf/alloc small and mixed, darwin footprint. A cheap first step
regardless: size std buffers just under a class (http.xo 32,768 to 32,736).

## Consequences

PENDING

## Alternatives

- Keep the power-of-two classes and size std buffers just under a class
  (16,000 instead of 16,384): fixes std's own buffers, not the user's.
- Round large blocks to pages (Go's way above 32 KiB): a different allocator
  path for sizes now served by classes.
