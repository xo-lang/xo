# 0290: std/smtp and mail messages

Status: Accepted (owner, 2026-10-20)

## Context

Survey rank 7 (the job worker and the auth service
send mail: password resets, receipts, alerts). Now possible: std/tls
(decision 0281) gives STARTTLS on a dialed connection, std/mail
(decision 0282) parses addresses, std/mime has the media types. Go
reference: net/smtp (client, PlainAuth, CRAMMD5Auth, SendMail),
net/mail's ReadMessage and Header, mime's WordEncoder for headers, and
mime/quotedprintable's Writer for bodies (decision 0276: Go's scope,
code, and tests, Xo's API).

## Decision

STARTTLS is required by default (`Security.StartTls`); plain text is
allowed only to a loopback host (owner, 2026-10-20). The tests use
FakeNet's raw connections (decision 0292), so they are `test` blocks
rather than programs on real sockets.

### std/smtp (net/smtp)

```
struct SmtpErr { code: Int, msg: Str }       // the server's reply (550 ...), or code 0 for a local error
enum Auth { None, Plain(user: Str, password: Secret[Str]), CramMd5(user: Str, secret: Secret[Str]) }
enum Security { StartTls, Tls, None }        // StartTls: STARTTLS required, else an error (Go's SendMail uses it when offered)

pub fn send(net: Net, addr: Str, from: Str, to: List[Str], msg: Bytes,
            auth: Auth = Auth.None, security: Security = Security.StartTls,
            roots: Bytes = b"") -> () ! SmtpErr uses net.dial
```

- One call, as Go's SendMail: dial, read the greeting, EHLO (HELO when
  refused), STARTTLS through std/tls with the host's name verified
  (`Security.Tls` is implicit TLS on port 465; `None` only to a loopback
  host, as Go's PlainAuth refuses plain text elsewhere), AUTH, MAIL FROM,
  RCPT TO for each, DATA with dot stuffing and CRLF line endings, QUIT.
- Go's checks and texts: a line with CR or LF in an address is refused
  (`smtp: A line must not contain CR or LF`), PLAIN over an unencrypted
  connection to a non local host is refused (`unencrypted connection`).
- `uses net.dial` (decision 0280): a mailer function needs only dial.

### Messages (std/mail, completing decision 0282's address parsing)

```
struct Message { headers: List[(Str, Str)], body: Bytes }
pub fn read_message(b: Bytes) -> Message ! MailErr          // net/mail.ReadMessage: headers then the body
fn (m: Message) header(name: Str) -> Str?                   // first value, case insensitive
pub fn parse_date(s: Str) -> Time ! MailErr                 // net/mail.ParseDate (RFC 5322 dates)

/// A message ready for smtp.send: From, To, Subject (RFC 2047 encoded
/// when not ASCII), Date, Message-ID, MIME-Version, and a text/plain or
/// multipart/alternative (text and HTML) body in quoted-printable.
pub fn compose(from: Address, to: List[Address], subject: Str, text: Str,
               html: Str? = None, date: Time, id: Str) -> Bytes
```

- `compose` is new API (Go has no message builder); its parts are Go's:
  mime.QEncoding for headers, quotedprintable.Writer for the body,
  multipart.Writer for alternatives, and Address.String for the address
  headers.

### Tests

Go's net/smtp tests (smtp_test.go's fake server conversations: basic,
STARTTLS, AUTH, extensions, the error replies) as golden programs over
local sockets, a differential test against Go's SendMail talking to the
same fake server (the bytes each client sends), and a Docker run against
a real server (axllent/mailpit) with STARTTLS; net/mail's message_test
tables for read_message and parse_date; quotedprintable's writer tests.

### Test map (Go test, what it becomes)

| Go test | Xo test |
|---|---|
| net/smtp TestAuth, TestAuthPlain, TestClientAuthTrimSpace | std/smtp unit tests: PLAIN and CRAM-MD5 responses, the unencrypted refusal, the localhost exception |
| TestBasic, TestHELOFailed, TestExtensions, TestHello | goldens: a scripted fake server over a local socket; the client's lines compared to Go's recorded transcript |
| TestSendMail, TestSendMailWithAuth, TestAuthFailed | goldens on the fake server: dot stuffing, CR/LF refusal, 535 as `SmtpErr { code: 535 }` |
| TestTLSClient, TestTLSConnState | a golden with std/tls's test server: STARTTLS and implicit TLS, a wrong host failing verification |
| net/mail TestParsing, TestDateParsing, TestDateParsingCFWS | std/mail tests: read_message and parse_date tables |
| mime TestEncodeWord, TestEncodedWordLength | std/mime tests: the Q encoder used by compose |
| quotedprintable TestWriter, TestWriterBinary, TestRoundTrip | std/mail tests of the body encoder (the decoder exists since 0282) |
| (new) compose | a golden and a differential test: Go builds the same message with mime.QEncoding, quotedprintable.Writer, multipart.Writer and a fixed boundary; bytes compared |

Plus one Docker run against mailpit (STARTTLS, PLAIN): the message mailpit
stores equals compose's bytes.

## Consequences

- Services send mail with one call over std/tls, with Go's checks
  (CR/LF in addresses, PLAIN only over TLS or to localhost).
- std/mail grows from address parsing (decision 0282) to whole messages:
  reading (read_message, parse_date) and writing (compose).
- Each module's change log (decision 0276) records the port:
  std/changes/smtp.md and std/changes/mail.md.

## Alternatives

- Expose Go's Client step by step (Hello, StartTLS, Auth, Mail, Rcpt,
  Data): more surface than services use; `send` covers them, and a step
  API can come later.
- Leave mail to HTTP APIs (SendGrid, SES): common, but SMTP is what
  self-hosted services and local development (mailpit) speak.
