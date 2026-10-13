# 0209: Test harnesses as an `xo dev` subcommand written in Xo

Status: Accepted (owner, 2026-10-13); not started. Queued after decision
0195's zero-C work, as part of the tooling hardening.

## Context

Three Python scripts (scripts/selfhost-goldens.py,
selfhost-gogoldens.py, elf-goldens.py) build the golden programs with
xo1, run them, and compare output. They overlap, behave differently by
platform (a text mode read turned show_table's raw CR into LF,
gotchas/python-text-mode-want-cr.md), and need Python. After decision
0178 stage 7 the Go compiler packages are deleted (the test helpers
stay), so Go tests are not the long term home for Xo's own tooling.

## Decision

The harnesses become a hidden developer subcommand of xo, written in Xo
(for example `xo dev goldens --backend=arm64 <dir>`, later the replay
recorder, the ELF and Mach-O checks, and the Windows batch steps), with
byte exact comparison by default and code shared with `xo test`. One
thin Go test per harness calls it, so the existing tiers and batches
keep running it. The Python scripts run beside it until both agree on a
full batch, then they are deleted.

## Consequences

- The harness dogfoods Xo's std (files, processes, text) on every run
  and works on every host xo1 runs on, without Go or Python.
- A std bug can break the harness itself; running beside the old scripts
  until they agree catches that before the switch.
- `xo dev` is not part of the user facing CLI surface in spec/core.md
  (hidden, like the dump commands).

## Alternatives

- Go tests only: rejected; ties Xo's tooling to the Go toolchain and is
  not runnable outside the repository.
- Keep the Python scripts: rejected; platform differences and three
  overlapping implementations.
