# mawk reads a pipe in blocks: an awk that should exit early does not

Status: Observed (2026-11-19, TestOffloadWslJob on host 3)

Symptom: offload_wsl_job_test.sh's "ssh gone" cases failed on host 3
(the job ran to the end and left ~/survived) and passed on the Mac.

Cause: the test stood in for a dropped ssh with `awk '{print} /JOB-RAN/
{exit}'`. On Debian the awk is mawk, which fills a 4 KB buffer from a
pipe before it handles a line, so it saw JOB-RAN only at EOF, after the
job ended, and the job's heartbeat never hit a closed pipe. The Mac's
awk handles each line as it arrives.

Fix: read the line with a bash `while IFS= read -r` loop, which stops at
once (or use `mawk -W interactive`). Any reader that must react to a
line before the writer ends has the same trap.
