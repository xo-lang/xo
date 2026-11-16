# os.tasks faults in std module tests

Observed 2026-11-16 (std/ui pg_bus test). Fixed 2026-11-16.

Symptom: a test in a std module run by `xo test --std-module NAME` that
uses `os.tasks` faults with `os.tasks is not available here`
(tasks.prog_group). The same test in a plain `xo test DIR` passes, and
spec/std.md section 8 says tests get `os.tasks`.

Cause: compiler/ssa `uses_os_tasks` decides whether a test binary opens
the os.tasks root scope (the program's task group) by scanning the
program's modules for `.tasks`, and it skipped every std module. Under
`--std-module` the module whose tests run is a std module, so nothing
was scanned and the group was never set.

Fix (b07cdabad, 2026-11-16): a test build also scans the std modules it
tests (the root modules); cmd/xo TestXoTestStdModuleOsTasks runs a probe
std module's os.tasks test under `--std-module`. Tests may use os.tasks
again; the scope workaround in std/ui's pg_bus test is no longer needed.
