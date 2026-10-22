# Baseline for decision 0330: mime and multipart with copying slices (2026-10-21)

The numbers option C (offset packed in the length word, no allocation per
slice) is judged against. xo built from main at the parent of this commit;
linux/arm64 in Docker (scripts/docker-linux.sh, Debian trixie), the arm64
backend, valgrind callgrind, two runs per program (instruction counts are
exact and the runs differ by 566 instructions).

## Programs

- **mime**: `mime.parse_media_type("text/html; charset=utf-8; boundary=\"x y\";
  n=<i>")` 200,000 times, summing the kind's length and the parameter count.
- **multipart**: `multipart.parse` of a three-part body (one part with a file
  name and a content type) 100,000 times.

(bench/results/json-vs-go-2026-10-21.md has the Mac wall and instruction
figures against Go's packages for the same programs.)

## Totals (Ir)

| program | run 1 | run 2 |
|---|---|---|
| mime x 200,000 | 1,724,159,855 | 1,724,159,289 |
| multipart x 100,000 | 4,796,985,712 | 4,796,985,146 |

That is 8,620 instructions per parse_media_type and 47,970 per multipart.parse
(about 16,000 per part with three headers).

## Where it goes: the slice machinery

Functions in the profile that exist because a slice copies (share of the
total; the sum is what option C aims at):

| function | mime | multipart |
|---|---|---|
| prelude.llrt_str_slice_at | 10.49% | 2.69% |
| prelude.slice_check | (below 1%) | 4.35% |
| raw.str_at (builds the copy) | 6.58% | 3.98% |
| libcstr.memcpy | 5.98% | 3.69% |
| obj.rs_new_n (two entries: obj and alloc) | 5.32% + 4.70% | 3.37% + 2.97% |
| alloc.free1 + cabi.obj_rc_free (releasing the copies) | 4.21% + 3.70% | 3.19% + 2.45% |
| strings.trim (trim slices) | 6.37% + 1.47% | 1.56% |
| sum | about 49% | about 27% |

The rest is the parsing itself: mime `consume_token` 6.4%, `parse_media_type`
3.5%, `scan.find`/`eq_at` 5.6%, `Map.put` 2.2% (mime); `canonical_key` 6.1%,
`read_header` 5.3%, `index_of` 5.1%, `read_line` 4.8% (multipart).

So with allocation-free slices the expected floor is about half the
instructions of parse_media_type and about three quarters of multipart.parse
(the copies are not all slices: header values stored in the Map are real
strings and stay).

## Reproducing

`scripts/docker-linux.sh arm64 WORK -- sh run.sh` with a linux/arm64 xo built
by `xo build --backend=go --targets=linux/arm64 ./compiler/xo`, the programs
built in the container with `--backend=arm64`, and
`valgrind --tool=callgrind ./prog run`, then `callgrind_annotate`. The
scripts are in ~/xo-lead/notes/lib1/ (cg-run.sh).
