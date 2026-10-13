# Editing std/ during TestSelfhostSSA gives thousands of line differences

Status: Observed (2026-10-12)

Symptom: TestSelfhostSSA reported 9,570 differences, every one a source
position one line apart (`@ std/native/llrt/num.xo:73:10` against
`:72:10`).

Cause: the test runs the Go SSA builder and the Xo one at different
times over `--std=std`, the working tree's std directory; a line added to
std/native/llrt/num.xo between the two runs moved every position after it.

Fix: do not edit std/ (or selfhost/) while a selfhost differential test
runs; rerun it on a quiet tree. Differences that are only positions one
line apart are this, not a builder bug.
