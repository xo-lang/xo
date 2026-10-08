# Notes: 01-crud-api (strict Go)

## Layout
- `store.go`: mutex guarded in-memory store (users map plus email index), sentinel errors `ErrNotFound`, `ErrDuplicateEmail`.
- `handler.go`: Go 1.22+ `ServeMux` method/path patterns, JSON decoding and validation.
- `main.go`: reads `PORT` (default 8080), runs `http.Server` with timeouts.
- `handler_test.go`: in-process tests via `httptest.NewRecorder` (no network at all).

## Ambiguities resolved
- Name length is counted in Unicode characters (runes), not bytes. 100 multi byte characters is valid.
- Email validation is only "contains `@`", exactly as specified.
- Email uniqueness is case-insensitive (`A@x.com` conflicts with `a@x.com`); the stored email keeps the case the client sent.
- Patching a user to its own current email is not a conflict.
- `nickname` is omitted from JSON when unset. In PATCH, `"nickname": null` clears it. `null` for `name` or `email` is a 400.
- Unknown fields, malformed JSON, wrong types, trailing data after the JSON value, and non-object PATCH bodies are 400. An empty PATCH object `{}` is a no-op returning 200 (or 404 if the user is missing).
- A non-integer or non-positive `{id}` returns 404 (no such resource) rather than 400.
- PATCH order of checks: body validation (400) first, then existence (404), then email conflict (409).
- Ids are never reused after delete; a deleted user's email becomes available again.
- Bodies are capped at 1 MiB via `http.MaxBytesReader`.
- Wrong methods get the mux's built-in 405.
- `GET /users` on an empty store returns `[]`, not `null`.

## Bugs caught by tools
None. The first strict-check run passed every check (vet, staticcheck, errcheck, nilaway, exhaustive, go test -race), so no tool caught a bug in this solution.

## False positives / suppressions
- No findings, so no false positives judged.
- No `//nolint` style comments and no `_ =` error discards. The only ignored-looking error path is the JSON encode error in `writeJSON`, which is logged rather than discarded (headers are already sent, so nothing else can be done).
- The decoder trailing data check treats any result other than `io.EOF` from a second `Decode` as an error; this is intentional, not a suppressed error.
