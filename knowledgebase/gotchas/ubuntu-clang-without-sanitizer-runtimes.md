# Ubuntu's clang has no sanitizer runtimes

Status: Observed (2026-10-09)

Symptom: on a fresh Ubuntu 24.04 EC2 machine (arm64 and amd64) with
`apt-get install clang lld llvm build-essential`, every sanitizer test of
internal/llvm failed at link time: TestRuntimeThreads, TestRuntimeTasks,
TestRuntimeEventLoop, TestGoldenThreads, TestGoldenTasks (ThreadSanitizer)
and TestGoldenASan (AddressSanitizer), 82 TSan and 6 ASan builds, with

    /usr/bin/ld: cannot find /usr/lib/llvm-18/lib/clang/18/lib/linux/libclang_rt.tsan-x86_64.a: No such file or directory

Cause: on Ubuntu the compiler-rt archives live in `libclang-rt-18-dev`,
which `clang` (18.1.3) only suggests. Debian's image used by
`scripts/linux-test.sh` installs `libclang-rt-dev` explicitly, so the
Docker runs never hit it. `-fsanitize=thread` compiles fine, so the test
harness's "no sanitizer" skip (it looks for `unsupported option`) does not
fire, and the probe in `sanitizerRuns` treats a failed build as "the real
build reports it".

Fix: install `libclang-rt-<clang major>-dev` (`infra/ci-aws/remote-setup.sh`
does). `build.ClangError` now appends a hint naming the package when
clang's output says a `libclang_rt.*` archive is missing
(`internal/build/clangerror_test.go`). The tests still fail rather than
skip: a CI machine without the runtimes must not lose sanitizer coverage
silently.
