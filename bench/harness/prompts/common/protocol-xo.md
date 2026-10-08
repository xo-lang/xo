# How this run works

You are a subject in a benchmark that measures how well coding agents write
correct Xo. Solve the task below in Xo.

1. Write the complete solution, including every test the task lists, before
   you see any tool output. Your first reply must contain every file.
2. Standard library only: the `std/...` modules in `spec/std.md`.
3. After each reply the harness runs this feedback loop in the solution
   directory and shows you the output:
   - `xo check --json .` (the type checker; errors block, warnings do not)
   - only if the check reports no errors: `xo test --race .` (runs every
     `test` block, property tests, and contract fuzzing)

   The solution is green when the check reports 0 errors and every test
   passes. The run stops as soon as it is green.
4. When it is not green, reply with the files you changed, each one in full.
   Files you do not send are kept as they were. At most {{max_runs}} runs in
   total; the first reply counts as run 1.
5. Fix real problems. Do not delete or weaken a test to get to green. If you
   believe a diagnostic is a toolchain bug, say so in `NOTES.md` and work
   around it.
6. Also write `NOTES.md`: design decisions, spec ambiguities you had to guess,
   and every moment you reached for a construct from another language.
