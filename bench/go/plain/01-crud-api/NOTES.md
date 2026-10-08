# Notes

## Ambiguities resolved

- Name length is counted in Unicode code points (runes), not bytes. Names are not trimmed.
- Email validation is only "contains @", as specified. No further format checks.
- Email uniqueness is an exact, case-sensitive string match. Patching a user to its own current email is not a conflict. Deleting a user frees its email.
- `nickname` is a string; omitted from the JSON response when unset. On create, `null` or absent means no nickname. On PATCH, `"nickname": null` clears it, while omitting it leaves it unchanged.
- `name` or `email` sent as `null` or a non-string is a 400. Unknown body fields are a 400. A body that is not a JSON object is a 400.
- Validation runs before the duplicate email check, so an invalid body returns 400 even if it would also conflict.
- PATCH with an empty object `{}` returns 200 and the unchanged user.
- A non-integer or non-positive id in the path returns 404 (the resource cannot exist), not 400.
- PATCH on a missing id returns 404 even when the body is also invalid only if the id fails to parse; for a parseable id the body is validated first (400), then existence is checked (404).
- Error bodies are `{"error": "..."}` for 400, 404, and 409. DELETE success returns 204 with no body.
- `GET /users` with no users returns `[]`, not `null`.
- Ids are never reused after delete.
- Concurrency: a single `sync.RWMutex` guards the store; email index is updated under the same lock so duplicate checks are atomic.

## Bugs caught by the loop

None. The first run of `go build ./... && go vet ./... && go test -count=1 ./...` passed, so the loop caught no bugs.
