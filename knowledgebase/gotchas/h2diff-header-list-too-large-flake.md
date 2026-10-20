# TestH2Diff: "raw header list too large" answers RST_STREAM now and then

Status: Observed (2026-10-20, Xo Lang 3); cause not confirmed.

## Symptom

internal/llvm TestH2Diff fails about one run in ten with one line:

    go:     raw header list too large: 1[HEADERS 431 DATA 63 END], open
    native: raw header list too large: 1[RST_STREAM PROTOCOL_ERROR], open

The Go reference answers a header list over the server's limit with 431;
the native server sometimes resets the stream with PROTOCOL_ERROR
instead. Twenty runs each: 1 failure on a3425f02 (before f8d367b7's
HTTP/2 header changes) and 2 on f8d367b7, so it predates them; it also
fails on the arm64 gate's base, not only under load.

## Cause (theory)

The test's header block is large, so it reaches the server in more than
one read. h2srv's fragment decodes what has arrived (hpack decode_part)
and h2w.field_keep tracks the list's size and a protocol error in one
state (C_HF: the size left, a too-large flag, an error flag). When the
block splits at certain points, the overflow may be seen in a later
fragment in a way that sets the error flag rather than the too-large one
(a field cut at the fragment boundary, or a size accounted twice), and
block_end then answers PROTOCOL_ERROR instead of 431. The split depends
on how the client's frames arrive, which is why it is intermittent.

## Fix

Not yet. To confirm: drive the case with the header block split at each
possible byte offset (a test client writing the HEADERS and CONTINUATION
frames in chosen pieces), and compare the answer with Go's for every
split; then fix field_keep's accounting across fragments. Until then a
TestH2Diff failure on exactly this line is this flake; any other line is
not.
