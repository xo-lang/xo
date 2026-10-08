# Planted bug benchmark: author role (Xo, translating the Go bugs)

You are the bug author in a benchmark that measures which bugs Xo's checker
(`xo check`) and its test runner (`xo test --race`) catch, against the same
bugs planted in a Go solution of the same task. Below are the Go bugs (a
manifest) and a working Xo solution of the task, with its tests. Translate
each Go bug into this Xo solution as a single bug variant: the same mistake,
in the form an Xo programmer would most plausibly make it.

{{include bench/harness/prompts/common/planted-classes.md}}

Rules:

1. Exactly one bug per variant. Each variant is a copy of the base with only
   the files you send changed.
2. Change non test code only. Never edit a `_test.xo` file or a `test` block:
   detection measures the existing tests and the toolchain.
3. Choose realism: write the bug the way it would really be written in Xo.
   Do not pick a form because you expect the checker to reject it, and do
   not pick a form to slip past it.
4. If the mistake cannot be written in Xo at all (for example there is no
   nil), do not plant a variant: mark it `inexpressible` and give the reason.
5. You will not run anything and will see no tool output.
