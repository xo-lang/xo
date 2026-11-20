# Go's HTTP/2 client: "client conn could not be established" in place of the connection error

Status: Observed 2026-11-20 (task #482)

## Symptom

A golden with a raw h2c server that sends a bad SETTINGS frame at once
(http_h2_client_bad_settings) printed, on the Go backend in 2 of 10 runs
on main 15a140263, `http2: client conn could not be established` for one
case in place of `connection error: PROTOCOL_ERROR`. Native and LLVM were
steady.

## Cause

Go's transport (net/http h2_bundle.go, Go 1.26.11): the read loop rejects
the SETTINGS and closes the ClientConn. If that happens before RoundTrip
reserved stream 1, awaitOpenSlotForStreamLocked sees `closed &&
nextStreamID == 1` and returns errClientConnNotEstablished, which aborts
the retry and hides the read loop's error. Which one a request sees is a
timing race in Go itself, so the Go backend (which uses Go's transport)
inherits it.

## Fix

Make the server's bad frames depend on the request: the golden's server
reads until the client's HEADERS frame for stream 1 arrived, then writes
the SETTINGS. A test of a server's first frames against Go's client
should wait for the request the same way. 20 of 20 on TestGolden and
TestGoldenGoParity after.
