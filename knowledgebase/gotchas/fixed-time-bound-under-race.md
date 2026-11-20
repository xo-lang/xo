# A fixed time bound in a std test fails under --race

Status: Observed 2026-11-20 (task #456).

**Symptom.** `std/native/http_test.xo`'s "a chunked body of many small
chunks in one buffer" test (`expect(took < 150ms)`, 24 ms on the Mac
Studio) failed under `stdproven race http` on host 3. The code was not
slower: ThreadSanitizer slowed the per chunk loop about 7 times.

**Cause.** A wall clock bound fits one build. `--race` (and `--check`,
a loaded host) slows the work, and it slows some work much more than
other work. A Content-Length read of the same bytes as a baseline did
not help: under `--race` it went from 2.6 to 3.6 ms while the chunked
read went from 24 to 131 ms, because the per chunk loop is what TSan
instruments.

**Fix.** Bound the time against a baseline measured in the same run
that does the same work per unit and leaves out only the cost under
test. Here that is the same chunked body behind a 1 byte header (a small
buffer, no per chunk copy): `took < small * 3 + 20ms`. Mac Studio:
18.5 against 15.3 ms, `--race` 136 against 134 ms, the copying client
505 against 22 ms (fails). An allocation ceiling
(`task.with_budget(alloc:)`) did not catch the copying client: the
test passed with the fix reversed.
