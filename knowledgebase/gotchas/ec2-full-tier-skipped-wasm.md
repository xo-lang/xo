# The EC2 full runs skipped TestSelfhostWasm (no Node)

Status: Observed (2026-10-13), fixed.

Symptom: a change that dropped the wasm goldens from 131 to 70 passing
(the wasm assembler lacked i64.rotr after a digest port) went through
the lead's batch 4 full run on EC2 with internal/selfhost `ok`.

Cause: TestSelfhostWasm runs the wasm goldens under Node's WASI and
skips when `node` is not on PATH; scripts/remote-full.sh's instance
setup did not install Node, so every EC2 batch skipped the wasm tier
silently.

Fix: remote-full.sh installs `nodejs` and prints the Node version in the
log header ("MISSING" when absent). When reading a batch log, check the
header's node line; a skip of TestSelfhostWasm in a full run is a gap,
not a pass.
