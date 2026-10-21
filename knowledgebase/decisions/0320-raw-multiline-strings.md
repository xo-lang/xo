# 0320: Raw multi-line strings `r"""..."""`

Status: Accepted (owner, from the 2026-10-21 poll, relayed by the lead;
implemented by Xo Publish: compiler/lexer raw_triple, spec core 1.6).

## Context

Xo has three string forms that matter here (core 1.6): `"..."` and
`"""..."""` decode escapes and interpolate `${expr}`; `r"..."` and
`r#"..."#` do neither but must fit their text between one pair of quotes,
and keep every byte, indentation included. Text that is long, spans
lines, and is full of `$`, `{` and `\` (SQL, shell, regular expressions,
JSON or HTML fixtures, embedded Xo source in tests) had no good form:
`"""` needs `\$` and `\\` throughout, and a multi-line `r"..."` carries
the code's indentation into the value. Agents writing tests hit this:
explain_data and test fixtures escape `${` by hand, and a missed escape
interpolates silently or fails to check.

Before this decision `r"""` lexed as the raw string `r""` followed by an
unterminated `"`, so the form was free.

## Decision

`r"""..."""` is a raw multi-line string:

- no escapes and no interpolation: every byte between the quotes is kept
  (`\n` is two bytes, `${x}` is four);
- its common leading indentation is removed exactly as for `"""`
  (lexer strip_indent): a first line holding only the opening quotes and
  a last line holding only the closing quotes (and spaces or tabs) are
  dropped, and the smallest indentation of the other non-blank lines is
  removed from each; text right after the opening quotes is kept as is;
- CR LF reads as LF, as in every string;
- `r#"""..."""#`, with any number of `#`, is closed only by `"""` and
  as many `#`, so it may contain `"""`;
- the token is a RawString (as `r"..."`) with the triple flag set, so the
  parser, checker and every backend treat it as any raw string; `xo fmt`
  keeps the source text.

## Consequences

- Spec core 1.6 and Changes in 0.10 describe it; golden raw_triple runs it
  on every backend; compiler/lexer tests cover indentation, `#`s, CR LF
  and one-line use; the VS Code grammar has a string-raw-triple rule.
- An unterminated `r"""` is XO0151 ("raw string literal not terminated;
  expected `"""`"), the existing code for unclosed strings.
- No program changes meaning: `r"""` did not lex before.

## Alternatives

- Keep the indentation (Rust's `r#"..."#` does): a multi-line raw string
  inside an indented function would then need its text at column 0, the
  problem `"""` already solved; the same stripping as `"""` keeps one rule
  for both multi-line forms.
- A different prefix (`'''`, backticks as in Go): a new quote character
  for one feature; `r` already means raw and `"""` already means
  multi-line, so their combination needs no new rule to learn.
- An escape-free `"""` (Python's raw triple strings are `r"""` too): would
  change existing programs.
