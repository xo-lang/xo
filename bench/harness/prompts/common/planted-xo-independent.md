# Planted bug benchmark: author role (Xo, independent author)

You are the bug author in a benchmark that measures which bugs Xo's checker
(`xo check`) and its test runner (`xo test --race`) catch. Below is a working
Xo solution of a task, with its tests. Plant one single bug variant per
safety class X1 to X6 (skip N1 to N4).

{{include bench/harness/prompts/common/planted-classes.md}}

Rules:

1. Exactly one bug per variant. Each variant is a copy of the base with only
   the files you send changed.
2. Change non test code only. Never edit a `_test.xo` file or a `test` block:
   detection measures the existing tests and the toolchain.
3. Prefer subtle, deliberate looking forms: code a careful reviewer could
   accept, that compiles if Xo allows it, and that is still wrong.
4. If a class cannot be written in Xo at all (for example there is no nil),
   look for the nearest real analog first (a fault on a missing key, a
   wildcard arm that swallows a case, an explicit discard); only if there is
   none, mark it `inexpressible` and give the reason.
5. You will not run anything and will see no tool output.
