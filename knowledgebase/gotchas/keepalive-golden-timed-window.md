# websocket_deadline's keepalive case counted pongs inside a timed window

Status: Observed (2026-11-16), fixed 2026-11-17 (task #182).

Symptom: internal/llvm TestGolden and internal/arm64 TestGoldenRun
websocket_deadline failed once on a loaded host with
`pongs at least 5: false` (and fewer server pings) where want.txt has
`true`.

Cause: the case moved a 150 ms read deadline forward on every pong while
the client pinged every 40 ms, and the server wrote its hello on a 250 ms
timer. The output depended on how many pongs fit before the hello, so a
ping sender stopped for over 150 ms by the scheduler or a busy host ended
the read with a timeout and few pongs. The Mac under 40 CPU hogs never
showed it (120 of 120 runs passed), which is why it looked like a one off.

Fix: make the output an event count, not a time count. The server writes
the hello right after its 50th pong, the client's window is 600 ms
against 20 ms pings, and the program itself has a 200 ms gap at the
fourth ping (a stalled sender), so the golden proves that the deadline
moves with the pongs on every run. The old version fails that gap 20 of
20 runs; the new one passes 20 of 20. For any timing golden: print counts
of events the program orders itself, and inject the stall you fear in the
program rather than hoping the host produces it.
