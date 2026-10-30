# A serial Go test holds back every parallel test of its package

Status: Observed (2026-10-30)

Symptom: the full tier's cmd step (tier29, 724 s) ran 3 to 13 tests at once for its first
340 s and over 100 after; the llvm step (3317 s) ran 2 to 30 at once from 2105 to 3110 s,
against 100 to 300 before.

Cause: `go test` runs top-level tests in source order (files sorted by name). A test that
calls t.Parallel pauses and resumes only after every serial top-level test of the package
has finished; a serial test's own parallel subtests start when its function returns. So
the serial tests run one at a time: in cmd/xo first, with every parallel test waiting
(18 tests, 332 s), and in internal/llvm, after the golden batches drained, as a chain of
Diff tests nothing overlapped.

Fix: a test that builds or runs programs calls t.Parallel unless it changes process
state: cmd/xo's setenv(t, ...) and chdir(t, ...) are per test (they set the environment
and directory of the processes t starts), so they are safe; t.Setenv, os.Setenv,
os.Chdir and t.Chdir are not (t.Setenv panics after t.Parallel). Scheduler, race,
preemption and constant time tests stay serial. To find the serial ones (a top-level
test with no "pause" event) and the idle stretches they cause:
`python3 scripts/serial-hold.py $logs/STEP.json`, on a scripts/gate.sh step's
`go test -json` log.
