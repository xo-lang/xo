# os.tasks faults in std module tests

Observed 2026-11-16 (std/ui pg_bus test).

Symptom: a test in a std module run by `xo test --std-module NAME` that
uses `os.tasks` faults with `os.tasks is not available here`
(tasks.prog_group). The same test in a plain `xo test DIR` passes, and
spec/std.md section 8 says tests get `os.tasks`.

Cause: not found yet; the --std-module test binary does not set up the
program task group that plain test binaries do.

Workaround: start the tasks in a scope's group in the test
(`scope s { let g = s.group() ... }`), as std/ui's pg_bus test does.
Fix: the --std-module runner gives tests os.tasks; the test above can then
use os.tasks again.
