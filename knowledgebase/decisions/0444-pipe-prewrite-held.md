# 0444. http.pipe: writes before the first read are held

Status: Accepted (owner, 2026-10-30, with a follow-up: cap the bytes held
before the first read, past which a write waits as it does after it).
Proposed by Xo lang 5

## Context
http.pipe() (decision 0433) is Go's io.Pipe: each PipeWriter.write waits
until the reader has taken the piece. The natural order for a request
body, "write the first message, then Client.open", in one task, never
returns: the reader is the client's sender, which only open starts.
Go hangs the same way. Found writing a gRPC client (survey 2026-10-30,
item 3: a silent hang, not an error).

Measured 2026-10-30 (a probe, one task writing then opening, with and
without a server task in the process): the Go backend waited forever;
arm64 and LLVM returned, because the native pipe is a channel of one,
so their first write was held and a second would have waited. The
backends disagreed and none was safe.

## Decision
Until the BodyReader's first read, the pipe holds what is written and
write returns at once, up to 64 KiB held (owner's follow-up on
acceptance, 2026-10-30); a write that would pass the cap, and from the
first read on every write, waits for the reader as before. close and close_with_error after held pieces end the
body after them. Same on every backend (the Go backend wraps io.Pipe
with a pre-read queue).

## Consequences
- write-then-open in one task works for a first message of up to 64 KiB;
  a writer task that runs ahead of a late open holds at most 64 KiB in
  memory, then waits (golden http_pipe "cap": 8 writes of 32 KiB from a
  task stop before open, then all 256 KiB arrive). A larger first
  message written before open in the same task waits forever, as with
  Go's io.Pipe. 64 KiB is Go's net/http-sized buffer scale (the default
  HTTP/2 flow control window is 64 KiB less one byte), enough for a
  first gRPC or JSON message.
- Back-pressure is unchanged once the body is being sent.
- Not Go's io.Pipe for the pre-read window; spec std http's pipe line and
  duplex paragraph say so. Test: internal/llvm golden http_pipe
  ("prewrite": two writes and a close, then open, in one task; every
  backend; the golden runner's timeout bounds a hang).
- The native pipe still holds one piece in its channel after the first
  read (Go's waits for each); a writer may run one piece ahead there.

## Alternatives
- Fail the write when no reader is attached: a writer task started
  before open (a legitimate order) would fail by timing.
- An unbounded pre-read buffer (the first version): a writer task that
  runs ahead of a late open grows memory without end.
- A larger cap (1 MiB): more memory per pipe for little gain; a first
  message past the cap hangs either way.
- A runtime "write with no reader" fault after a delay: timing based.
