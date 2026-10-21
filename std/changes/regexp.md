# std/regexp changes

## 2026-10-21: test: a Go-generated corpus for regexp

- Kind: test
- Commit: (filled in after the push)
- Found by: the lead's plan for the regexp fast paths (Go's order, each proved by a corpus)
- Go reference: regexp (go1.26.1)
- Test: internal/stdtest/testdata/corpus/regexp (go, arm64, LLVM); scripts/stdcorpus regexp

260 patterns (curated: anchors, classes, repeats, lazy forms, alternation,
groups, flags, folding, empty matches, bad syntax; and seeded random ones)
against 40 texts: whether the pattern compiles, find_index, find_all,
captures, replace_all, replace_all_literal, and split, equal to Go's. It
passes on the current engine on all three backends.

## 2026-10-21: perf: the Pike VM queues are flat and preallocated

- Kind: perf
- Commit: (filled in after the push)
- Found by: XO_PROF=cpu (bench/results/regexp-time-csv-vs-go-2026-10-21.md)
- Go reference: regexp/exec.go queue (sparse set with a dense entry array)
- Test: std module tests native/regexp on arm64 and LLVM

Per match the queues and thread captures are allocated once, not per input
position. match 1.7 times faster, find_all and replace_all 1.9, captures 1.7
(instructions); results are the same.

## 2026-10-21: port: every Unicode class natively

- Kind: port
- Commit: 9212b69b
- Found by: the std coverage table (decision 0276)
- Go reference: regexp/syntax unicodeTable, unicode.Categories, Scripts, FoldCategory, FoldScript, CategoryAliases (go1.26.1, Unicode 15.0.0)
- Test: std/native/regexp_test.xo (1,092 cases: `\p{NAME}`, `\P{NAME}`, and `(?i)[\p{NAME}]` for every category, script, and alias, on the runes at the edges of the class's ranges, against Go's ReplaceAllString), TestUnicodeTables

Native builds had only `\pL`, `\p{Nd}`, `\p{Any}`, and `\p{ASCII}`; any
other class Go knows (`\p{Greek}`, `\pN`, `\p{Lu}`) was an error. They now
have all of them, generated from Go's tables (std/native/llrt/unicode_classes.xo,
about 64 KB more in a program that uses regexp). As in Go, a script
name with an underscore (`\p{Old_Italic}`) is an invalid class: Go's
lookup canonicalizes it to `Olditalic`, which no table has.

## 2026-10-21: test: Go's RE2 search tests

- Kind: test
- Commit: b2ef88de
- Found by: differential test against Go
- Go reference: regexp/testdata/re2-search.txt, regexp Compile, FindStringSubmatchIndex (go1.26.1)
- Test: std/native/regexp_test.xo (1,486 pattern and text pairs: compile errors, the match, every capture)

No difference found: the native engine compiles, rejects, and matches as
Go's regexp on every pair, except patterns with a Unicode class native
builds did not have yet (`\p{Greek}`, decision 0172), which the test
counted apart until the entry above.
