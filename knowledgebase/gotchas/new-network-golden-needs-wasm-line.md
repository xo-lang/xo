# A new golden that uses the network needs a wasm_unsupported line

Status: Observed (2026-11-17, Xo Lang 3)

Symptom: a gate fails in its first step (repo, internal/repotest) with
`TestWasmUnsupportedListsNetGoldens: golden NAME calls the network (main.xo)
but is not listed in internal/selfhost/testdata/wasm_unsupported.txt`, and
fail-fast skips every later step (llvm, arm64), so the gate proves nothing
else. Three branches hit it on one day (goldens http_set_header,
grpc_creds, grpchealth_calls); a local `xo run` of the golden and the
module's tests pass.

Cause: WASI preview 1 has no sockets, so every golden that listens or
dials must be listed for wasip1 (TestSelfhostWasm expects it to fail
there), and the repo test checks each golden's main.xo for network calls.
`xo dev new-golden` does not add the line.

Fix: with each new network golden, add
`NAME # wasip1 # network: the network (\`rt.net_listen\`): WASI preview1 has no sockets`
to wasm_unsupported.txt in the same commit (wasip1 only: the wasip2
component has sockets and must pass it). Before a gate, a local
`go test ./internal/repotest -run TestWasmUnsupportedListsNetGoldens`
takes under a second. See also [wasm-two-unsupported-lists.md](wasm-two-unsupported-lists.md).
