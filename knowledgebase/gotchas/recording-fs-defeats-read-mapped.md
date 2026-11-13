# The run cache's recording Fs turns read_mapped into a whole read

Status: Observed (2026-11-13, Xo Lang 1)

## Symptom

With the run cache on (the default), every darwin `xo build` of a
separately compiled program peaked about 42 MB higher than with
`XO_BIN_CACHE=off`: llvm-fib's edit 87.5 MB against 46 MB
(bench/results/release-vs-go-v0.50.1.md), above Go's 69 to 71 MB.
`--phase-times` showed the jump in `phase undef` (peak 37 to 77 MB, 1.65
to 7.0 ms).

## Cause

A stored build runs with the recording Fs (compiler/xo/recorder.xo
RecFs, decision 0360 stage 2). It is a user value converted to Fs, and
`read_mapped` (std/native/prelude.xo) maps only the real Fs: through any
other Fs it reads the file whole. The undefined-symbol check
(main.xo sepcomp_undefined) read std's store (about 40 to 60 MB) with
`read_mapped(os.fs, ...)`, so under the recording it copied it. The link
already read the same store through `ctx.real_fs ?? os.fs` (ld.xo).

## Fix

Read files under xo's caches (which the recording skips anyway) with
`read_mapped(ctx.real_fs ?? os.fs, path)`. When a phase's peak differs
between the run cache on and off, look for a `read_mapped` (or another
mapping read) through `os.fs`.
