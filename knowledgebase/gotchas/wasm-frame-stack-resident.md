# Wasm: the memory-frame stack was 16 MB resident per instance

Status: Observed (2026-11-04, decision 0447 stage 5 step 3 measurement)

## Symptom

A wasip3 component whose requests call an outgoing HTTP client used
about 20 MB of resident memory per instance against 3 MB for a hello
component: 752 MB at 32 connections and 5.45 GB at 256 under `wasmtime
serve`, where Rust's component used 67 MB and 146 MB.

## Cause

A function with more locals than FRAME_MIN keeps the rest in a frame on
a stack in linear memory (compiler/wasm func.xo memory_frame), and that
stack was one FRAME_STACK block of 16 MB from heap_large at the first
framed call. Linear memory grows by doubling, so the module's memory
went from 7 pages to 1,023 at the first request, and the block was
resident in every instance. A debug build that trapped on any heap need
over 16 MB gave the wasmtime backtrace lbump < heap_large < ls_init.

## Fix

The stack grows in chunks ($ls_push, $ls_pop in gen.xo ls_text): 64 KB
first, then a chunk of twice the last new one's size (or the frame's
own) whose first frame gives it back on return; the last chunk given
back is kept for the next overflow. With a 1 MB stack the proxy's peak
was 167 MB at 32 connections and 322 MB at 256. Golden frame_chunks
recurses 200 deep through a 200 local function (1,112 byte frames, the
64, 128 and 256 KB chunks). Two traps writing it: values used only after
the recursive call are computed after it, so the function kept 69 wasm
locals and no frame (check `call $ls_init` or `$ls_push` in the
XO_KEEP_WAT output); and 600 deep exhausted wasmtime's own call stack
("call stack exhausted"), which is not this stack.
How to see it next time: print `memory.size` at a request's end, then
trap on a large heap need for the backtrace (heap_large's grow loop).
