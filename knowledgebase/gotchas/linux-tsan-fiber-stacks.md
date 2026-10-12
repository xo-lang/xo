# ThreadSanitizer on Linux runs out of address space for 64 MiB fiber stacks

Status: Observed (2026-10-12, EC2 c8g, Amazon Linux 2023, kernel 6.18,
clang 15.0.7), fixed in std/llrt/xo_rt.c (fibers_init).

Symptom: on linux/arm64, the ThreadSanitizer builds of
TestGoldenThreads/conc_stress, TestGoldenThreads/clock_timers,
TestGoldenTasks/conc_stress, and TestRuntimeEventLoop exit 70 with
`fault: out of memory`. (TestH2Sanitizers/thread, "connection refused",
also passed once this and the memcpy fix landed; its cause was not traced
separately.)
The same tests pass on the Mac, and the same programs pass on Linux
without the sanitizer.

Cause: mmap of a fiber stack (64 MiB reserved, MAP_NORESERVE) failed with
ENOMEM after about 880 fibers. ThreadSanitizer on Linux reserves nearly
all of the 48 bit address space as PROT_NONE for its shadow and meta
memory and leaves the program a few application ranges (about 96 GiB on
arm64: /proc/self/maps at the failure showed 262143 GiB mapped, with
`6000000000-aaaa00000000 ---p` and `aaaf00000000-ffff00000000 ---p`).
conc_stress and evloop.c park 2000 tasks at once (2000 fibers, 125 GiB of
stacks). Neither overcommit (mode 0 honors MAP_NORESERVE) nor
vm.max_map_count (65530, about 32,000 fibers) was the limit: a plain C
loop mapped 32,754 such stacks on the same host.

Fix: under ThreadSanitizer on Linux (XO_TSAN and __linux__) the default
task stack is 8 MiB (room for thousands of fibers); XO_TASK_STACK_MB still
overrides. Programs that need deep recursion are tested without the
sanitizer (deepRecursion in internal/llvm/threads_test.go).

How to find it next time: a failing `mmap` is one of about 100 xo_oom
call sites, all printing the same message. Wrap xo_oom with the call
site (`__FILE__`, `__LINE__`, errno) and dump /proc/self/maps entries
over 1 GiB; the remote-full.sh `--hold` instance is where to do it.

Related: a kernel with 39 bit addresses (512 GiB) would hit the same
limit without any sanitizer at about 8000 parked tasks.
