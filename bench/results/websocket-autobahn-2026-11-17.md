# std/websocket against the Autobahn TestSuite with permessage-deflate (2026-11-17)

Decision 0313 stage 3, rerun with compression (task #235): the harness
(`bench/websocket/autobahn/run.sh OUT both`, BACKEND=go for the Go rows)
now runs cases 12 and 13 (permessage-deflate) as well as 1 to 7 and 9.
The echo server upgrades with `compress: true` (it answers an offer;
cases without one are unchanged) and the client dials with `compress:
true`. crossbario/autobahn-testsuite 25.10.1 (linux/amd64 under Docker
Desktop's emulation on the Mac); std/websocket at a2158401f (deflate)
plus this change (dial takes an answer without client_no_context_takeover).

| side | backend | cases | result |
|---|---|---|---|
| server (fuzzingclient) | arm64 | 516 | OK 513, INFORMATIONAL 3 |
| server (fuzzingclient) | go | 516 | OK 513, INFORMATIONAL 3 |
| client (fuzzingserver) | arm64 | 516 | OK 441, INFORMATIONAL 3, UNIMPLEMENTED 72 |
| client (fuzzingserver) | go | 516 | OK 441, INFORMATIONAL 3, UNIMPLEMENTED 72 |

No case fails. INFORMATIONAL: 7.1.6, 7.13.1, 7.13.2, as on 2026-10-21.
UNIMPLEMENTED: the client cases 13.3 to 13.6 (72), which need the client
to offer `client_max_window_bits` or context takeover; gorilla's client
offers neither (only no context takeover both ways), and std/websocket's
offer is gorilla's.

## The client and an answer without client_no_context_takeover

The first client run failed 12.1 to 12.5 and 13.1 (108 cases): the
fuzzingserver answers the offer with `permessage-deflate;
server_no_context_takeover` only, and dial refused it as gorilla's Dialer
does (errInvalidCompression). RFC 7692 7.1.1.1 lets a server leave
client_no_context_takeover out: the client then may keep context, and a
client that never does (std/websocket's) is still correct. dial now
refuses only an answer without server_no_context_takeover (the server's
messages could not be inflated one at a time) and takes the rest; the
client rows above are after that change (test "dial refuses a server that
keeps context, and takes an answer that lets the client keep it").

## The fuzzing client's connect timeouts

Three of the first eight full arm64 server runs stopped early (after
287, 307 and 415 cases) with the fuzzing client's `User timeout caused
connection failure`, as one run did on 2026-10-21 under load; the cases
run before the stop were all OK. In each, the server was alive and
idle when the run stopped: it accepted a connection at once, its open
files stayed at 11 or 12 through the run (no leak), and 1,500
sequential handshake and echo connections from the host (also with the
request head split across writes) never took over 44 ms. Runs that
sampled a handshake every 2 s from the host saw at most 77 ms and no
stall. Eight later arm64 server runs (one of cases 9, 12 and 13 only,
seven of every case, three of them with the 2 s probe) and three Go
backend runs all finished every case. The timeout is on the emulated
client's side of Docker's networking, under load; the rows above are
full runs.
