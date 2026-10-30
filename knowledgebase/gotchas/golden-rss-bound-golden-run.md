# A golden's peak RSS bound fails only under golden-run.sh

Status: Observed (2026-10-30, linux/amd64 hourly goldens, host_usage).

Symptom: host_usage printed `rss false true` in scripts/linux-goldens.sh on the linux/amd64 host, deterministically, while `xo run` of the same golden and a direct run of the built binary printed `rss true true`.

Cause: a small static native program peaks near 1 MB of resident memory, and the run environment (golden-run.sh's timeout wrapper, a copied directory, stdin from /dev/null) moves it either side of a 1 MB bound.

Fix: bound measured memory loosely, by what the check means. host_usage needs only "bytes, not Linux's unscaled KiB", so it checks > 100 KB. Reproduce a hosts-only golden failure through scripts/linux-goldens.sh (the hourly path), not `xo run`.
