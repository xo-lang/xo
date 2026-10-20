# std/uuid changes

## 2026-10-20: port: std/uuid, RFC 9562 version 4 and 7 UUIDs

- Kind: port
- Commit: (filled in after the push)
- Found by: survey
- Go reference: github.com/google/uuid v1.6.0 uuid.go, version4.go, version7.go (outside Go's std)
- Test: std/native/uuid_test.xo (google/uuid's Parse results on its test inputs and random UUIDs)

New module: `v4` and `v7` from the Rand and Clock capabilities, and
`parse` with google/uuid's accepted forms and error texts, on every
backend.
