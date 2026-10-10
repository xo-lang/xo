# net/http's HTTP/2 quirks the native server and client must copy

Status: Observed (2026-10-10, decision 0121; TestH2Diff found each)

The native HTTP/2 code (`std/llrt/xo_h2.c`) reproduces net/http's
bundled HTTP/2 (Go 1.26 `h2_bundle.go`), and the Go against native
comparison found these, none of which the RFCs decide:

- **The client keeps its connection.** Go's Transport reuses an HTTP/2
  connection, so its stream ids go up across requests and show in error
  texts (`stream error: stream ID 15; INTERNAL_ERROR; received from
  peer`). A native client with a connection per request said `stream ID
  1`. The native client now keeps idle connections per roots, address,
  and server name, and sends a request again on a new connection when a
  reused one did not take it.
- **GOAWAY's last stream id** counts the stream of the offending frame
  when the frame parsed (an error of `processFrame`, or HPACK and header
  size errors of `readMetaFrame`, which return the frame), but not when
  parsing failed (`Framer` errors return no frame): an even stream id
  gives `last=2`, DATA on stream 0 gives `last=0`. `h2_conn_err` uses
  `c->cur_sid`, set once a frame parsed.
- **HEADERS padding longer than the frame is a stream error**
  (RST_STREAM), the same in DATA a connection error (GOAWAY).
- **After a GOAWAY of an error nothing more is sent on any stream**
  (`scheduleFrameWrite` stops popping the write scheduler), so a
  response finished just then never goes out.
- **A handler's panic resets the stream after what it queued**
  (`handlerPanicRST` goes through the stream's queue): the client sees
  the header and the first data, then RST_STREAM. Resetting at once
  dropped the queued header and turned the client's error into one
  before the header (with the request in its text).
- **The 431 needs the overflow in the last fragment.** A CONTINUATION
  more than twice what the header list may still hold ends the
  connection (CVE-2024-27316), and a truncated list holds nothing more,
  so a list that overflows in an earlier frame is a PROTOCOL_ERROR, not
  431. The check needs the decoded size after each frame: HPACK is
  decoded incrementally (`hp_decode_part`), a field cut by a frame end
  waiting for the next.
- **`hpack.Encoder` names a field by the last static entry of its name**
  (`byName` keeps the last index: `:status` is 14), so `:status 431` is
  `0x4e` on the wire, not `0x48`. Not observable through HTTP, but a raw
  frame test reading the first byte sees it.
- **checkConnHeaders' texts start with `http2: `**:
  `http2: invalid Connection request header: ["upgrade"]`.
- **Frames of different streams interleave in any order** on both
  servers (handlers race the serving loop): a raw frame test compares
  the frames of each stream in order, not the connection's order.

How to avoid surprises: change `xo_h2.c` together with a case in
`internal/llvm/h2_diff_test.go` (raw frames or the program's calls) and
run TestH2Diff a few times (`-count=3`): a case that races shows up as
a flaky difference.
