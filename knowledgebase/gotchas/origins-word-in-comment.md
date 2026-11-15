# A project's name in a new std comment fails TestThirdPartyOrigins

Status: Observed (2026-11-15, twice: std/sql batch S1 and S2)

## Symptom

`go test ./internal/repotest -run TestThirdPartyOrigins` fails with
`std/sql.xo:217: names sqlite with no Origin: sqlite marker in the file`
after a doc comment that only mentions the project ("BEGIN and COMMIT on
MySQL and SQLite").

## Cause

The test (decision 0302) flags every std file line that names a third
party project the repository copies code from, unless the file carries
that project's `Origin:` marker or the line is in
internal/repotest/testdata/origins-allow.txt. It cannot tell a mention
from a copy.

## Fix

Say what the code does instead of naming the project ("a driver that runs
the queries one by one"), or, for a reviewed mention, add the line to
origins-allow.txt. Run the test before a push that adds comments to std
declaration files.
