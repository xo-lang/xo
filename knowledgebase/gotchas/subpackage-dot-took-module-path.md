# A subpackage named `.` took its module's import path

Status: Observed (2026-11-08)

## Symptom

`xo check .` (or build, test, run) in a package directory below its
module root, whose code imports the module's root package, failed with
`XO0604: import cycle: example.com/app imports example.com/app, which
imports it back`. The same package built from the module root
(`xo run ./admin`) worked. Azure's messaging/azservicebus/admin,
messaging/azeventhubs/checkpoints and storage/azblob/integration hit it;
they looked like a nested module problem but were not.

## Cause

Checker.module_path_for computed the directory's path relative to the
module root with rel_path on two relative paths: the root `..` and the
directory `.`. They share no prefix, so the relative part came out as
`.` and the package took the module's own import path.

## Fix

module_path_for compares absolute paths (Checker.abs). Test: cmd/xo
TestSubpackageFromItsDir. When reducing an XO0604 that names one module
twice, try the same package from the module root first: a difference
there points at path resolution, not at the imports.
