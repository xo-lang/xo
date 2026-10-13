# The LLVM differential test accepts only `@.` globals

Status: Observed (2026-10-13, decision 0206 stage 2).

Symptom: `TestSelfhostLLVM` (-short) failed with `the Go emitter's IR:
unexpected line "@xo_sys_words = weak global [16 x i64] ..."` though
both emitters wrote the same line.

Cause: internal/selfhost frozen_test.go splitIR sorts a module's lines
by their start (`declare`, `%` types, `!` metadata, `define`, and
globals only as `@.`); any other top level line is an error.

Fix: name emitter globals with a leading dot (`@.xo.sys_words`), as the
string constants are, or extend splitIR.
