# net/http reads a bodiless response's extra bytes as the next response (Go backend)

Status: Observed (2026-11-14, gate deadw-(private commit); reproduced 2026-11-15)

## Symptom

TestHTTP1ClientDiff fails now and then on a loaded host, on the Go
backend only: response 438 answers
`Get "http://ADDR/r/438": ... malformed HTTP response "yyyy..."`
where the golden has the redirect to /r/25. The same gate log shows
`dial tcp \xff\xff:1: no such host` just below, but that is
TestHTTPClientRedirectNotUTF8's t.Log from a passing test, not the failure
(the gate failure audit read it as one, row 16).

## Cause

Response 437 is a 204 followed by 43 bytes. net/http reads no body for a
204 and puts the connection back in its idle pool; its read loop then
peeks, sees the bytes, and closes the connection ("Unsolicited response
received on idle HTTP channel"). When the next request takes the idle
connection before that peek runs, net/http reads the bytes as its
response. A request header `Connection: close` does not stop the reuse
(net/http checks `Request.Close`, which std/rt sets only for CONNECT), and
std/rt shares one Transport across clients, so neither a header nor a
client per request avoids it.

## Fix

The golden is each response's answer on a fresh connection:
retryHTTP1GoReuse replays a differing Go backend line in a fresh process
(START=i N=i+1), up to three times. The native backends get no replay, so
a reuse bug of theirs still fails. TestHTTP1ClientGoReuseReplay forces
the race (the server sends the extra bytes 20ms after the head) and checks
the replay.
