# TestH2Diff "raw data over the window" raced the server's handler

Status: Observed (2026-10-12), fixed 2026-10-12.

Symptom: internal/llvm TestH2Diff failed on both native backends with one
line different: `raw data over the window: open` where the Go backend
printed `raw data over the window: 1[RST_STREAM FLOW_CONTROL_ERROR], open`.
Seen in a combined full-suite run on dbb8b05 under heavy load, on the
Linux host (bench/results/linux-2026-10-12.md), and on the first
`go test -run TestH2Diff ./internal/llvm` of 2026-10-12 18:05 with the
heavy lock held (other agents running light work).

Cause: not a late frame, as first thought. The case sent 65 DATA frames
of 16 KiB (1 MiB + 16 KiB) on a POST stream and expected the 65th to
overflow the server's 1 MiB window. But both servers give received bytes
back: the native one as each frame arrives (decision 0121, point 5), so
it never reset the stream (a frame dump showed a WINDOW_UPDATE per frame
and no RST_STREAM in 30 s, also with 66 frames); net/http as the
handler reads the body, so it reset the stream only when its handler
fell behind the frames (under load). Waiting longer could not fix it:
in both TestH2Diff runs with a 30 s wait, the Go backend printed `open`
too (the case then passed by accident).

Fix: the case now sends 4095 bytes, then one frame of 1 MiB - 4094
bytes. Both servers hold back window updates under 4 KiB (net/http's
`inflow`, ported to std/llrt/xo_h2.c), so after the 4095 bytes both
windows have 1 MiB - 4095 left whatever the timing, and the second frame
is 1 byte over (under the 1 MiB MAX_FRAME_SIZE both advertise). A probe
gave RST_STREAM FLOW_CONTROL_ERROR on the Go and LLVM backends with and
without a 1 s pause between the frames, and no reset for a second frame
that fits exactly. `go test -count=3 -run TestH2Diff ./internal/llvm`
then passed three times (18.2 s in all, 2026-10-12).

Also, the raw client's `events` no longer ends at a fixed deadline: it
reads at least the case's wait and then until the reply is complete
(every stream it names ended or reset, a GOAWAY followed by the close,
the streams a case names in `needs`), at most 30 s (`h2EventsMax`). So a
frame that comes late under load still counts, while a server that
hangs still fails. Every raw case, the bad preface, the CBC suite, and
the stop with a stream in flight use it; h2Dial waits up to 30 s for the
server's SETTINGS.

Lesson: a protocol case that expects an error must reach it whatever the
peer's scheduling. A flow control overflow is only certain when the
window cannot have been replenished: stay under the peer's update
threshold, then overflow it in one frame.
