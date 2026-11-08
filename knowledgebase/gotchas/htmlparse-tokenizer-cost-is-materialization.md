# htmlparse: the cost was token materialization and byte reads, not compares

Observed 2026-11-08 (host 1, linux/arm64, `--ab` on bench/perf/htmlparse, 7 rounds).

Symptom: std/htmlparse ran 1.65x x/net's Parse and a flat profile (tokenizer 55%, str_eq 6%) suggested text tag compares
(Go uses atoms) as the next cost.

What measured:

- Worked (tok workload, tokenize only, each against its parent): skipping the lower_bytes and replace_nul copies for a
  name with no upper-case byte or NUL 0.962; answering `amp;`, `lt;`, `gt;`, `quot;`, `apos;`, `nbsp;`, `copy;` before
  entity_of's binary search 0.879 (but 1.017 on a page of only uncommon references); one `match` on the token type
  instead of up to six enum equality calls 0.969; peeking the next byte in place instead of `tokenizer_rb` and backing
  up, in read_tag and the attribute value reader 0.903 and in `next` 0.842. Together tok 0.633, parse 0.749.
- Did not work: copying the text between references in `unescape` in runs, not bytes, 1.008; a per-node cache of the
  scope and special facts (replacing about ten string compares per open element in the scope walks) made parse 1.079,
  the extra list and flag reads costing more than the compares; returning literals for the 35 usual tag and attribute
  names (no allocation) 1.007.
- Later the same day (parse profile at 5 kHz: str_eq 4.6% over 11 callers, none above 1.3%; utf8_valid 5.1%): testing a
  node's namespace for empty by length at 17 sites 0.989 (0.981 to 0.994, n=6), kept; token text and names with nothing to
  convert returned as `Str` slices (views) of the input, to skip the copy and the UTF-8 check of `to_str_lossy`, tok 1.079
  and parse 1.071 (worse: carrying the input `Str` in the Tokenizer and slicing it per token costs more than the check it
  saves; llrt's raw.str_cut cannot be used, the Go backend compiles this module). Integer atoms are capped near 3.5% (the
  whole str_eq share minus what the length test took) and were not built.

Cause: per-call overhead of a non-inlined byte reader and per-token allocations, not string compares. Profile with a
tokenize-only workload first (`XO_PROF=cpu` on host 1), and `--ab` each step alone; an idea that removes work can still
add more list traffic than it saves.
