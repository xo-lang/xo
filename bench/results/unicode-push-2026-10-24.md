# Case mapping: the changed rune is pushed inline, 2026-10-24

Third item from the mixed-text case profile (`case-mixed-2026-10-24.md`,
after `unicode-u24-2026-10-24.md`). `unicode.case_append` sent every
changed rune through `raw.push_rune`, which re-checks the scalar value
and then takes the `rt.rs_push_word` path into `obj.rs_push_word`: two
calls and a range check per changed rune, where the rune came from the
decoder or from a case table and is a scalar value already.

## The change

`case_append` encodes the changed rune where it stands, with
`rt.rs_push_byte` for one byte and `rt.rs_push_word` for two, three and
four, which is `raw.push_rune`'s encoding without the call and the
check. `raw.push_rune` is unchanged for its other callers.

## What it is worth

`bench/perf/unicode`, Xo arm64, base worktree at `origin/main` and the
branch built by an xo of the same revision, both programs from the same
`main.xo`, `xo bench --ab --rounds=9 --keep-loaded`, instructions,
ratio B / A:

| workload | base | now | B / A |
|---|---|---|---|
| lower_u (mixed to_lower) | 3,550,612,506 | 3,088,959,522 | 0.870 (0.870 to 0.870) |
| upper_u (mixed to_upper) | 3,510,925,481 | 3,088,102,313 | 0.880 (0.880 to 0.882) |
| eqlower (case insensitive compare) | 9,510,877,411 | 8,181,511,922 | 0.860 (0.860 to 0.860) |
| runecase (Rune.to_lower/to_upper) | 151,454,331 | 151,422,685 | 1.000 (0.999 to 1.000), within noise |
| lower (ASCII) | 378,214,438 | 378,224,031 | 1.000 (1.000 to 1.000), within noise |
| upper (ASCII) | 386,586,579 | 386,637,143 | 1.000 (0.999 to 1.000), within noise |

`runecase` does not push a rune (it returns the mapped one), and the
ASCII rows never take the changed-rune path, so those three are controls.

## Correctness

- Corpora `runes`, `strings`, `trimchars`, `equalfold` and `strwhere` on
  arm64, LLVM and the Go backend.
- Goldens `unicode_table` and `native_eq_lower` on arm64 and LLVM, and
  the Go backend against the same `want.txt`.
- The wasm goldens that carry case mapping (the same set as the wasm
  check for the little endian tables).
- The full fixed point.

## What is left

The case rows now decode inline, read the tables in one load, fold the
ASCII runs in the runtime's append, and push a changed rune without a
call. The next profile would have to be taken on a longer run than this
bench (97 ms of CPU gave 14 samples); nothing in the earlier profile is
still above ten percent.
