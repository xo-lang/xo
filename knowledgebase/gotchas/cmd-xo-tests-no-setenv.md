# cmd/xo tests cannot use t.Setenv, t.Chdir, or os.Setenv

Status: Anticipated (2026-10-14, bench/results/short-tests-2026-10-14.md)

Symptom: a new cmd/xo test that calls `t.Setenv` or `t.Chdir` panics
("test using t.Setenv or t.Chdir can not use t.Parallel"); one that
calls `os.Setenv` or `os.Chdir` changes what other tests' xo processes
see, at random.

Cause: every cmd/xo test is `t.Parallel()` (the package's -short run went
from 25 s to about 5 s), and Go forbids process wide environment and
directory changes in parallel tests.

Fix: the xo the tests drive is a process, so give it the test's
environment instead: `setenv(t, k, v)`, `chdir(t, dir)`, `getenv(t, k)`,
and `command(t, name, args...)` (cmd/xo/env_test.go; `run`, `xo`,
`execCLI` use them; subtests inherit the parent's settings by name).
Module worlds: `modSetup(t, src)` (modtest.SetupWith). A test that
really needs the process environment drops `t.Parallel()` and says why.
