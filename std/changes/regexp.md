# std/regexp changes

## 2026-10-22: perf: a first byte filter and one Int per backtrack job

- Kind: perf
- Commit: this commit
- Found by: bench/results/regexp-time-csv-vs-go-2026-10-21.md, re-measured in bench/results/regexp-first-byte-2026-10-22.md
- Go reference: regexp/backtrack.go (bitState jobs); the first byte filter is Xo's own (Go's prefix scan only covers literal prefixes)
- Test: internal/llvm/testdata/golden/regexp_table (extended: folds to ASCII, empty matches, assertions), internal/stdtest corpus regexp

The compiled program records the ASCII bytes a match can start with (when no
match is empty); both matchers skip start positions at other ASCII bytes
(non-ASCII bytes are always tried). The backtracker's three job lists are one
list of packed Ints. `xo bench --ab`: match wall 0.757x, find_all 0.911x,
captures 0.636x; replace is flat (+0.9% instructions, wall within noise).

## 2026-10-21: perf: patterns anchored at the start try one position

- Kind: perf
- Commit: (filled in after the push)
- Found by: bench/results/regexp-time-csv-vs-go-2026-10-21.md
- Go reference: regexp/backtrack.go startCond (go1.26.1)
- Test: internal/stdtest/testdata/corpus/regexp (go, arm64, LLVM)

A pattern starting with `^` (without the multiline flag) is tried at position 0
only. Anchored short-text matching is 1.35 times fewer instructions.

## 2026-10-21: perf: literal-prefix scan and the bit-state backtracker

- Kind: perf
- Commit: (filled in after the push)
- Found by: bench/results/regexp-time-csv-vs-go-2026-10-21.md
- Go reference: regexp/backtrack.go, Prog.Prefix (go1.26.1)
- Test: internal/stdtest/testdata/corpus/regexp (go, arm64, LLVM)

Texts shorter than 262,144 divided by the program's length (programs up to 500
instructions) are searched by backtracking with a visited bit per state, as
Go does; a literal prefix of the pattern is searched for when no thread is
alive. `is_match` on short strings is 2.1 times faster than before the
afternoon's work in total (29,759 M to 8,186 M instructions; Go 3,943 M).
Results are the same on the corpus.

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
