# std/smtp changes

## 2026-10-21: behavior: the Go backend runs smtp.xo too

- Kind: behavior
- Commit: 8c5bf765
- Found by: decision 0312 (std written once)
- Go reference: net/smtp (go1.26.1), as ported in std/native/smtp.xo
- Test: internal/stdtest/testdata/corpus/smtp (go, arm64, LLVM); golden test_smtp

The Go backend compiles std/native/smtp.xo instead of its own Go port
(std/rt/smtp.go, deleted), so every backend runs one implementation; the
steps and texts are the same. The corpus (in-process servers) takes 0.16 s of
CPU on the Go backend instead of 0.12 s.

## 2026-10-21: fix: replies, HELO, AUTH, and an empty message as Go's net/smtp

- Kind: fix
- Commit: 52e13d37
- Found by: differential test against Go (scripts/stdcorpus smtp: net/smtp's SendMail and std/smtp against the same scripted server, 500 sessions)
- Go reference: net/smtp smtp.go (SendMail, hello, ehlo, Auth), auth.go; net/textproto reader.go ReadResponse, writer.go dotWriter (go1.26.1)
- Test: internal/stdtest/testdata/corpus/smtp

Both backends' ports of SendMail differed from Go in five ways:

- In a multi-line reply, a line with another code is now kept and reading
  goes on, as textproto does; it used to fail with `status code
  mismatch`.
- After the HELO fallback there are no extensions, so AUTH is skipped and
  MAIL FROM has no BODY=8BITMIME, where `smtp: server doesn't support
  AUTH` was reported.
- Extension names keep the server's case.
- AUTH follows Go's loop:
  - a 235 ends it, CRAM-MD5 included;
  - another reply, or a challenge that is not base64, sends `*` and QUIT
    before the error.
- An empty message is sent as an empty line before the dot.

## 2026-10-20: port: std/smtp, Go's net/smtp SendMail

- Kind: port
- Commit: 4855efd0
- Found by: survey (rank 7: the job worker and the auth service send mail)
- Go reference: net/smtp smtp.go and auth.go, net/textproto ReadResponse and DotWriter (go1.26.1)
- Test: internal/gen/testdata/golden/test_smtp (a fake server on FakeNet; Go, arm64, LLVM)

New module (decision 0290): `smtp.send` with STARTTLS required by
default, implicit TLS, plain text only to loopback, PLAIN and CRAM-MD5,
and Go's checks and texts.
