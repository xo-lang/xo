# std/mail changes

## 2026-11-11: feature: parse_date_civil (dates of any year)

- Kind: feature
- Commit: this push
- Found by: decision 0614 (the lead's report: dates outside 1677 to 2262 are `mail: date out of range`)
- Go reference: net/mail ParseDate (go1.26.1) for years 1 to 9999
- Test: golden time_civil_wide's codecs twin time_civil_codecs (go, arm64, llvm; unix seconds and offsets from net/mail)
- Timing: none (parse_date keeps its path; try_layout also returns the offset)
- Shipped in: not yet released

## 2026-11-11: internal: read_message reads lines with std/textproto

- Kind: internal
- Commit: this push
- Found by: decision 0598 stage 2
- Go reference: net/textproto (go1.26.1)
- Test: `xo test --std-module=mail` (native, LLVM); the mail corpus on go, arm64, llvm
- Timing: none (header and DATA handling, not a hot path)
- Shipped in: not yet released

read_message keeps net/mail's own header rules and reads its lines and continued lines with std/textproto's Reader; canonical keys are textproto.canonical_key. Its private line reader, canonical_key and trim are removed. Output unchanged.

## 2026-11-10: refactor: width bounds as the type constants (decision 0587 point 4)

- Kind: refactor
- Commit: this commit
- Found by: decision 0587 point 4 (std's literal bounds become the type constants)
- Go reference: none (no behavior change; Go's math.MaxInt32, MaxUint16, MaxUint32)
- Test: xo test --std-module=mail and its corpus where it has one

In std/mail, the IPv6 group check is against `Int(UInt16.max)`. No behavior change.

## 2026-11-10: refactor: one file with its bodies (decision 0588 stage 2)

- Kind: refactor
- Commit: this commit
- Found by: decision 0588 stage 2 (pure modules merged with their implementation)
- Go reference: none (no behavior change)
- Test: std/mail_test.xo (`xo test --std-module=mail` on native and llvm); internal/stdtest corpus mail on go, arm64 and llvm

std/mail.xo now holds the bodies of std/native/mail.xo (removed), and the Go
backend compiles them from it as its own package; std/rt/mail.go (the Go backend's display methods) is removed, the
module's own `display` methods serve it. The API, texts and errors
are unchanged: every item the declaration file had is `pub`, with the same
parameter names.

## 2026-11-02: fix: parse_date reads a zone named by a signed hour (`-07`) as Go does

- Kind: fix
- Commit: this commit
- Found by: the mail fuzz harness (internal/stdtest/mailfuzz_test.go, addresses, address lists, messages and dates against Go's net/mail)
- Go reference: time/format.go parseTimeZone and parseSignedOffset (go1.26.1)
- Test: internal/stdtest/testdata/corpus/mailfuzz (2,000 cases, the `+3`, `-23` and `-24` zones among them) on arm64, LLVM and the Go backend; TestMailFuzz seeds 2 to 6; the mail corpus and module tests
- Timing: none (one more branch for a zone that starts with a sign)
- Shipped in: v0.24.0 (100730e63, zone_name_len) through v0.36.0

`Mon, 02 Jan 2006 15:04:05 -07` was `mail: header could not be parsed`;
Go's time.Parse takes a sign and an hour of at most 23 as a zone name
with no offset it knows, so the date is read at offset zero, as `EST` or
any other name is on a machine whose local zone does not define it.

## 2026-10-28: perf: parse_date's layout lists are constants

- Kind: perf
- Commit: this commit
- Found by: xo vet --cost XO0914 (cmd/xo TestVetCostStd): four list literals inside parse_date's nested loops were made and freed on every iteration
- Go reference: net/mail parseDate (its layouts are a package-level slice built once)
- Test: internal/stdtest/testdata/corpus/mail (go, arm64, LLVM)

Not measured on its own (no mail date row); the same layouts in the same
order.

## 2026-10-27: perf: atoms and quoted strings are read without a rune at a time

- Kind: perf
- Commit: this commit
- Found by: bench/perf/mail list at 1.42x and addr at 1.38x Go; their CPU profiles (bytes_decode_rune 11 to 16%, is_atext 9%, a string per rune of a quoted string, contains("..") over each atom)
- Go reference: net/mail consumeAtom, consumeQuotedString, skipSpace (go1.26.1)
- Test: internal/stdtest/testdata/corpus/mail (go, arm64, LLVM); internal/llvm and internal/arm64 mail_basic and mail_messages; twincheck bench/perf/mail

An atom's ASCII bytes are checked against a 128 bit mask, with its
leading, trailing and doubled dots noted as they go by (it searched the
atom for them after); a quoted string copies the runs between escapes
whole, and is the text itself when there is none (it wrote a string per
rune), a byte that is not UTF-8 still becoming U+FFFD; skip_space slices
only past a space. Results are the same. Quiet --ab, 15 rounds, the
parent's std as A: list wall 0.468 (0.465 to 0.471), instructions
0.497; addr wall 0.527 (0.517 to 0.534), instructions 0.511.

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
