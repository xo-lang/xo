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
- wasi/wasm32 is not part of this gate; the Xo wasm backend covers wasm.

## Alternatives

- Delete Go first and accept losing the targets: rejected by the owner.
- Keep the Go backend alone after deleting the rest of the Go code: not
  chosen; it keeps the Go toolchain as a dependency.
