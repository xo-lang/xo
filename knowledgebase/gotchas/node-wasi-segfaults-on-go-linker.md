# Node's WASI crashes running the Go linker built for wasip1

Status: Observed (2026-10-10)

Symptom: `node runwasi.mjs link.wasm ...` (node:wasi, Node 22.22) exits with
status 139 (segfault) after printing the link header, writing an empty
output file. The same `link.wasm` and arguments succeed under wazero and in
Chrome with `@bjorn3/browser_wasi_shim`.

Cause: not found; the crash is inside Node (its uvwasi layer or V8), not in
the linker.

Fix: test wasip1 toolchain binaries with wazero
(`go install github.com/tetratelabs/wazero/cmd/wazero@v1.8.2`,
`wazero run -mount=.:/w link.wasm ...`) or in a browser, not node:wasi.
