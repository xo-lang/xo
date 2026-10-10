# The tree dump quotes diagnostics with strconv.Quote, not the token quoting

Status: Observed (2026-10-10)

Symptom: after the habits port, one of 20000 token mutations differed
between the parsers only in a diagnostic line: the Go dump had
`"`if i == 1 {\n ...}++` is Go..."`, the Xo dump `{\x0a ...`.

Cause: parser.Dump prints a message with `strconv.Quote`; selfhost/parser
used the token dump's quoting (`lexer.quote`, printable ASCII kept, every
other byte `\xHH`), which agrees for messages of plain ASCII without
newlines. Before the habits port only the first error was printed and
never compared; a habit message embeds source text, newlines included.

Fix: the Xo dump quotes messages with `Bytes.quote()`, which is
strconv.Quote on every backend (spec/std.md 2). Use the token quoting
only where internal/ast Text uses textQuote (attributes of nodes).
