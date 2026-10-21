# std/smtp changes

## 2026-10-20: port: std/smtp, Go's net/smtp SendMail

- Kind: port
- Commit: (filled in after the push)
- Found by: survey (rank 7: the job worker and the auth service send mail)
- Go reference: net/smtp smtp.go and auth.go, net/textproto ReadResponse and DotWriter (go1.26.1)
- Test: internal/gen/testdata/golden/test_smtp (a fake server on FakeNet; Go, arm64, LLVM)

New module (decision 0290): `smtp.send` with STARTTLS required by
default, implicit TLS, plain text only to loopback, PLAIN and CRAM-MD5,
and Go's checks and texts.
