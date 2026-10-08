# Go names change on the way into Xo

Status: Observed (2026-10-08, Go interop prototype)

Symptom: a program written against the Go documentation does not resolve:
`regexp.match(...)` is "member not found", `url.Error` and `hash.Hash` do
not exist, and `uuid.nil_()` (guessed by analogy with `match_`) is wrong.

Cause: `xo bind` (internal/gobind) renames to fit Xo:

- functions, methods, fields, and parameters become snake_case
  (`FindAllStringSubmatch` -> `find_all_string_submatch`,
  `ParseRequestURI` -> `parse_request_uri`, `Sum256` -> `sum256`);
  constants become UPPER_SNAKE (`BlockSize` -> `BLOCK_SIZE`);
- a name that is an Xo keyword (or `go`, `self`, `static`, `true`,
  `false`) gets a `_` suffix: `regexp.Match` is `regexp.match_`. Words
  that are not keywords keep their name: `strings.Map` is `strings.map`,
  `uuid.Nil` is `uuid.nil()`;
- a handle type whose name is a prelude type gets the Go package name in
  front: `url.Error` is `url.UrlError`, `hash.Hash` (returned by
  `sha256.New`) is `sha256.HashHash`;
- fields are getter and setter methods (`u.host()`, `u.set_host(v)`),
  package variables are zero argument functions (`uuid.name_space_dns()`);
- `rune` and `int32` are the same go/types kind; the binder tells them
  apart by `types.Basic.Name()` so `rune` maps to Rune.

Fix: read the generated declaration file (`.xo/gobind/<path>.xo`); every
declaration carries the Go signature as a `/// Go:` doc line, and skipped
names are listed at the top with the reason (the checker reports them as
XO1102, not XO0601).
