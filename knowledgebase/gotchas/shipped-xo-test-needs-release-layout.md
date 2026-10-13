# A shipped xo built into a bare temp directory finds no std

Status: Observed (2026-10-13, batch 7 full run on main 40b08de1, EC2
Linux arm64; reproduced on the Mac); fixed 2026-10-13

Symptom: cmd/xo TestCLIPattern (decision 0210) failed with every `xo
check`, `vet`, `doc`, `fix`, `test`, and `build` exiting 70 with
`fault: prelude type missing: Int at check/types.xo:126:3` (the self
hosted checker's Checker.prelude_obj). Each `DIR/...` still matched its
plain `DIR` run, since both faulted the same way; only the `./...`
check that expects exit 1 and the broken file caught it. It had passed
for the agent that added it, likely with $XO_STD set in its shell.

Cause: shippedXo built xo1 to `<TempDir>/xo`. With no $XO_STD, xo1
looks for std beside its binary (check.std_dir: `../lib/xo/std`, `std`,
`../std`) and otherwise falls back to `std` in the working directory,
which in the test's temp module does not exist. load_std then returned
-1 for the prelude without a word, and the checker faulted the first
time a body named `Int`.

Fix: two parts.
- selfhost/check: check_dirs returns an error when the std directory
  has no prelude.xo ("std library not found: no DIR/prelude.xo (set
  XO_STD ...)"), and run returns one after the declarations when the
  prelude lacks a core type (Bool, Int, Never, Option, Result, Str), so
  every command prints `xo <command>: ...` and exits 2 instead of
  faulting. cmd/xo TestCLINoStd covers both.
- cmd/xo shippedXo builds into a release layout (`bin/xo`, with
  `lib/xo/std` a symbolic link to the repository's std), as
  xoModulesBinary already did, and clears $XO_STD for the runs.

Avoid: a test that runs a built xo1 or other self hosted command that
checks code must give it std: a release layout or $XO_STD. Do not rely
on the working directory.
