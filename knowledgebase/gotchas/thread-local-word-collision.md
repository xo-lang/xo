# Two runtime modules on one thread local word

Status: Observed (2026-10-15, the scheduler perf work on zero-c)

Symptom: tasks.xo run_claimed was changed to keep its context as the
thread's spare in thread local word 12. The goldens passed, but an xo
built with the change exited with status 137 (sometimes 133) and no
message on every `xo build`, even with XO_THREADS=1, while passing under
lldb; one full LLVM golden run faulted fs_ops_edges with a false XO-F001
deadlock. The first guess was a late write into a finished task's
context (a waker still in park_signal): taking the park's lock before
reuse and moving the spare to the Xo heap changed nothing, and an
AddressSanitizer run of 25 concurrency, fs and net goldens with
contexts from malloc and free (no reuse) found no late write.

Cause: word 12 was already sysio.xo's (TL_FS_MSG, the heap_large block
of the last file operation's error message). Every fs error stored its
message there, and the next task run took that block as its spare
context: a run filled the context's 0xAB debug pattern and found "No
such file or directory" and a path in it. The search for free words had
looked for `TLS_` names and literal indexes and missed sysio.xo's `TL_`
names; std/native/llrt/sched.xo's comment on the table already listed
words 11 and 12 as sysio.xo's.

Fix: the spare is word 13, sched.xo's table names it, and
internal/repotest TestThreadLocalWordOwners resolves every word the
native runtime touches (rt.tlocal_get and tlocal_set arguments, byte
offsets from rt.tlocal_addr) to the files touching it and fails when a
file uses a word it does not own (it fails on word 12 in tasks.xo).
Read sched.xo's table before taking a word.
