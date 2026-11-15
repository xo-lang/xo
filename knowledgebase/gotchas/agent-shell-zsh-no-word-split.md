# The agent's shell is zsh: an unquoted $var is one word

Status: Observed (2026-11-14, Xo Lang 3)

## Symptom

A loop that built `ab-host.sh ... -- $a` from `a="bench/perf/urltype string"`
reached the host as one argument: `ab-host: bench/perf/urltype string is
not a directory of NEW`, exit 2, for every run of the chain (and
`stage2 self` the same way).

## Cause

The Bash tool's commands run under the user's login shell, zsh on the
Mac. zsh does not split an unquoted parameter expansion on spaces (its
SH_WORD_SPLIT option is off), where bash does, so `$a` is one word.

## Fix

Pass the words separately (`run NAME bench/perf/urltype string`, with
`"$@"` inside the function), or run the loop under `bash -c '...'`. Do
not rely on word splitting of a variable in a command the Bash tool runs;
scripts with a `#!/bin/bash` line are unaffected.
