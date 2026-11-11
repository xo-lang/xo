# 0598: std/textproto, the text protocol reader std/mail, std/multipart and std/smtp share

Status: Accepted (owner, 2026-11-11 morning, via the lead; proposed by Xo Lang 3, 2026-11-11; the lead's queue: the second gap
of the function level audit, bench/results/std-audit-2026-11-11.md).
Related: 0597 part 3 (shared parsers made public as in Go), 0588 (one file
per std module), 0276 (std follows Go's standard library), 0259 (outside
bytes stay `%XX`).

## Context

The std audit of 2026-11-11 ranks net/textproto second among Go's
packages Xo lacks (60 points: `MIMEHeader.Get`, `TrimString`,
`CanonicalMIMEHeaderKey`, `NewReader` and 16 more, 56 stdlib files and 2 of
the pinned modules). Xo has the code three times over, private:

- std/mail.xo: `canonical_key` (CanonicalMIMEHeaderKey), `trim_wsp`,
  a line reader `LineR` and the header block reader of ReadMessage;
- std/multipart.xo: `canonical_key` (canonicalMIMEHeaderKey, None for a
  bad byte), `valid_field_byte`, `valid_value_byte` and its own header
  block reader (`Header`);
- std/smtp.xo: `dot_stuff` (Writer.DotWriter) and `parse_code_line`
  (ReadCodeLine);
- std/native/llrt's HTTP/1 parser: the server's own canonicalization (a
  hot path, measured, not part of this decision).

Programs that speak a line protocol (SMTP, POP3, NNTP, a Redis or
memcached text client, a mail filter) have none of it.

## Decision (proposed)

1. A new merged module `std/textproto` (one file, pure, every backend
   runs its Xo), ported from Go's net/textproto (go1.26.1 reader.go,
   header.go, writer.go, textproto.go) with Go's tests where they fit:

   ```xo
   pub fn canonical_key(s: Str) -> Str      // CanonicalMIMEHeaderKey
   pub fn trim(s: Str) -> Str               // TrimString
   pub struct Header { ... }                // MIMEHeader, in field order
   pub fn header() -> Header
   pub fn (h: Header) get(key: Str) -> Str?  // first value; the key canonicalized
   pub fn (h: Header) values(key: Str) -> List[Str]
   pub fn (h: Header) keys() -> List[Str]   // canonical, first seen order
   pub fn (h: Header) add(key: Str, value: Str) -> Header
   pub fn (h: Header) set(key: Str, value: Str) -> Header
   pub fn (h: Header) del(key: Str) -> Header
   pub fn (h: Header) to_map() -> Map[Str, Str]  // first values, for http's headers

   pub struct Reader { ... }                // over Bytes held in memory
   pub fn reader(b: Bytes) -> Reader
   pub fn (var r: Reader) read_line() -> Str ! TextErr             // Eof at the end
   pub fn (var r: Reader) read_continued_line() -> Str ! TextErr
   pub fn (var r: Reader) read_header() -> Header ! TextErr        // ReadMIMEHeader
   pub fn (var r: Reader) read_code_line(expect: Int) -> (Int, Str) ! TextErr
   pub fn (var r: Reader) read_response(expect: Int) -> (Int, Str) ! TextErr
   pub fn (var r: Reader) read_dot_bytes() -> Bytes ! TextErr
   pub fn (var r: Reader) read_dot_lines() -> List[Str] ! TextErr
   pub fn (r: Reader) rest() -> Bytes                               // unread bytes
   pub fn (r: Reader) header_truncated() -> Bool    // ended at the input's end (Go: header plus io.EOF)

   pub fn dot_encode(b: Bytes) -> Bytes     // Writer.DotWriter's output

   pub enum TextErr { Protocol(msg: Str), Code(code: Int, msg: Str), Eof, UnexpectedEof }
   ```

   Error texts are Go's (`malformed MIME header line: ...`,
   `short response: ...`, Error's `NNN msg`); text that is not UTF-8
   becomes U+FFFD (decision 0259 keeps `%XX` for URLs only); read_header
   has no size or count limit, as Go's exported ReadMIMEHeader has none
   (the limits are net/http's and mime/multipart's, through an internal
   entry point).
2. Stage 2: std/mail, std/multipart and std/smtp use it and drop their
   copies, each behind its own corpus (mail, multipart, smtp) on every
   backend, with an `--ab` row where the module has a bench (multipart).
3. Not planned: Conn, Dial, Pipeline, Cmd and the Writer's PrintfLine
   (a socket protocol client in Xo writes its lines and reads replies with
   Net and this Reader; std/smtp keeps its own client loop), and a
   streaming Reader over a source function (as multipart.Reader[fx]); the
   second is a later stage if a program needs to read a header block
   larger than memory.

## Consequences

- One canonicalization and one header block parser for mail, multipart
  and smtp, as Go has; a fix lands once.
- New public surface (std/textproto): spec/std.md gets a section; the
  audit's map.txt maps net/textproto's rows to it.
- The HTTP/1 server keeps its own parser (llrt, measured fast path).

## Alternatives

- Keep the copies private: the audit gap stays, and the three copies
  already differ (multipart's canonical_key returns None for a bad byte,
  mail's returns the key unchanged, as Go's exported function does).
- A streaming Reader first: no current caller needs it; the in-memory
  Reader covers the three modules, which hold the header block whole.

## Progress

- 2026-11-11, stage 1: std/textproto.xo with Go's reader, header and
  writer tests (std/textproto_test.xo) and the textproto corpus against
  Go on go, arm64 and llvm (spec/std.md 77, std/changes/textproto.md).
