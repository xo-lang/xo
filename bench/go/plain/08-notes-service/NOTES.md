# Notes

## Layout

- `domain`: Note type, validation, ID generation, filter, ordering, opaque cursor, pure `Page` function.
- `store`: one `<id>.json` file per note, atomic writes (CreateTemp, write, fsync, close, rename, best effort dir fsync). Depends on `domain` only.
- `api`: `net/http` handlers on a Go 1.22+ pattern `ServeMux`. Depends on `domain` and `store`.
- `server`: `package main`, flags `--data-dir` and `--addr` (Go `flag` accepts the double dash form), SIGINT/SIGTERM graceful shutdown via `http.Server.Shutdown`.
- `archtest`: the dependency rule test. It parses imports (including test files) of `domain`, `store`, `api` with `go/parser` and checks them against an allow list; a second test proves the checker flags `domain -> store`, `domain -> api`, `store -> api`.

## Ambiguities resolved

- Title length counts Unicode characters (runes), not bytes. Invalid UTF-8 is rejected. No trimming: a whitespace only title of length 1 to 200 is accepted.
- Duplicate tags in one note are rejected (validation error). Tags are not lowercased automatically; uppercase is a 400.
- Note ids are 32 lowercase hex characters from `crypto/rand`. This avoids reserved Windows names and characters, trailing dots/spaces, and case collisions on case insensitive file systems. Any path id not matching that shape is a 404 and the store refuses it, so no path traversal.
- ETag is a strong tag `"<version>"`. `If-Match` accepts the quoted or bare version. Missing or empty header is 428. Anything else that does not equal the current version (stale number, weak `W/` tag, `*`, a list, garbage) is 412. `*` is deliberately not honored because it would bypass lost update protection.
- Order of checks on PUT: invalid id 404, then If-Match (428/412 syntax), then body validation 400, then store (404 not found, 412 stale).
- PUT is a full replacement of title, body, tags. Unknown JSON fields (for example a client echoing `id` or `version`) are ignored; trailing data after the JSON object is a 400. Body is capped at 1 MiB.
- DELETE success returns 204 with no body.
- List response shape: `{"notes": [...], "next_cursor": "..."}`, `next_cursor` omitted on the last page. `limit` defaults to 20, must be 1 to 100, otherwise 400. A bad cursor is 400.
- "Newest first" means by creation time, ties broken by id descending, giving a total order. The cursor is a base64url encoding of (created unix nanos, id) of the last item returned, and the next page starts strictly after that key. Notes created between pages sort before the cursor, so they never cause duplicates or gaps; they show up on a fresh listing. With identical timestamps a new note may appear on a later page (only if its id sorts after the cursor), still without duplicates or gaps.
- The cursor does not bind the filter; changing `tag`/`q` mid pagination is allowed and just applies the new filter from the same position.
- Text search uses `strings.ToLower` on both sides (simple Unicode case folding is not attempted).
- `GET /export` uses `application/x-ndjson`, newest first, flushing after each line, from a snapshot taken at request start.
- Concurrency: the store holds one mutex across the version check and the disk write, so two updates with the same expected version serialize and the second sees a conflict. This serializes all writes, which is acceptable for this scale.
- The store keeps an in memory index loaded at `Open`; it assumes it is the only writer to the data directory. `Open` deletes stale `.tmp-*` files left by a crash. A corrupt note file makes `Open` fail rather than silently dropping data.
- "Never leaves temp files after a failed write" is tested by injecting a failing rename (unexported `rename` field, set in the internal store test) and checking the directory, the in memory state, and a reopened store.
- Time is injected into `api.New` as a `func() time.Time`; tests use a fake clock that advances a fixed step per call (or zero, to exercise timestamp ties).

## Bugs the loop caught

None. The first run of `go build ./... && go vet ./... && go test -count=1 ./...` passed with no compile, vet, or test problems, so the loop caught no bugs in this task. Any remaining defects would be ones the tests do not exercise.
