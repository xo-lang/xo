# std/regexp changes

## 2026-10-21: test: Go's RE2 search tests

- Kind: test
- Commit: (filled in after the push)
- Found by: differential test against Go
- Go reference: regexp/testdata/re2-search.txt, regexp Compile, FindStringSubmatchIndex (go1.26.1)
- Test: std/native/regexp_test.xo (1,486 pattern and text pairs: compile errors, the match, every capture)

No difference found: the native engine compiles, rejects, and matches as
Go's regexp on every pair, except patterns with a Unicode class native
builds do not have yet (`\p{Greek}`, decision 0172), which the test
counts apart.
