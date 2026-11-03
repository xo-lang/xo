# A wasm component imported every function of the runtime's import blocks

Status: Observed (2026-11-03, decision 0447 R2h's claim checked with
`wasm-tools component wit`)

Symptom: a wasip2/wasm command that only used std/udp imported
`wasi:sockets/tcp` and `tcp-create-socket`, a TCP only program imported
`wasi:sockets/udp`, and every command imported wasi:cli/environment,
wasi:cli/stdin and wasi:random whether it read them or not. R2h's docs
said a UDP only program asks the host for no TCP; it did. Every network
golden still passed, since the test host grants the whole network.

Cause: compiler/wasm writes each runtime file's imports as one block
(rt9.xo's rt9_imports lists all of tcp, udp, ip-name-lookup and the
streams), strip_runtime drops only uncalled runtime functions, and
asm.xo encodes every import it is given. component.xo's `wanted` reads
the core module's imports, so the component imported every interface in
the block. Two shared functions (the connection shutdown and the
address bytes) also called both kinds' imports, so even a dropped
uncalled import would not have narrowed a network program. And a function in
the function table (a listener's free, `$sk_lis_free`) is kept whenever
its file is, so the TCP drop it calls stayed imported in a UDP program.

Fix: gen.xo `drop_unused_imports` (commands and components) removes an
imported function no other line names, and rt9.xo's shutdown and address
functions include only the branches of the kinds the program calls
(gen's `udp` and `tcp` flags, set from the rt.wasi_* calls in its scan);
the listener's free takes its table slot only in a TCP program.
TestWasmCommandImports pins hello (no sockets), a UDP program (no TCP)
and a TCP program (no UDP).

How to avoid: a claim about what a component imports is checked with
`wasm-tools component wit` on a built program, not read from the code
that decides it; a runtime function shared by several interfaces is
generated per use, or it imports them all; and a table entry roots its
function, so it is added only when that function can be needed.
