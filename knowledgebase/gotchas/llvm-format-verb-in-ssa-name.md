# SSA names like `%t3` are format verbs

Status: Observed (2026-10-08, internal/llvm, first program)

Symptom: clang rejected the generated IR: `%!t(string={})3 = load %t1,
ptr %!s(MISSING)`.

Cause: the emitter named temporaries `%t1`, `%t2`, ... and built
instructions as `fmt.Sprintf(name + " = " + format, args...)`: the
temporary's name became part of the format string, and `%t` is Go's
bool verb.

Fix: `fn.def` passes the name as an argument
(`emit("%s = "+format, append([]any{t}, args...)...)`). Any text that
contains `%` (temporaries, labels, string constants) must go through an
argument, never be concatenated into a format; literal `%` in a format is
written `%%` (`"getelementptr %%xo.list, ..."`).
