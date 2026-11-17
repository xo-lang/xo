# A want.txt edit misses the golden's want_wasm.txt and want_windows.txt

Status: Observed (2026-11-17, task #194)

Symptom: main's wasm goldens go red after a push that changed a golden
and passed its own gate. debug_routes failed on wasip1 and wasip2 (the
run prints `mem_total: true`, want_wasm.txt had no such line), and
http_client_trace failed on wasip2 (want.txt gained `DNSStart` and
`DNSDone` lines that wasm never prints).

Cause: a golden with a `want_wasm.txt` or `want_windows.txt` reads that
file on its target instead of want.txt, so a line added to want.txt
alone is missing there. The wasm and Windows runs are not in a std or
compiler gate unless the push asks for them, so the gap shows on main.
On wasip2 the host resolves and dials in one call (hclient.xo,
`netwasi.dial_addr`), so there are no DNS trace events to print.

Fix: when a push edits a golden's want.txt, `ls` its directory and make
the same edit in each `want_*.txt` (or write a `want_wasm.txt` when the
target's output differs, as http_client_trace's drops the two DNS
lines). Run the wasm goldens for it:
`XO_WASM_GOLDENS=NAME go test ./internal/selfhost -run
'TestWasmCommandGoldens|TestSelfhostWasm'` (wasip2 needs wasm-tools and
wasmtime, which the offload hosts have).
