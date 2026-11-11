# A preview 1 call the component runtime stubs fails only under wasmtime

Status: Observed (2026-11-11, tier 68).

## Symptom

internal/selfhost TestWasmCommandGoldens/goldens/file_seek (wasip2/wasm
under wasmtime, Linux hosts) exits 1 with `error: s.txt: input/output
error`, while TestSelfhostWasm, the gate.sh `wasm` step and the Mac all
pass the same golden.

## Cause

A wasip2/wasm command runs the std runtime's preview 1 calls through
compiler/wasm/rt6.xo, which stubs every file call as ENOSYS, and
rt8.xo, which replaces the ones it implements over wasi:filesystem 0.2
(`rt8_replaces`). File.seek (decision 0600) added rt4.xo's
`$fs_file_seek` over `fd_seek`, but rt8.xo had no `fd_seek`, so the
stub answered and ENOSYS read as an I/O error. TestSelfhostWasm runs the
preview 1 build under Node, which has a real fd_seek, so no Mac check
built the component.

## Fix

rt8.xo's `$fd_seek` moves the descriptor entry's offset (offset=16),
which its fd_read and fd_write already use; End reads the size from the
descriptor's stat. A new preview 1 call in rt4.xo needs an rt8.xo
version too (and its name in `rt8_replaces`), checked by building the
golden with `--targets=wasip2/wasm` and running it with the flags of
internal/selfhost/wasm_cmd_test.go's commandGolden on a host with
wasmtime (/work/tools/bin on the offload hosts, infrastructure/offload.md).
