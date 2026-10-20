# std/mail changes

## 2026-10-20: port: std/mail, Go's net/mail address parsing

- Kind: port
- Commit: (filled in after the push)
- Found by: survey
- Go reference: net/mail/message.go, mime/encodedword.go, net/netip/netip.go (go1.26.1)
- Test: std/native/mail_test.xo (Go's results on message_test.go's inputs and random address strings); internal/llvm/testdata/golden/mail_basic

New module: `parse_address`, `parse_address_list`, and `Address.display`,
with RFC 2047 encoded names, groups, comments, and domain literals, with
Go's results and error texts on every backend.
