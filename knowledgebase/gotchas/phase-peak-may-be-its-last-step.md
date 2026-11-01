# A phase's peak can be one step at its end

Status: Observed (2026-11-01, decision 0448)

## Symptom

The self build's `--phase-times` line said emit peaked at 3937 MB, and a run of fixes
aimed at emit's work moved nothing: a lower live size, per function regions, and a
different part order all left the peak within 4 MB. Those attempts took most of a day.

## Cause

The phase line's peak says which phase, not when in it. Emit's parts stayed under the
earlier 3539 MB peak throughout. The rise came after them, in `drop_weak_all`, which
copied every part's text (about 1.1 GB) while the parts, the files list and its own
argument still held the originals.

## Fix

A peak that does not move under a change aimed at it is set somewhere else. Before
working on a phase's peak, read `debug.mem_mapped()` and `debug.mem_peak()` at
checkpoints through the phase, as a probe. Where a rewrite copies many large texts,
let each task take its text from a cell so the original goes when its copy exists
(`take_text` in compiler/arm64/parallel.xo).
