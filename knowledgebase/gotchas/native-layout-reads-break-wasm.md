# Raw reads of an object's words assume the native layout and break on wasm

Status: Observed (2026-10-21, Xo Lang 3; three instances in one day)

Symptom: code in std/native (and SSA the compiler builds for std
features) that is correct on arm64 and LLVM gives wrong results on the
wasm backend, with no fault: a TLS 1.3 record's plaintext came back
empty (tls13_go, tls13_rfc8448, tls13_server failed on wasm since
852fbeeb5), and decision 0291's hybrid Map lost writes after a copy on
write.

Cause: a read of a runtime object's header with the native widths and
meanings. The wasm runtime keeps i32 counts, lengths, and capacities
(compiler/wasm gen.xo: a string is {i32 rc, i32 cap, bytes}, a list
{i32 rc, i32 len, i32 cap, pad, elements}), so an 8 byte load at the
capacity's offset read the capacity plus the first data bytes (a byte
of 128 or more made it negative: raw.bytes_len clamped the length to
0); an 8 byte load at a count read the count plus the length; and the
wasm runtime's list.new() allocates where the native backends hand out
the immortal empty list, so "is this the immortal empty list" was never
true there.

Fix: read headers at the runtime's widths, rt.layout(7) bytes for a
count or a capacity (raw.bytes_len, aesgcm cap_of); in the SSA builder,
b.wasm decides (maplog.xo obj_count, list_cap); never infer a state from
an object's identity (immortal or not): mark it explicitly (the hybrid's
mark is an empty list of capacity 5). Test std/native code that reads
raw memory on wasm too: TestSelfhostWasm (merge tier) runs every golden
there.
