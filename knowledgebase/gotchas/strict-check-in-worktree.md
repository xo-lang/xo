# strict-check.sh finds no analyzers in a git worktree

Status: Observed (2026-10-08)

Symptom: in a `.claude/worktrees/<name>` checkout, `bench/go/strict-check.sh`
fails every analyzer step with "no such file or directory", so every Go
solution looks strict red (and planted bug detection counts a "new finding"
for nothing).

Cause: the script looked for the analyzers only in `../../.tools/bin`
relative to itself. `.tools/` is gitignored, so a worktree has none; the
binaries exist only in the main checkout.

Fix: the script honors `XO_TOOLS_BIN` (2026-10-08). `bench/harness/common.py`
sets it automatically: `$XO_TOOLS_BIN`, else `<checkout>/.tools/bin`, else
the main checkout's `.tools/bin` found through `git rev-parse
--git-common-dir`. When running by hand in a worktree:
`XO_TOOLS_BIN=<main checkout>/.tools/bin bench/go/strict-check.sh <dir>`.
