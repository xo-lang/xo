# Landing pad size, 2026-10-31

Two changes to the code the native arm64 backend emits for fault
exits (decision 0448's landing pads), measured on xo itself.

- P1 (c31f6469b): a pad that releases most of a root releases each
  fully released subtree as one value (compiler/ssa grouppads.xo),
  and a struct release in a landing block drops a field chain held
  in memory through its type's drop helper (isel.xo chain_mem).
- Lever 2 (static zero images): a fault exit that resets a `var`
  receiver to its zero value stored the zero word by word (about
  1,000 words for ssa.Builder). zero_val calls written back to var
  parameters are inlined (inline.xo find_zero_ret), and the MIR pass
  `zero_images` (after copy_prop) turns a run of 32 or more constant
  word stores to consecutive offsets of one base into a memcpy from a
  read-only image, keyed by its contents and shared across split
  parts.

## Size

Stage 2 `__text` of xo (each tree's xo builds its own compiler/xo),
darwin/arm64.

| change | base | new | delta |
|---|---|---|---|
| P1 | 64,321,440 | 56,330,136 | -12.4% |
| lever 2 (base f25d1f513) | 56,430,920 | 50,821,296 | -9.9% |

P1 by cause: landing reads 6.07 -> 0.45 MB, landing rc 12.43 -> 10.14 MB.

Neither change touches `build_native` (5,319,800 B before and after
both): its pads release about 1,000 separate locals, not the subtrees
of one root, so pad sharing is the next lever there.

## Speed

`xo bench --ab` on the linux/arm64 offload host, NEW / BASE, 95%
interval. memcpy replaces stores, so lever 2 is checked on the self
build and the fault paths.

| change | program | ratio (wall) |
|---|---|---|
| P1 | bench/perf/fault owned | 0.991 (0.981 to 1.005) |
| P1 | bench/perf/fault defer | 0.996 (0.994 to 1.008), 64.01 -> 63.77 ms |
| lever 2 | self (n=10) | 1.006 (0.986 to 1.022), 52,284 -> 52,585 ms; cpu 0.999 (0.994 to 1.002); max RSS 1.006 (0.997 to 1.019) |
| lever 2 | bench/perf/fault owned (n=10) | 1.011 (0.998 to 1.028), 25.67 -> 25.95 ms |
| lever 2 | bench/perf/fault defer | 0.997 (0.993 to 1.0), 64.0 -> 63.82 ms; cpu 0.997; max RSS 1.0 |

## Negative result: result homes (not landed)

build_native is 5.3 MB of xo's code, 90 percent of it cold landing
pads that release about 1,000 heap leaves each, one drop-helper call
per leaf. P1 and lever 2 do not change it (5,319,800 B before and
after). The idea tried here: a large struct that a call by reference
returns gets a home in the frame (arm64 byref.xo, a pass
`result_homes`), so a pad releases it with one call to its type's drop
helper. Branches lang1-reshome, lang1-reshome2, lang1-reshome3; base
eb95e9119, baseline the same tree with `XO_PASSES=-result_homes`.

| version | build_native | stage 2 __text |
|---|---|---|
| pass off | 5,364,840 | 56,877,224 |
| v1: homes for every such type, existing `var` argument homes take results too | 6,971,968 | 58,242,600 |
| v2: v1, and a value read whole somewhere stays in registers | 7,572,160 | 59,062,272 |
| v3: v2, only types with no home | 6,067,696 | 57,556,120 |
| v4: v3, and a block parameter takes the home only when every edge passes it in place | 5,805,056 | 57,294,984 |

Why it does not pay (build_native's IR dump with a `== held` section,
v4):

- Only Checker got a result home. 379 pad releases of Checker block
  parameters now come from it, but the helper calls fell by 1,431
  (599,664 to 598,233): a Checker has about 4 heap leaves.
- The leaf calls come from block parameters of types that already have
  a `var` argument home or have two values live at once, so they stay
  in registers: Builder (484 pad releases), Ctx (413), Opts (380),
  Lowered (257), Module (244).
- The growth is register pressure: spill slots 3,036 to 3,444, 4,290
  more stores and 1,599 more loads (v1 and v2 also evicted values that
  `var` argument homes kept across calls).

The pads' leaves are spread over many locals, so the next lever is
sharing the pads' common code (a tail merge of cold blocks in arm64
MIR), not homes.
