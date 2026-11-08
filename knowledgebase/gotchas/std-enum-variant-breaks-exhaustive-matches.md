# A new variant of a std enum breaks user code that matches it exhaustively

Status: Observed (Xo Lang 1, 2026-11-08; decision 0571's `TargetArch.Riscv64`
and decision 0573's `TargetOs.FreeBSD`)

## Symptom

After `TargetOs.FreeBSD` landed (8781f44e7), every program with a
`match target.os { Linux => ... Darwin => ... Windows => ... Wasi => ... }`
stopped building with XO0301 (a non exhaustive match), on every target,
since every arm is checked for every target (spec 12.2). In the repository:
four goldens (method_values, fs_paths_os, sys_files_clock, gen's
audit_second_pass), bench/xo/v0.4 and the planted 04-file-sync variants,
and the three blind solutions bench/paper/v0.{1,2,3}/04-file-sync, which
are frozen and left failing. The amd64 sample found method_values; the
lazy check of compiler/xo had not.

## Cause

Enums are closed and `match` is exhaustive (spec 3.x, XO0301), so a new
variant of a std enum that user code matches is a source break, even
though no existing value changes. `TargetOs` and `TargetArch` are the
common case: matching them exhaustively is the idiom spec 12.2 teaches.

## Fix

- In the change that adds the variant: scan the whole repository (not only
  std and compiler) for exhaustive matches on the enum, as
  new-target-variant-lazy-check.md describes, and give each an arm. Leave
  bench/paper's blind solutions as written (no test type-checks the
  affected ones on 2026-11-08: cmd/xo's same-path tests read
  08-notes-service, and TestSelfhostDefhash is a dump replay, skipped by
  default).
- Call it out as **Breaking** in CHANGELOG.md with the migration (add an
  arm or a `_ =>` arm), per decision 0567 rule 2; XO0301 names the missing
  variant, which serves as the diagnostic that names the replacement.
- An `_ =>` arm in user code avoids the break, at the cost of the check
  that every OS is handled.
