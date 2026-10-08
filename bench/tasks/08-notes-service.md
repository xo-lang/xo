# Task 08: Notes service in several modules

Build a small notes service split into separate modules (packages) with clear
dependency rules.

## Structure

- `domain`: the `Note` type (`id`, `title`, `body`, `tags`, `version`,
  `created`, `updated`), validation (title 1 to 200 characters, at most 10 tags,
  each tag lowercase `[a-z0-9-]{1,32}`), and pure search logic. Depends on
  nothing else in the project.
- `store`: persistence on the filesystem, one JSON file per note in a data
  directory. Writes are atomic (temp file plus rename). Depends on `domain` only.
- `api`: HTTP handlers. Depends on `domain` and `store`.
- `server` (the executable): wiring, flags (`--data-dir`, `--addr`), graceful shutdown.
- A test must fail if `domain` imports `store` or `api`, or `store` imports `api`.

## Behavior

- `POST /notes` creates (201, returns the note with `version: 1` and an `ETag`).
- `GET /notes/{id}` returns the note with `ETag: "<version>"`.
- `PUT /notes/{id}` requires `If-Match` with the current version; a missing
  header is 428, a stale version is 412. Success increments `version`.
- `DELETE /notes/{id}` also requires `If-Match`.
- `GET /notes?tag=x&q=text&limit=20&cursor=...` lists notes newest first,
  filtered by tag and by case insensitive text in title or body, with opaque
  cursor pagination that stays correct when notes are created between pages.
- `GET /export` streams all notes as JSON lines.
- Concurrent updates to the same note must never lose an update: one wins,
  the other gets 412.
- Note ids must be safe as file names on Linux, macOS, and Windows.

## Tests to write

- Validation rules.
- Optimistic concurrency: two concurrent PUTs with the same version, exactly one succeeds.
- Pagination: create 25 notes, page through with limit 10, no duplicates or gaps, including when a note is created mid way.
- Store survives a restart (new store on the same directory sees the notes) and never leaves temp files after a failed write.
- The dependency rule test.
