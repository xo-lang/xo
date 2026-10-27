# std/mail changes

## 2026-10-27: perf: Address.display quotes a name by its bytes

- Kind: perf
- Commit: this commit
- Found by: bench/perf/mail, the std Go twin (display 2.80x Go: a rune list per check and a string per rune)
- Go reference: net/mail Address.String, quoteString (go1.26.1)
- Test: internal/stdtest mail corpus; bench/perf/mail twincheck

quote_string writes ASCII bytes straight to a BytesBuilder and decodes a
rune only at a byte of 128 or more (invalid UTF-8 still becomes U+FFFD),
and the printable ASCII test of the name is a byte loop instead of
`runes().all`. Quiet --ab, 15 rounds, the parent's std as A: display wall
0.361 (instructions 0.497), about 1.01x Go; addr within noise.

## 2026-10-22: perf: the Date header uses a time.Layout

- Kind: perf
- Commit: da2bdc4c9
- Found by: Xo Lead's survey of constant layouts (std/changes/time.md)
- Go reference: net/mail Header Date (time.RFC1123Z)
- Test: std module tests native/mail; goldens mail_basic and mail_messages

`compose` formats the Date with a module level `time.layout(...)` constant
instead of hand written civil date code. mail.compose, 200,000 messages:
1,954 M to 1,911 M instructions (-2%); with the earlier formatter the
layout was 13% slower.

## 2026-10-21: behavior: the Go backend runs mail.xo too

- Kind: behavior
- Commit: c471ed8d
- Found by: decision 0312 (std written once)
- Go reference: net/mail (go1.26.1), as ported in std/native/mail.xo
- Test: internal/stdtest/testdata/corpus/mail (go, arm64, LLVM); goldens mail_basic and mail_messages (Go parity)

The Go backend compiles std/native/mail.xo instead of wrapping Go's
net/mail; only the display methods stay in std/rt/mail.go. Results are the
same. 200,000 parse_address and display calls on the Go backend: 115 ms
(Go's package 148 ms, the old wrapper 114 ms).

## 2026-10-21: fix: parse_date reads fractional seconds, an unclosed quoted local part is Go's error

- Kind: fix
- Commit: 4f542fe1
- Found by: differential test against Go (the new corpus)
- Go reference: net/mail ParseDate (time.Parse reads a fractional second after the seconds though the layout has none), addrParser.consumeAddrSpec (go1.26.1)
- Test: internal/stdtest/testdata/corpus/mail (scripts/stdcorpus mail: 2,500 addresses, 1,000 lists, 2,000 dates, and 600 messages, random and from net/mail's tests, with Go's results; TestStdCorpora on go, arm64, and LLVM)

Natively, a date with a fractional second (`Mon, 1 Dec 2006 15:04:05.123
+0000`) was `mail: header could not be parsed`; Go reads it (the instant
keeps the fraction), so parse_date now does. An address whose local part
is an unclosed quoted string (`<"unclosed`) gave `mail: unclosed
quoted-string`; Go gives `mail: empty quoted string in addr-spec`, and
now so does the native port.

## 2026-10-20: port: messages, dates, and a message builder

- Kind: port
- Commit: 100730e6
- Found by: survey (rank 7: services send mail; decision 0290)
- Go reference: net/mail message.go ReadMessage, readHeader, ParseDate; net/textproto ReadContinuedLine; mime/quotedprintable writer.go; mime QEncoding; mime/multipart Writer (go1.26.1)
- Test: std/native/mail_test.xo (Go's results on Go's test inputs, 3,000 random dates, 21 messages); internal/llvm/testdata/golden/mail_messages (every backend; compose byte for byte as Go's writers)

`read_message` (fields in order), `Message.header`, `parse_date` (a zone
name keeps the wall clock as UTC's, as Go's ParseInLocation in UTC), and
`compose` (text or text and HTML, quoted-printable, RFC 2047 subject).

## 2026-10-20: port: std/mail, Go's net/mail address parsing

- Kind: port
- Commit: b83e86b8
- Found by: survey
- Go reference: net/mail/message.go, mime/encodedword.go, net/netip/netip.go (go1.26.1)
- Test: std/native/mail_test.xo (Go's results on message_test.go's inputs and random address strings); internal/llvm/testdata/golden/mail_basic

New module: `parse_address`, `parse_address_list`, and `Address.display`,
with RFC 2047 encoded names, groups, comments, and domain literals, with
Go's results and error texts on every backend.
