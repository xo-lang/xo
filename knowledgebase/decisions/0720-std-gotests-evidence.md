# 0720: gotests, std evidence that replays Go's own test tables

Status: Accepted (owner, 2026-11-18, task #357; built by Xo Lang 3, 2026-11-19)

## Context

The lead's bug class report of 2026-11-18
(`~/xo-lead/notes/lead/bug-classes-full-2026-11-18.md`) counted 198 past
fixes where a std module's behaviour differed from the Go package it
ports: a different answer or a different error text. Fuzz and corpus
evidence (decision 0616) find these only for modules with a corpus, and
only on the generated inputs. Go's own `_test.go` files already hold
thousands of hand picked cases, most of them edge cases that came from
Go's bug reports.

## Decision

A new std evidence kind, `gotests`:

    go run ./scripts/stdproven gotests [-backends B,...] [-host LABEL] [-check] MODULE

`std/evidence/MODULE/gotests/` holds:

- `tables.txt`: one Go table a line, `PKG FILE VAR FIELDS KIND` (the Go
  package, its test file, the table variable or `FUNC.NAME` for a table
  local to a test function, the input fields by name or position, and the
  kind the adapters know it by);
- `go/main.go`: a Go adapter that calls the Go package;
- `main.xo`: an Xo adapter that calls the module.

The runner reads each table from the pinned Go release's source
(`go env GOROOT`) with go/ast. An element's inputs must be literals
(strings, a `+` of strings, integers, integral floats, time's `Local` and
`UTC`); any other element is counted as skipped. Each case goes to both
adapters in base64. Each adapter prints its result as text (a result's
fields, or `err ` and the error text). The two outputs must be equal,
line for line, on every backend named (arm64 and LLVM by default). Go's
answer is Go's own: the adapter calls Go, so the tables' expected
columns, which often reference package internals, are not needed.

A mismatch is a bug: fix it with a test, then run again. A projection is
allowed where the Xo API keeps less than Go (std/http's Url is HTTP only:
both adapters print "rejected" for a URL it has no form for, and only the
fields it keeps). Each projection is written in both adapters' comments.

`std/evidence/MODULE/gotests.md` is the evidence file in decision 0616's
format (kind gotests, the source hash, measure "N cases from T tables of
Go's tests (goX), K backends, 0 mismatches with Go, S elements skipped"),
with one line per table.

## Consequences

- First modules (2026-11-19): url (10 tables of net/url, 307 cases),
  http (5 tables of net/url through http.parse_url, 244), time
  (dateTests, 19). 0 mismatches on arm64 and LLVM.
- The owner's proof holds: with the url host parser's error texts as
  before b2ef88def, http's run has 6 mismatches on TestParseErrors
  (`invalid host "[::1]:namedport"` where Go says `invalid port
  ":namedport" after host`); with Earlier and Later swapped as before
  86e417086, time's run has 1 (dateTests[6], 02:30 in a spring forward
  gap: Go gives 01:30 PST).
- Cost: seconds per module; the adapters are built once per backend.
- Wiring the kind into scripts/stdcov (which marks need it) is xw-1's,
  with the lead deciding whether it becomes required for Proven.
- Only literal inputs are read; tables of Time values, of functions or of
  computed inputs need an adapter kind that builds them from literals, or
  stay uncovered.

## Alternatives

- Translating the tables by hand into Xo tests: thousands of lines to
  keep in sync with each Go release, and the expected columns are often
  Go internals.
- Running Go's test functions against an Xo shim of the Go API: needs a
  Go facade over every module, far more code than two adapters.
- Comparing against the tables' expected columns: fails where they use
  internal types, and misses errors whose text the tests only check with
  strings.Contains.
