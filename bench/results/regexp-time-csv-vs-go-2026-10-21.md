# std/regexp, std/time, and std/csv against Go's packages (2026-10-21)

Native Xo (`--backend=arm64`) and the Go backend against Go twins
(go1.26.1 regexp, time, encoding/csv), on the development Mac (Apple M4 Pro,
24 GB, macOS 27.0), xo from main (the tree of this commit's parent).

## Method

As bench/results/json-vs-go-2026-10-21.md: instructions retired
(`/usr/bin/time -l`), median of 5 runs minus the median of 5 `none` runs of
the same binary, interleaved; the programs take a mode argument. The machine
was quiet (80% memory free).

- **regexp** (text of 1,500 sentences, 150 KB; patterns `[a-z]+[0-9]*@[a-z]+\.com`,
  `(\d{4})-(\d{2})-(\d{2})`, `(\w+)=(\w+)`): compile 20,000 x 3; match 300,000 x 2
  on short strings; find_all of the three patterns over the text x 20; replace_all
  of two patterns over the text x 20; captures on 1,500 clauses x 3.
- **time**: format 300,000 x 2 (RFC 3339 in UTC; an HTTP date in
  America/New_York); parse 300,000 RFC 3339 texts; civil fields of 300,000
  instants in America/New_York; Duration add, multiply, millis 10,000,000 times.
- **csv**: 20,000 rows of six fields (some quoted, some empty), parsed 10
  times and formatted 10 times.

## Results (instructions, millions; ratio to Go)

| workload | Go | native | native / Go | Go backend | Go backend / Go |
|---|---|---|---|---|---|
| regexp compile | 1,403 | 1,705 | 1.22 | 1,329 | 0.95 |
| regexp match | 3,943 | 29,759 | **7.55** | 3,642 | 0.92 |
| regexp find_all | 6,615 | 18,697 | **2.83** | 6,615 | 1.00 |
| regexp replace_all | 5,000 | 16,033 | **3.21** | 4,921 | 0.98 |
| regexp captures | 434 | 1,919 | **4.43** | 433 | 1.00 |
| time format | 953 | 9,586 | **10.06** | 5,651 | 5.93 |
| time parse | 462 | 4,796 | **10.37** | 925 | 2.00 |
| time civil | 637 | 263 | 0.41 | 3,048 | 4.79 |
| Duration math | 351 | 190 | 0.54 | 1,248 | 3.56 |
| csv parse | 787 | 1,315 | 1.67 | 731 | 0.93 |
| csv format | 740 | 1,386 | 1.87 | 2,246 | 3.03 |

(The Go backend calls Go's regexp and csv parse, so those rows are 1.0 by
construction; its time and csv format are std/rt code.)

## Where the time goes (native, XO_PROF=cpu)

- **regexp match**: the Pike VM allocates per input position. `add`
  (the thread list builder) is 59% cumulative, `alloc.alloc` 14%,
  `obj.rl_copy` 26%: each start position copies the capture list
  (`var cap = matchcap`), each queue entry pushes an empty capture list, and
  each step copies it again. Go's regexp avoids this with a one-pass matcher,
  a bit-state backtracker for small programs and inputs, and a literal prefix
  scan; none are ported. The cost scales with start positions tried.
- **time format and parse**: `str_eq` is 35% flat. Each call parses the layout
  again (`layout_items` 35% cum, 58% with the validation it did) and then
  matches each item's field name against up to 14 strings. Go parses a layout
  per call too but with a byte switch.
- **csv**: allocation of the per field strings and row lists (not profiled
  further).

## Fixes in this commit

- time `layout_items` validated a field by building its argument list and
  searching it; it now checks the field without a list (`field_ok`): format
  9,612 to 8,725 M, parse 4,824 to 4,382 M (9%). The remaining cost is the
  string comparison of field names per item per call.

## Next (not done: each is a larger change)

1. time: give each layout item an integer field code when the layout is parsed,
   and match on it in `format` and `parse` (expected to remove the str_eq
   share, about a third).
2. regexp: a flat capture store for the Pike queues instead of a list per
   entry, then Go's literal-prefix scan and a backtracker for small programs
   (the 7.5x of match is mostly this).
3. csv: write fields into one builder instead of a string per field.

## Step 1, time (2026-10-21): integer field codes

Each layout item carries an integer code (`field_code`: the name's length and
first byte pick a candidate and one comparison confirms it), and `format` and
`parse` switch on the code instead of up to 14 string comparisons per item.
300,000 x 2 formats: 9,612 M to 7,390 M instructions (-23%; Go 953 M); 300,000
parses: 4,824 M to 3,768 M (-22%; Go 462 M). The remaining 8x is the layout
parse itself, done in every call (`layout_items` 40% of format): Go's
`time.Format` walks the layout string without building items. Parsing once per
distinct layout needs a place to keep it (Xo has no globals), so the next
option is a compile-time parse of constant layouts in the compiler (the
checker already reads them for XO0420) or a cheaper walk without items; not
done here.

## Step 2, regexp (2026-10-21): flat capture store for the Pike queues

The run queues are preallocated once per match (instruction, thread flag, and
one flat list of group positions) instead of pushing an empty list per
instruction and copying a list per thread and per start position.
Instructions (M), Go in brackets: match 29,759 to 17,354 (3,943); find_all
18,697 to 9,704 (6,615); replace_all 16,033 to 8,772 (5,000); captures 1,919
to 1,130 (434). Still 2.0 to 4.4 times Go: next are Go's literal-prefix scan,
the one-pass matcher, and the bit-state backtracker.

## Step 3, regexp (2026-10-21): literal-prefix scan and the bit-state backtracker

Go's two fast paths that apply to the benchmark's patterns, ported from
regexp/backtrack.go and Prog.Prefix: (1) when no thread is alive, jump to the
next occurrence of the literal text every match starts with; (2) for a program
of at most 500 instructions and a text shorter than 262,144 / program length,
search by backtracking with a visited bit for each (instruction, position)
(`BT`, `try_backtrack`), leaving the Pike VM for large texts. One-pass is not
ported. Instructions (M), Go in brackets: match 17,354 to 8,186 (3,943);
captures 1,130 to 1,103 (434; this text is over the backtracker's limit);
find_all 9,704 to 9,754 and replace_all 8,772 to 8,802 (6,615 and 5,000; the
150 KB text is also over it, so they stay on the Pike VM). Total against the
start of the day: match 29,759 to 8,186 (7.6 to 2.1 times Go), find_all 18,697
to 9,754 (2.8 to 1.5), replace_all 16,033 to 8,802 (3.2 to 1.8), captures
1,919 to 1,103 (4.4 to 2.5). The corpus (260 patterns, 40 texts including a
48 KB one for the Pike VM) passes on go, arm64, LLVM.

## Step 4, csv format (2026-10-21)

A quoted field was written one rune at a time through a string built for
each rune; it is now written in runs of plain bytes, with only the quote, CR,
and LF rewritten. Format: 1,386 M to 621 M instructions (Go 740 M, now 0.84
times Go's encoding/csv). Parse is Go's reader ported line by line and stays
at 1.67 times (1,315 M against 787 M); a profile of it (7 samples, too short to
rank) shows read_line, index_rune, and utf8_valid, so a gain needs a rewrite of
read_record's byte scanning, not done.

## Step 5, time layout walk without items (tried, not kept)

A direct walk of the layout (a `Lw` cursor yielding literal and field chunks,
no item list, as Go's nextStdChunk) with the same codes was built and passed
the time module tests, but measured slower: format 7,390 M to 7,655 M, parse
3,768 M to 6,208 M (parse walks twice: once to validate, because a bad
layout must fault before any ParseErr). The cost is not the list: it is the
Str slices and compares made for each field name (body slice, find(":"),
name comparison) and the cursor's string fields, which a loop over bytes with
integer state would avoid but at a size comparable to rewriting format and
parse. Left as it was after step 1. The remaining 7.7x on format needs a
byte-level field recognizer (no Str for the name) or compile-time parsing of
constant layouts.

## Step 6, time: a byte-level field recognizer (kept)

`layout_items` recognizes a field in the layout's bytes (`field_code_b` and
`arg_literal` compare bytes in place; the argument is a literal constant), so
no Str is sliced for a field name or argument unless the layout is bad;
`format` writes into one StrBuilder. 300,000 x 2 formats: 7,390 M to 6,206 M
instructions; 300,000 parses: 3,768 M to 3,300 M (Go 953 M and 462 M, now
6.5x and 7.1x). What is left is mostly the item list and the small padded
number strings (`pad`), which a Go-style append of digits into the builder
would avoid.
