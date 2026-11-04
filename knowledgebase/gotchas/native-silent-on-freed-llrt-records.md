# Native runs pass a use-after-free of an llrt record that wasm traps on

Status: Observed (2026-11-03); fixed by a check mode (decision 0484)

## Symptom

Golden http_reverse_proxy_ws trapped 5 of 5 as a wasip2 command
(wasmtime: "unknown handle index 10774209" in rt9's wasi_sock_write,
the index a heap address that changed per run) and passed 5 of 5 on
native arm64, XO_RC_CHECK=1 included, with identical output.

## Cause

The client stream's end (hclient.end_keep, then close_at) released the
connection object (tasks.h_release), closed its poll descriptor, freed
its buffer and cleared its record while the proxy's tunnel task still
wrote on it (gotchas/proxy-body-reader-not-replayable.md, follow-up).
On wasm the next write read a resource handle out of freed memory and
wasmtime's handle table rejected it. Native has no such table: a freed
llrt block (a Conn, a poll descriptor, a buffer) still holds plausible
words or is reused by the next allocation of its class, so the write
goes through or fails quietly. Nothing in the native debug modes
catches it: XO_RC_CHECK=1 only checks decision 0322's uniqueness
proofs (Reuse checks), XO_LEAKCHECK only counts what is never freed,
and XO0915 (`xo vet --cost`) follows a raw address inside one block,
not a record shared with another task.

## How to avoid

- A record another task writes on (a tunnel, a full duplex sender)
  is closed with end_shared and freed with the last reference, never
  with end_keep or end.
- Run a network or concurrency golden as a wasip2 command
  (`XO_WASM_GOLDENS=NAME go test ./internal/selfhost -run
  TestWasmCommandGoldens`, on an offload host) when a change moves a
  connection's lifetime: it is the one backend that traps on this.
- TSan (`--race`) sees it only when the two tasks run on two threads.

## Fix

Decision 0484: XO_FREE_CHECK=1 poisons freed heap blocks (held in a
per thread ring before reuse) and a closed connection's fields
(heapos.retire, from hclient.xo close_at), so this bug crashes on native
at the use (tlsio.write < hclient.send_chunk < http.tunnel). The
cause turned out to be a use after close, not after free: close_at
zeroed the fields of a record that lives on, and zero is a valid
descriptor (and a valid wasm address). Run a network or concurrency
golden under XO_FREE_CHECK=1 when a change moves an object's or a
connection's lifetime.
