# 0461: every std module has a maturity level on three axes

Status: Accepted (owner, 2026-11-02)

## Context

Decision 0276 makes Go's standard library std's reference scope, and the coverage table
(`knowledgebase/infrastructure/std-coverage.md`) says which Go packages are covered. It says nothing about how
far a covered module can be trusted: a module with a single smoke test and one with Go's whole test suite ported
both read "done". The website's std coverage page needs a level per module that a reader can check against
committed evidence. API stability is not a criterion: it is a question for 1.0.

## Decision

Each std module (and the prelude) gets three marks, one per axis, and an overall level that is the weakest of
the three. The levels are Early, Solid and Proven.

| Axis | Early | Solid | Proven |
|---|---|---|---|
| Features, against Go's package or the module's domain | core uses | common uses, gaps listed | near full parity, or beyond Go |
| Testing | own tests | Go's tests ported or a Go differential; parsers fuzzed | also `--race` and load stress clean, long fuzz runs, security reviewed where it applies (crypto, tls, http, x509) |
| Performance | not measured | measured against Go, within 2x | at or better than Go on its benchmark |

Reading rules used by the page (website `content/docs/std-coverage.md`, generated from committed files):

- Features: from the rows of the coverage table that name the module. All done is Proven, any partial is Solid;
  a module with no Go counterpart is Solid unless a reviewer sets it.
- Testing: a corpus under `internal/stdtest/testdata/corpus/MODULE` (Go's tests ported) or a Go differential test
  is Solid, but a module that parses untrusted input (http, url, mime, multipart, mail, smtp, tls, x509,
  websocket, json, xml, xmlcodec, toml, csv, archive, compress, dns, regexp, template, html, netip, time) is Early
  until a fuzz harness exists.
- Performance: a committed result file with a Go twin sets the mark; a measured result above 2x counts as Early
  (the criterion has no lower level); no committed row is Early.
- A mark with no committed evidence is Early. Each mark on the page links its evidence.

## Consequences

- The page shows, as of 2026-11-02, 3 modules Solid and 46 Early overall: the gaps are mostly missing fuzz
  harnesses and missing rows against Go, which are now an explicit list.
- Moving a mark means adding evidence (a fuzz test, a result file, a `--race` and load run), not editing the page.
- The generator for the page is a script over the committed files; the page is regenerated for each release.

## Alternatives

- API stability levels (preview, stable, mature): rejected by the owner, a 1.0 question.
- One overall level with no axes: hides which evidence is missing.
