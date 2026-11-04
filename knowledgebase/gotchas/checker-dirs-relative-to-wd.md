# The checker's module directories are relative to the working directory

Status: Observed (xo-lang-4, 2026-11-04).

## Symptom

A new XO1306 for a nested module's import (aws report entry 7) fired in
a cmd/xo test, which runs `xo check <abs dir>/...` from the test's own
directory, and never in the report's repro, which runs `xo check ./...`
from inside the module.

## Cause

compiler/check keeps paths in display form (`Checker.display`,
`rel_dir`): a ModRoot's `dir` below the working directory is relative,
and the module at the working directory is `"."`. A test like
`owner.dir.starts_with("${from.dir}/")` compares `"service/sts"` with
`"./"` and never matches from inside the module, while the absolute
paths of a run from elsewhere do.

## Fix

Compare `c.abs(a)` with `c.abs(b)` whenever two checker directories are
related by prefix, and give the test both forms: an absolute path from
outside and a `chdir(t, dir)` subtest running `./...` inside
(cmd/xo TestNestedModuleRequired).
