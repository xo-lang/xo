# TestGrpcFuzz failed on every call: the reference server sent an invalid MAX_FRAME_SIZE

Status: Observed (2026-11-22, task #695)

## Symptom

TestGrpcFuzz failed at every seed: 44 of 50 calls differed at seed 2, and on
Xo's side every call ended with status 13 and the message
`Post "http://127.0.0.1:PORT/f.S/M": connection error: PROTOCOL_ERROR`
(printed hex encoded). grpc-go's side read the canned responses normally.
v0.55.0 failed the same way; 9ef0e8e27 (the 2026-11-18 fuzz evidence) passed.

## Cause

internal/stdtest/grpcref canned.go opened each connection with
`SETTINGS_MAX_FRAME_SIZE = 1 << 24` (16777216). RFC 9113 section 6.5.2 allows
16384 to 16777215 and makes any other value a PROTOCOL_ERROR. Since 0017d94aa
(2026-11-19, GO-2026-4918) std/http's HTTP/2 client checks SETTINGS as Go's
`Setting.Valid` does, so it refused the connection. grpc-go's transport does
not run that check and accepted the value, so the clients diverged on every
call. The harness was at fault, not either client.

## Fix

canned.go sends `1<<24 - 1`. When a reference server is hand written with
`http2.Framer`, keep every SETTINGS value it sends inside
`http2.Setting.Valid`. After a client starts validating a peer's input, rerun
the differential harnesses that use that client against hand-written servers.
