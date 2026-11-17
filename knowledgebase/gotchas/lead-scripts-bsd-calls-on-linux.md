# Lead scripts' BSD calls on Linux: fallbacks that never run

Status: Observed 2026-11-17, fixed in heavy.sh, quiet.sh and offload.sh.

## Symptom

`scripts/lead/test-heavy.sh` in a golang:1.27.4 Docker container (and on
a Linux gate host) failed scenarios 11, 12, 15 and 17 and hung ten more
until the 420 s cap. The Mac passed all 19.

## Cause

Four macOS calls whose Linux fallbacks were wrong or dead:

- `stat -f %m FILE || stat -c %Y FILE`: GNU `stat -f` means "file
  system", prints a block of text ("Block size: 4096 ...") and then fails,
  so the fallback's output was appended to the garbage.
- `date -r EPOCH`: GNU `date -r` reads a file's mtime, so an epoch is
  "No such file".
- `sysctl -n vm.loadavg | awk ... || awk ... /proc/loadavg`: a pipe's
  status is its last command's (awk, 0), so the `/proc` fallback never ran;
  the load was empty, `[ "" -lt N ]` failed, and every measurement drain
  waited forever.
- `lsof` is missing from slim Linux images, so a gate holding a
  build.lock looked stalled and was stopped; `top -l 1` fails on Linux,
  so the memory cap summed nothing.

## Fix

GNU form first where the BSD spelling means something else on Linux
(`mtime()`: `stat -c %Y || stat -f %m`; `hms()`: `date -r || date -d @`),
a captured value with an emptiness check instead of `pipe || fallback`,
`/proc/PID/fd` when there is no lsof (`holds_lock`), and `ps -o rss`
when `top -l` fails. Test any lead script change on Linux too:
`TH_ONLY="s17"` runs a subset of test-heavy.sh scenarios.
