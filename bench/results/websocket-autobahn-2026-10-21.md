# std/websocket against the Autobahn TestSuite (2026-10-21)

Decision 0313 stage 3: `bench/websocket/autobahn/run.sh OUT both` runs
the Autobahn TestSuite (crossbario/autobahn-testsuite, linux/amd64
under Docker Desktop's emulation on the Mac) against std/websocket:
the fuzzingclient against `bench/websocket/autobahn/server` (an echo
server, messages up to 64 MiB) and the fuzzingserver against
`bench/websocket/autobahn/client` (std/websocket's `dial` for each
case), cases 1 to 7 and 9 (300 cases; 10 and 12 to 13 are compression
and the like, which std/websocket does not negotiate). Main 8ac517e6
plus this harness; BACKEND=go builds with the Go backend.

| side | backend | result |
|---|---|---|
| server (fuzzingclient) | arm64 | INFORMATIONAL 3, NON-STRICT 4, OK 293 |
| client (fuzzingserver) | arm64 | INFORMATIONAL 3, NON-STRICT 4, OK 293 |
| server (fuzzingclient) | go | INFORMATIONAL 3, NON-STRICT 4, OK 293 |
| client (fuzzingserver) | go | INFORMATIONAL 3, NON-STRICT 4, OK 293 |

No case fails. The cases that are not OK, the same on every row:

| case | behavior | what |
|---|---|---|
| 6.4.1 | NON-STRICT | invalid UTF-8 in the 2nd of 3 fragments: the close comes at the message's end, not at the bad fragment (fail fast is optional) |
| 6.4.2 | NON-STRICT | as 6.4.1, the 2nd fragment ending at the bad byte |
| 6.4.3 | NON-STRICT | as 6.4.1 in 3 chops of one frame |
| 6.4.4 | NON-STRICT | as 6.4.2 in 3 chops of one frame |
| 7.1.6 | INFORMATIONAL | a 256 KiB message, then a close, then a ping: undefined order of the echo and the close |
| 7.13.1 | INFORMATIONAL | close code 5000: undefined by the RFC |
| 7.13.2 | INFORMATIONAL | close code 65535: undefined by the RFC |

No case fails, so std/websocket meets decision 0313's stage 3 bar
(every case passes or is non-strict). The non-strict cases are the
optional fail-fast ones: std/websocket checks a text message's UTF-8
when the message is complete, so a bad first fragment is answered
(close 1007) at the end of the message, not at the fragment.
