# The build recorder must not record the machine's state (/proc, /sys)

Status: Observed (Xo Lang 2, 2026-10-30)

Symptom: on Linux an unchanged `xo build`, `xo run` or `xo test` never hit the run cache
(decision 0360): `phase run_cache_miss` every time; TestNoGCRunCache, TestWholeBuildCache and
TestWholeBuildWindowsPDB failed on the linux/arm64 offload host from its first tier (28), the Mac
passed. In every release with the recorder, 0.28.0 (063346659) through 0.35.0 (from the code at the tags).

Cause: the build recorder (compiler/xo/recorder.xo) notes every file the build reads as an input
with its SHA-256. compiler/xo/split.xo mem_gb reads /proc/meminfo on Linux (debug.mem_total is 0
there) for the memory budget, and /proc/meminfo's MemFree and the rest change from one read to the
next, so the entry's hash never matched again. The entry listed it as its 5th file.

Fix: rec_skip starts with rec_machine (/proc/ and /sys/): the machine's state is not a build input.
To find such a miss: copy the entry (<bin cache>/<key>.m) after one build, build again, and rehash
each `file` line (hex encoded paths, x-prefixed); the first file that differs is the culprit.
