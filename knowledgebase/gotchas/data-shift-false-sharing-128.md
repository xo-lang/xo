# A 64 byte layout shift looked like 128 byte false sharing; moving .data alone did nothing

Status: Observed (2026-11-02, Xo Lang 1, 3, 4 and 2)

Symptom: main's bench/perf/jitter /alloc on a Neoverse host lost about 25% CPU a request
when a change moved xo_static 64 bytes (72 bytes less .text), and padding probes that grew
.text moved the speed back and forth; false sharing at 128 bytes (Neoverse pairs 64 byte
lines) in rt.static_word's table looked like the cause.

Cause: not .data. Linking the same objects with XO_DATA_SHIFT=0 and 64 (decision 0459),
which moved xo_static to a different 128 byte phase with the code unchanged, moved CPU a
request by 1%. A probe that grows .text moves every function after it too, so it tests code
and data placement together.

Fix: tell them apart before chasing either: XO_DATA_SHIFT=64 moves only .data;
XO_ARM64_PAD (decision 0075) moves only code. xo_static and xo_sys_words are now 128 byte
aligned, so their line sharing no longer changes with unrelated edits.
