# External bytes reach programs as a Str that is not UTF-8

Status: Observed (2026-10-18, FuzzHTTPDiff; Xo Lang 3). Fixed the same
day: decision 0259 (Accepted).

Symptom: a native HTTP server answered `0 /status/\x82 HTTP/1.0` with
500 where net/http answered 200: the handler's `req.path[8..]` faulted
("slicing a Str inside a UTF-8 sequence"). The 0.19.0 seed does the
same, so it is not new.

Cause: spec/core.md says `Str` is UTF-8 text, but native std builds `Str`
from bytes it read without validating them (`raw.str_at`,
`raw.str_slice`, `rt.cast[Bytes, Str]`): the HTTP request line, header
values, percent-decoded query values, `Fs.read_text`, file names, stdin
lines, env and args, archive names (decision 0259 lists them). Only
`Bytes.to_str()` (None) and JSON (U+FFFD) validate.

Seen inside std too: TestHTTPFuzz/client's redirect to
`http://\x82\x82:1/x` faulted in netpolicy.split_hostport (`slice 0..2
of Str splits a UTF-8 sequence`), ending the client; fixed by byte copies
(raw.str_slice), TestHTTPClientRedirectNotUTF8. A Str slice faults when
the byte at either end is a continuation byte (prelude.xo
llrt_str_slice_at), so std code that slices outside text at offsets it
found on the bytes must copy bytes instead.

Fix: decision 0259: every boundary makes outside bytes valid UTF-8
(U+FFFD; percent escapes in URL parts) through scan.xo's helpers (std/rt
text.go on the Go backend); internal/repotest TestOutsideBytesValidated
keeps it so.
