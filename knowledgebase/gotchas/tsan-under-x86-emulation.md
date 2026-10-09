# ThreadSanitizer cannot start under x86-64 emulation on Apple silicon

Status: Observed (2026-10-09)

Symptom: in a `--platform linux/amd64` container on an arm64 Mac
(Docker Desktop with Rosetta: `/proc/cpuinfo` says `VirtualApple`), every
ThreadSanitizer binary exits 66 with `FATAL: ThreadSanitizer: memory
layout is incompatible, even though ASLR is disabled`. TestRuntimeThreads,
TestRuntimeTasks, TestRuntimeEventLoop, TestGoldenThreads, and
TestGoldenTasks all failed. AddressSanitizer and UBSan run fine there.

Cause: TSan maps its shadow memory at fixed x86-64 addresses that the
emulator's address space layout does not leave free. Not a runtime bug:
the same tests pass natively on linux/arm64.

Fix: `tsanBuild` in `internal/llvm/threads_test.go` first builds and runs
an empty program with `-fsanitize=thread` (`sanitizerRuns`, once per test
binary); when that builds but cannot run, the tests compile without the
sanitizer and still run the programs (logged with `-v`). Real x86-64 TSan
coverage needs an x86-64 Linux machine (a CI runner).
