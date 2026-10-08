# Notes

## Layout

- `domain/`: `Note`, `ValidateContent`, `ValidID`, `NewID`, and pure search
  (`Filter`, `Search`, `Cursor`). Imports only the standard library.
- `store/`: one `<id>.json` file per note, atomic temp file plus fsync plus
  rename, stale temp files removed on `Open`. Imports `domain` only.
- `api/`: HTTP handlers on `http.ServeMux` method patterns. Imports `domain`
  and `store`.
- `server/`: the executable (`--data-dir`, `--addr`, SIGINT/SIGTERM graceful
  shutdown with a 15 s drain).
- `archtest/`: the dependency rule test. It parses every `.go` file (tests
  included) in each package directory and fails on any `benchtask/...` import
  not on that package's allow list. A second test writes synthetic violating
  files (domain importing store or api, store importing api) into
  `t.TempDir()` and asserts the checker flags them, so the rule test is proven
  able to fail.

## Ambiguities resolved

- Title length counts Unicode code points (runes), not bytes; invalid UTF-8 is
  rejected. Whitespace only titles are accepted because the spec only says
  1 to 200 characters.
- Duplicate tags are rejected (the spec is silent); tags omitted or `null`
  become `[]`.
- Validation failures return 400 (not 422). Bodies over 1 MiB return 413.
  Trailing data after the JSON object returns 400. Unknown JSON fields are
  ignored so clients can send back a whole note.
- `PUT` is a full replacement of `title`, `body`, `tags`; `id`, `created`,
  `version` are controlled by the server.
- `If-Match`: missing or empty is 428; it must be a single strong tag `"N"`
  (a bare `N` is also tolerated). `*`, weak tags, and lists are 400 because
  the spec requires the current version explicitly. A stale version is 412.
  Header check comes first, so a missing header is 428 even for an unknown id;
  an unknown id with a header is 404.
- `DELETE` success is 204.
- Listing order is `created` descending with id descending as a tie breaker
  (total order). The cursor is base64url of `<unix nanos>.<id>` of the last
  note returned; the next page starts strictly after that key, so notes
  created later (which sort before the key) never shift or duplicate later
  pages. A backdated note that sorts after the key does appear later, which
  the domain test checks too.
- `limit` defaults to 20, valid range 1 to 100, otherwise 400; bad cursor 400.
  The list response is `{"notes": [...], "next_cursor": "..."}` with
  `next_cursor` omitted on the last page.
- `GET /export` returns `application/x-ndjson`, reading and flushing one file
  at a time. Errors after headers are sent can only be logged.
- Ids are 26 lowercase base32 characters from `crypto/rand.Text`. `ValidID`
  allows only `[a-z0-9]{1,64}` and rejects Windows device names (con, prn,
  aux, nul, com0-9, lpt0-9), so ids are safe file names everywhere, cannot
  traverse paths, and cannot collide on case insensitive filesystems.
- Concurrency control is a single in process mutex around read, version
  compare, and write. That serializes all mutations (correct and simple, not
  the most concurrent). Multiple processes on one data directory are out of
  scope.
- No fsync of the directory after rename (not portable to Windows); the file
  itself is fsynced before rename.

## Tool findings

- No tool reported anything; run 1 passed every check, so no bugs were caught
  by vet, staticcheck, errcheck, nilaway, exhaustive, or the race detector.
- No false positives to report.
- No `//nolint` comments and no `_ =` error discards anywhere.
