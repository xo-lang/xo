# A canceled task waiting on a constant's build read past an empty list

Status: Observed (2026-11-12, Xo Lang 3; fixed the same day).

## Symptom

A program whose tasks read a heap constant (a `const` List, Map or other
collection) crashed with `SIGSEGV ... at address 0x910160a9aa0003ed` (two
arm64 instructions read as a pointer) about 1 run in 30, on the native
backend and LLVM, never on Go. The fault pointed at the constant's read
(`DOCS[i]`). It happened only when one task failed while others were
reading the constant for the first time. `--race` builds aborted inside
TSan's `MemoryAccess` at the same read, 8 runs in 10. Seeds 0.48.2 and
0.49.1 had it too. Reading the constant once in main before the tasks
started hid it.

## Cause

consts.xo `once` parks a reader that finds the constant being built by
another task. When the reader's task is canceled while it waits (a
sibling failed, so its scope cancels it), `once` returns the immortal
empty list with the context unwinding, as its comment says. The code
generated after the read (compiler/ssa show.xo const_value) had no
unwinding check: it went on to `list.index(l, 0)` on the empty list and
read the element slot past the static empty list, which held code bytes.

## Fix

show.xo `const_wait_check`: in a program with concurrency, an
`rt.unwinding` check follows the `rt.const_once` call, on the slow path
only (a built constant's inline read cannot be canceled, so the common
read costs nothing). Golden `const_wait_canceled` (a slow initializer, 7
readers, a sibling failing at 20 ms) faulted 30 of 30 runs before the fix
and passes 30 of 30 after.

## Avoid

A runtime call that can return early on cancellation with a placeholder
value needs an unwinding check at its call site before the value is
used. When adding such a call, use `check_then` after it.

## The report stopped before its frames

The crash report printed its first line and the registers, then the
process died with no `frames:` line or traceback. Two tasks had faulted
at the same read at the same moment. The handlers were installed with
SA_RESETHAND, so the first fault set the default action for the whole
process, and the second thread's fault ended it while the first thread
was still writing. crash.xo now installs them without SA_RESETHAND: a
second faulting thread parks in on_crash, the reporting thread sets the
default itself when done, and on_alarm sets it before its kill.
TestCrashReport's `tasks` kind (four tasks faulting at once) lacked the
traceback 20 of 20 runs before and has it after.
