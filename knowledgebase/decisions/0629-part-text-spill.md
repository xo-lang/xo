# 0629: Part text spill: the parts' assembly text in unlinked files between emit and assembly

Status: Accepted (Xo Lang 1, 2026-11-13; lead)

## Context

After decision 0628 the gcp aiplatform/apiv1 test build's peak moved
from the SSA tables to emit and assembly. Past emit, 932 MB was live, of
it 906 MB the parts' assembly text (arm64 gen_part, raw.str_join): every
part's text waited for three whole-program passes (drop_weak_all, which
needs the module's exports; hollow, whose reach needs every part's
facts; darwin's undefined-symbol check) and then the assembler. The
self build of xo has about 1.1 GB of text.

Assembling each part on its emit task would free the text sooner but
changes the output: hollow rewrites the text of the functions nothing
reaches (a trap as the body) after the reach over every part is known,
so assembling first would mean stripping atoms instead, and every later
address would move.

## Decision

1. arm64 gen_part, once the parts' text so far passes XO_TEXT_SPILL MB
   (256 by default; `on` 0, `off` never; a shared atomic count), writes
   its part's text to a file of its own under the build directory's
   textspill/ and removes the file's name at once: only the open handle
   names it, so the system frees its blocks when the handle closes or
   the process ends, a crash or a stop included (textspill.xo
   PartText). Program.parts is a List[PartText] (in memory or in a
   file).
2. Every pass after emit reads a part back, rewrites it in its place
   (File.write_at and truncate) or uses it, and drops it:
   drop_weak_all on its tasks, hollow (hollow_plan_parts, or incr_hollow
   with the back-end cache), undef (undefined_xo_parts), the object keys
   and the assembler (cached_macho_objects, macho_objects, one part per
   task). A part's hollow and undefined-symbol facts are copied out of
   its text (an encode and decode, as the cache stores them): views of
   the text kept every part's text alive.
3. The paths that read the whole text (--emit=asm and `xo asm` keep
   spill off; XO_KEEP_ASM, the cc assembler, windows, XO_SIZE_CAUSES)
   read the parts back.
4. The targets: darwin, linux and freebsd, arm64 and amd64. windows
   stays in memory (an open file cannot be removed there); wasm and the
   LLVM backend have no parts.

## Measurements

`xo bench --ab` on linux/arm64 host 1, main bb7a34528 against this
(3fe1b7367), each side's stage 2 xo and std. The host was loaded (median
load 7 to 20 on 12 CPUs), so wall and CPU are noisy; max RSS is not.

| build | n | max RSS | CPU | wall |
|---|---|---|---|---|
| aiplatform/apiv1 test build, default caches, each run in a fresh cache (headline; rerun quiet, load under 4) | 9 | 3.74 to 3.36 GB (0.897, 0.883 to 0.918) | 1.009 (noise) | 1.015 (noise) |
| the first run of that row (caches emptied by rm -rf, partly warm on a few rounds, host loaded) | 9 | 3.79 to 3.41 GB (0.901) | 1.003 | 1.059 |
| aiplatform/apiv1, caches off (diagnostic) | 9 | 3.07 to 2.67 GB (0.869, 0.847 to 0.882) | 0.994 (noise) | 0.942 (noise) |
| the self build (compiler/xo) | 3 | 3.87 to 3.64 GB (0.940) | 1.008 | 1.015 |

The first default-cache run emptied the caches with rm -rf, which failed
on a few rounds with "Directory not empty": the background std build of
the round before was still writing (gotchas/cache-rm-under-background-writer.md).
The headline row is the rerun: each run gets a fresh mktemp -d cache, switched
once the round before's std/.bg.lock is free, on a quiet host (2026-11-13).

Mac (darwin/arm64, a darwin footprint, not the headline):
Mac (darwin/arm64), on 0628, caches off unless said, XO_TEXT_SPILL=off
against the default:

| build | max RSS off | max RSS on | wall |
|---|---|---|---|
| the self build (compiler/xo) | 4.57 GB | 3.90 GB | +1.6% |
| aiplatform/apiv1 test build | 3.26 GB | 2.95 GB | +2% |
| aiplatform/apiv1, default caches, cold (2 rounds) | 3.98, 3.99 GB | 3.50, 3.49 GB | +5.9%, -1.7% |

Peak disk (df sampled every 0.2 s): about 0.5 to 0.6 GB above the
build without it.

Output: byte for byte the same with it on and off for the self build on
darwin/arm64 (with the same output file name: the ad-hoc signature
names the file), linux/arm64 and linux/amd64, and through the back-end
cache's incremental hollow path.

Peak memory footprint (darwin keeps MADV_FREE_REUSABLE pages in max RSS,
decision 0383), aiplatform caches off: 2.77 to 2.46 GB.

## Consequences

- The peak is now split between emit (the SSA still live, 2.69 GB heap
  on aiplatform) and the assembly (2.98 GB: every object held in a list
  until the link); keeping the objects out of the heap is the next step.
- Writing and reading the text back costs about 1 to 2% wall (Mac +1.6 to
  2%, host 1's self build +1.5% loaded, its aiplatform rows within noise).
- A build needs about as much free disk as its spilled text.

## Alternatives

- Assemble each part on its emit task: changes the output (above).
- Keep the text compressed in memory: a compression pass per part and
  per rewrite, and still in the heap.
- Pack ssa.Value (56 to 40 bytes): after 0628 the SSA is freed before
  the peak, so under 0.1 GB at it.
