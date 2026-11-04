# Native runs pass a use-after-free of an llrt record that wasm traps on

Status: Observed (2026-11-03)

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

## Fix (proposed, not built)

A debug allocator mode (say XO_FREE_CHECK=1, or under XO_RC_CHECK=1):
alloc.free fills a freed small block with a poison word
(0xdeadbeefdeadbeef) and keeps it in a FIFO quarantine of N blocks
before it goes back to its class's free list; obj.h_free_n and
poll.pd_close clear the object's kind word to a freed tag. A load of a
poisoned handle or pointer then faults on its first use (an address
in the non-canonical range), and alloc on a quarantined block's reuse
checks that the poison is intact (a write after free). The cost is
only in the debug mode; the hot path checks one static word as
COUNT already does.
