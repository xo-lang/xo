# std/mail changes

## 2026-10-20: port: messages, dates, and a message builder

- Kind: port
- Commit: (filled in after the push)
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
