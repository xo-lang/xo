kind: review
module: csv
result: pass
commit: 48d129239
source: 7505f63ca8098b6d34d1b4c130678c85b855ef91de0a323cf36ab640af7959db
date: 2026-11-14
host: darwin/arm64 (reading and probes)
command: none (section 3 does not apply: no secret); read against knowledgebase/infrastructure/std-security-checklist.md
measure: 0 Go vulnerability entries for encoding/csv (182 stdlib entries read, 2026-11-14); 3 inputs a peer controls (text to parse or parse_maps, a file to a Reader, records to format when a program writes a peer's fields), none with a size limit (as Go), each linear in its input on the probes (a 1.2 MB quoted field of escaped quotes 6 ms; a 128 MB line through a Reader 27 ms); 0 defects open (F1, a Reader returned a file's bytes that are not UTF-8 as Str and csv.format faulted on such a field, and F2, a Reader rescanned a long line after every 64 KiB, quadratic, both fixed in 742fd0387); 0 fault sites reachable from input; 1 difference from Go documented (G1: a Reader turns bytes that are not UTF-8 into U+FFFD where Go keeps them; its lazy_quotes path that read them as an empty field fixed in 9feb50523); check_delims' fault a caller contract; 0 insecure defaults

Security review of std/csv (std/csv.xo, the declarations, and
std/native/csv.xo, the native bodies; std/rt/csv.go runs Go's
encoding/csv itself on the Go backend) against
knowledgebase/infrastructure/std-security-checklist.md (decision 0616),
by Xo Lang 2. std/csv is a port of Go's encoding/csv Reader (reader.go
readLine and readRecord with Comma, Comment, FieldsPerRecord, LazyQuotes
and TrimLeadingSpace) and Writer (writer.go Write and fieldNeedsQuotes
with UseCRLF), decisions 0164 and 0298: parse, parse_maps, reader and
read, format, valid_sep and valid_comment (spec/std.md 16). It reads
text a peer sends: uploads, exports fetched from a URL, files.

The probes of this review are Xo programs outside the repository
(std/csv has no _test.xo): ~/xo-lead/notes/xo-lang-2/prov/review/
csvprobe (sizes and times), csvutf8 (what a program can do with a
Reader field that is not UTF-8) and csvfault (F1's reproducer), built
with the xo of thr0638 against the std of 97fc633a8, native backend;
after the fixes, the second reader's probes reader4/ (edge, long,
fault) with the xo and std of 3839b835f (~/xo-wt/csvsec), native and
the Go backend, which gave the figures marked "after".

## 1. Go's CVE history

The Go vulnerability database (vuln.go.dev, index/modules.json and the
182 distinct stdlib entries, fetched on 2026-11-14) has no entry for
encoding/csv.

Sibling classes, checked:

- Unbounded reads from a peer (GO-2021-0142, encoding/binary's
  ReadUvarint read without bound): csv has no length prefix; every
  read is the text's own bytes, and a Reader reads 64 KiB at a time
  (native/csv.xo:62) only as far as the next record needs.
- Quadratic work on repeated syntax (the class of many parser entries,
  such as GO-2026-4341 in net/url): a quoted field of escaped quotes,
  a quoted field over many lines, lazy quotes, many records, many
  fields, many blank or comment lines, long leading space and one long
  line through parse all ran linear on the probes (section 2); one
  long line through a Reader was quadratic (F2, fixed in 742fd0387).
- Text that is not UTF-8 reaching a Str (decision 0259's class): F1,
  fixed in 742fd0387.

## 2. Limits on hostile input

std/csv sets no size limit, as Go's encoding/csv sets none: a record
holds as many fields and bytes as the text gives it, and a Reader holds
one record (plus the 64 KiB chunk it reads) at a time. What a program
gets is bounded by the text it passes (parse) or the file it opens
(reader); a program that takes text from a peer bounds that text, as
with Go. Quote nesting does not exist in CSV (a quote inside quotes is
`""`), and there is no recursion.

| Input a peer controls | Limit | Where | Probe (csvprobe, Mac, native) |
|---|---|---|---|
| Bytes of one quoted field of escaped quotes (`""` repeated) | none (Go: none); linear | read_record (native/csv.xo:232-275), buf grows by appending | 25,000, 100,000 and 400,000 pairs (50 KB to 800 KB): 0, 1 and 6 ms |
| Lines of one quoted field | none; linear | read_record (258-266) | 25,000 to 400,000 lines: 0 to 5 ms; through a Reader, 400,000 lines 4 ms |
| A quote run with lazy_quotes | none; linear | read_record (253-254) | 1.2 MB: 5 ms |
| Records | none; linear | read_all (288-308) | 400,000 records: 30 ms |
| Fields of one record | none; linear | read_record (190-226) | 400,001 empty fields: 2 ms |
| Blank and comment lines | none; linear | read_record (168-181) | 400,000 each: 6 ms |
| Leading space with trim_leading_space | none; linear | read_record (191-208) | 400,000 spaces: under 1 ms |
| One line through a Reader | none (Go's bufio grows likewise); linear since F2's fix (each chunk read is searched once, into a builder); memory linear | fill (54-73), read_line (76-123) | 4, 16, 64 and 128 MB with no line break: 2, 25, 363 and 1489 ms at 97fc633a8 (second reader's probe reader2/longline); 128 MB 27 ms after; corpus csv "reader of lines longer than several read chunks" |
| Fields per record | `fields` (FieldsPerRecord): a record of another count is `wrong number of fields`, as Go | read_all (297-304), Reader.read (334-340) | corpus csvfuzz (fields -1, 0 and 2 against Go) |
| parse_maps header keys | a Str keyed Map; a duplicate key keeps the later field (Go has no parse_maps). Hash flooding: Xo's Str hash is seeded per process from the operating system's generator (decision 0545), as Go seeds its maps | parse_maps (349-365) | corpus csv |

The lines and columns of errors are Int counters of the text's own
lines and bytes, far from overflow.

## 3. Constant time

Not applicable: std/csv handles no secret.

## 4. Fault behaviour

### F1 (fixed in 742fd0387): a Reader returned bytes that are not UTF-8 as Str, and format faulted on them

At 97fc633a8, `csv.reader(f).read()` made each field with field_str,
`rt.cast[Bytes, Str]` with no check. That holds for parse, whose text
is a Str and whose field boundaries are a separator, a quote or a line
end, but a Reader's bytes come from a file, which may hold anything,
so a peer's file put text that is not UTF-8 into a Str, against
spec/core.md 3 and decision 0259. std/rt/csv.go's M_read did the same
on the Go backend. format slices a field after each quote and relies
on the field being valid text, so the 7 byte file `"x""\x82"` plus a
line break, read and written back, stopped the program (`slice 2..3 of
Str splits a UTF-8 sequence at std/native/csv.xo:394:17` at
97fc633a8, exit 70; reproducer ~/xo-lead/notes/xo-lang-2/prov/review/
csvfault). Since the Reader landed (6621f603f, 0.38.0).

Fixed: field_str (154) takes whether the Rd has a file; a Reader's
fields go through scan.text_of after the parse (each byte of an
invalid sequence U+FFFD, decision 0259's helper), so error positions
stay Go's byte columns, and parse's fields, cut from a Str, keep the
cast. The call sites (219, 248, 251, and 272 since 9feb50523) pass
`r.f is Some(_)`.
std/rt/csv.go's Reader returns `ListOf(textsOf(rec)...)`.
TestOutsideBytesValidated's entry for native/csv.xo
(outside_bytes_test.go:94) still counts the one cast, now parse's
only. Test: corpus csv "reader over a file of invalid UTF-8 gives
valid fields" (the reproducer's bytes and one more; the record and
format of it) on arm64, LLVM and the Go backend.

### F2 (fixed in 742fd0387): a Reader rescanned a long line after every chunk it read

At 97fc633a8 read_line looked for the line break from the start of
the unread text after each 64 KiB fill and copied the buffered line,
so a line of L bytes with no break cost about L^2 / 128 KiB: 4, 16, 64
and 128 MB took 2, 25, 363 and 1489 ms (Mac, native; found by the
second reader). Fixed: a line past the buffer gathers the chunks in a
BytesBuilder and searches only each new one (read_line, 79-102), as
Go's bufio scans only what it adds: 128 MB in 27 ms. parse is not
affected (its Rd has no file, so the first fill marks it done). Test:
corpus csv "reader of lines longer than several read chunks" (a 330
KB line, with and without quotes, against parse).

### G1 (difference from Go, not a fault)

- A Reader turns each byte of an invalid sequence into U+FFFD (F1's
  fix), where Go's strings keep the bytes; written back with format,
  such a field differs from the file. Decision 0259's rule, on every
  backend.
- Until 9feb50523, a Reader with lazy_quotes whose text ended inside
  quotes on such bytes kept `buf.to_str() ?? ""`, an empty field on
  native backends and U+FFFD on the Go backend (second reader). Now
  272 goes through field_str like every other field; corpus csv
  "reader with lazy quotes, a file ending inside quotes on bytes that
  are not UTF-8, gives U+FFFD". TestOutsideBytesValidated lists
  native/csv.xo field_str and std/rt/csv.go's Reader M_read as
  boundaries since 9feb50523, so dropping either check fails it.
- A Reader's fields are views of its read buffer (text_of returns the
  bytes without a copy when they are valid), so a program keeping one
  short field per record keeps that record's buffer alive: up to a 64
  KiB chunk, or a whole long line. parse's fields keep its text alive
  the same way. Not a defect; a program keeping a few fields of many
  records copies them.

### Fault sites

- format (native/csv.xo:405 and 420): not reachable from input since
  F1's fix: every field is valid text (parse's are slices of a Str, a
  Reader's go through text_of), and the slices fall on a quote, CR or
  LF (single byte characters), so never inside a sequence.
- check_delims (280-285) faults `csv: invalid field or comment
  delimiter` for a `sep` or `comment` Go's validDelim refuses, or a
  comment equal to sep. These are the program's options, not the text;
  a program that takes a separator from a peer checks it with
  valid_sep and valid_comment first, as documented (csv.xo:49-59). A
  caller contract.
- read_line indexes `line[n - 2]` only for n >= 2 (112) and
  `line[line.len() - 1]` only when the line is not empty (118).
- read_record: `line[0]` after `line.is_empty()` (209); `line[1..]`
  after `line[0] == '"'` (228) or after the rune `"` (243);
  `line[i + sep_len..]` after index_rune found the separator at i,
  whose encoding is sep_len bytes (221); `line[sep_len..]` after the
  first rune was decoded as `sep` (246): valid_delim refuses U+FFFD as
  a separator (25), so a byte that is not UTF-8 (decoded as U+FFFD,
  one byte) can never match a multi-byte separator. `line[i..]` after
  trimming stops at a decoded rune boundary (206).
- parse_maps: `recs[k][j]` for j below the header's count, and
  read_all with fields 0 has refused any record of another count (356-
  362).
- needs_quotes: `b.decode_rune(0)` only on a non-empty field (379).
- No `!` unwrap, `or_fault` or arithmetic that can overflow on input.

## 5. Defaults

- lazy_quotes is false, fields is -1 (any count), trim_leading_space
  false, comment none: Go's defaults except FieldsPerRecord, where Go's
  zero value is "the first record's count"; -1 accepts more, which is
  not a security property.
- format quotes a field starting with a space, holding the separator, a
  quote, CR or LF, or `\.`, as Go. Neither Go nor std/csv neutralizes
  spreadsheet formulas (a field starting `=`, `+`, `-`, `@`): CSV
  injection is the program's to handle when it writes a peer's text for
  a spreadsheet, as with Go.
- No option weakens a check.

## Sign-off

Second reader: a subagent of Xo Lang 2 (2026-11-14), reading the code
at 97fc633a8 separately: needs changes (one: the Reader on one long
line is quadratic, not linear; added above as F2 with its probe and
the measure line changed), made above. It confirmed F1 by running the
reproducer (bytes 78 22 82, a fault in csv.format at csv.xo:394, exit
70), that field_str and std/rt/csv.go M_read pass the file's bytes
unchecked, every file:line reference, and that the fault site list is
complete: the only Str slicing is format's (405, 420), and every Bytes
index and slice in read_line and read_record is guarded as listed,
including 221 and 246 with a multi-byte separator. F1 and F2 were open
at 97fc633a8; the line numbers above moved to 3839b835f's, and the
review is redone at 3839b835f below.

Third reader: a second subagent of Xo Lang 2 (2026-11-14), reading the
code at 3839b835f separately, confirmed both fixes from 742fd0387. F1:
every Reader field passes through scan.text_of (219, 248, 251) or, on
the Go backend, textsOf; the csvfault reproducer now reads 78 22 EF BF
BD and formats it with exit 0; nine files of invalid UTF-8, a sequence
split across a 64 KiB chunk among them, gave only valid fields and no
fault in format, on native and the Go backend. F2: one line of 16, 64
and 128 MB read in 5, 22 and 27 ms, 4 million short lines in 239 ms,
and the Reader matched parse on twelve chunk boundary cases (LF and
CRLF split across chunks, EOF on a boundary, quoted lines spanning
chunks, a split multi-byte character). Every native/csv.xo line
reference and the source hash matched 3839b835f. It asked for four
changes, all made: the lazy end inside quotes now goes through
field_str (9feb50523, G1), field_str and M_read are boundaries of
TestOutsideBytesValidated (9feb50523), the post-fix probes name their
xo, and the review lands at std/evidence/csv/review.md as the code
comments cite.
