# 0196: Windows and Intel Mac on LLVM before the Go compiler is deleted

Status: Accepted (owner, 2026-10-12)

## Context

Decision 0178 stage 7 deletes the Go implementation, and with it the Go
backend. Today the Go backend is the only one that builds windows/amd64,
windows/arm64, and darwin/amd64: the arm64 backend emits darwin/arm64 and
linux/arm64, and the LLVM backend is tested on darwin/arm64, linux/arm64,
and linux/amd64 (knowledgebase/infrastructure/toolchain.md). Deleting the
Go backend first would drop those targets.

## Decision

The Go compiler is not deleted until the LLVM backend supports Windows
(windows/amd64 and windows/arm64) and Intel Mac (darwin/amd64): programs
build, link, and pass the golden suite on each. This is a gate on stage 7
step 4 (deletion) alongside the lead's readiness criteria; it does not
block the freeze, goldens, or bootstrap steps.

## Consequences

- Windows support on LLVM needs a Windows runtime (threads, fibers for the
  scheduler, IOCP and Winsock I/O, files, console, time zones, TLS), PE/COFF
  output (lld-link or our own linker), unwinding, and Windows test hosts.
- darwin/amd64 needs a runtime pack, the x86-64 task switch (exists), and
  test hosts.
- darwin/amd64 is met (2026-10-12, bench/results/intel-mac-2026-10-12.md):
  on the owner's Intel Mac (macOS 26.5.2, Apple clang 17) internal/llvm
  passes, TestGolden 143 of 143 (Go backend, LLVM, and want.txt agree,
  leak check clean), TestGoldenSplit 145 of 145, TestGoldenThreads 108
  of 108, 0 unsupported. It needed no emitter or runtime change: the IR
  carries no triple (clang supplies the host's), the C runtime already
  had the x86-64 fiber switch with Mach-O names, `__ulock`, and kqueue,
  and the LLVM backend needs no runtime pack (it compiles std/llrt with
  the program). Builds are native on that Mac (with the Go driver at the
  time; since the xo1-x86 change xo1 runs there too, built by LLVM);
  the development Mac
  cannot run x86_64 binaries. Test host: `scripts/intel-mac-test.sh`.
  Gap: ASan and TSan do not start there
  (gotchas/darwin-amd64-sanitizers-sigill.md), so those tests run
  without the sanitizer.
- The self hosted toolchain itself runs on both x86-64 hosts
  (2026-10-12, bench/results/xo1-x86-2026-10-12.md): xo1 built by the
  LLVM backend on linux/amd64 and darwin/amd64 passes the LLVM goldens
  169 of 169 on each, and on each it rebuilds itself to a fixed point
  with no Go toolchain. So deleting Go does not strand x86-64
  hosts; release archives for them still need an amd64 build host or
  an LLVM cross compile in xo1 (the results file lists both).
- wasi/wasm32 is not part of this gate; the Xo wasm backend covers wasm.

## Alternatives

- Delete Go first and accept losing the targets: rejected by the owner.
- Keep the Go backend alone after deleting the rest of the Go code: not
  chosen; it keeps the Go toolchain as a dependency.
