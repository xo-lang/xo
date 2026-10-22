# 0330: zero-copy Str and Bytes slices (views that share the buffer)

Status: Accepted (owner, 2026-10-21; proposed by Xo Lang Lib 1). Owner: Xo Lang Lib 1, with Xo Lang 2 reviewing the runtime and emitter changes; staged as in Decision item 5.

## Context

Spec core 3.6 says "slices of immutable values are O(1) and share
storage". The native runtime copies: `s[a..b]` and `b[a..b]` allocate and
copy the range (`prelude.llrt_str_slice_at`, `llrt_bytes_slice_at`). Go's
slices are free, so ports of Go code that slice in hot paths pay for it.
Evidence from 2026-10-21, native arm64, instructions retired:

- std/time layouts: every field name and argument sliced out of the layout
  cost enough that recognizing fields in bytes (no slices) cut format by 16%
  (bench/results/regexp-time-csv-vs-go-2026-10-21.md, step 6); Go's
  nextStdChunk compares bytes by index.
- std/mime `parse_media_type` and std/multipart: slicing for each token;
  `raw.str_at` + `llrt_str_slice_at` + `slice_check` are 20 to 30% of the
  profile.
- std/websocket `utf8_cut(chunk)` and streaming parsers cut a buffer at a
  rune boundary and carry the tail: each cut copies the rest.
- std/netip and std/regexp: `rune_at` and substring tests.

Hot paths work around it by indexing bytes, which is why the ports read
like C. A view would let them read like Go.

## Decision

Make slicing O(1) for `Str` and `Bytes` as the spec already says.

1. **Representation.** A `Str` or `Bytes` value becomes (buffer reference,
   offset, length). The buffer is the existing reference counted object; a
   view holds a count on it. Whole-value operations (len, index, compare,
   hash, concatenation, printing, system calls) take offset and length.
2. **Immutability makes sharing safe.** `Str` and `Bytes` are values that are
   never modified in place by others; `BytesBuilder` and `StrBuilder` own
   their buffers uniquely until `freeze`, which hands the buffer over. A view
   of a frozen buffer never sees a write.
3. **Memory.** A small view of a large buffer keeps the buffer alive. Rule:
   a view that is stored beyond the function (a struct field, a Map or
   List element, a return from a parse) is compacted (copied) when it is
   under 1/4 of its buffer and the buffer is over 4 KB, at the point it is
   stored; a view used in place is never copied. `s.clone()` copies
   explicitly.

   **Superseded by std cloning plus a vet note (owner, 2026-10-22).** The
   runtime does not compact. std parsers that keep a token of a large
   input copy it (`clone()`): url.parse_query and mime.parse_media_type
   over 4 KiB (bench/results/views-parsers-clone-2026-10-22.md; json, toml,
   multipart, and xml build their tokens; csv keeps views by design), and
   `xo vet --cost` notes (XO0911, decision 0327) a view of a parameter's
   text, made in a loop or taken from its split or lines, that is stored in a
   field or a returned collection: "this keeps the whole input alive;
   clone() it". Runtime compaction stays possible later and compatible.
4. **Runtime and ABI.** The layout of Str and Bytes is fixed in the C-free
   runtime (llrt/obj.xo, the static word table, the wasm runtime, and every
   emitter's inline string equality and hashing). Moving to (buffer, off,
   len) changes: string literals in rodata (a view with no count), the
   interned table, `rt.cast[Bytes, Str]`, `raw.str_at`, system call buffers
   (raw.bytes_addr = buffer + offset), the Go backend (Go strings and byte
   slices already are views, so it needs no change).
5. **Staged.** (a) Bytes first, with the websocket and mime/multipart ports
   as the measurement; (b) Str; (c) the rule in item 3.

## Consequences

- The slice-heavy ports (time, mime, multipart, websocket, url, http
  header parsing) lose their per-token allocations: expected 15 to 40% of
  their instruction counts, and the byte-index workarounds can go.
- A rune or byte boundary check stays on Str slices (a slice inside a UTF-8
  sequence faults, as now).
- Every place that reads `obj_addr(s) + layout(0)` as the data pointer
  (raw.xo, the emitters' string loads, TLS and socket buffers) must add the
  offset: a wide mechanical change with a fault surface on every backend,
  hence staging and the identity check (scripts/identity) at each stage.
- A one word wider Str and Bytes value (24 bytes: pointer, offset, length,
  or 16 with the offset packed into the length word for views under 4 GB):
  structs holding strings grow; benchmark before choosing.

## Measurements (2026-10-21, native arm64, instructions per slice)

A loop of 1,000,000 `b[i..i+n]` on an 8 KB Bytes (`len()` of the result),
setup removed: 16 bytes 166 instructions, 256 bytes 205, 4096 bytes 962; a
byte index costs about 8 instructions. So a small slice costs about 160
instructions of which the copy is a few: the allocation (and the free) is the
cost. Consequence for the layout:

- **One word view object** (a header [count, len, cap, data_ptr, parent],
  no byte copy): saves the copy only, so a 16 byte token slice drops from 166
  to about 140 (still one allocation) and a 4 KB slice from 962 to about 140.
  It helps large slices (streaming parsers, websocket frames) and not tokens.
- **Three word value** (pointer, offset, length in registers; no allocation
  for a slice): a token slice costs a few instructions. It is the only layout
  that makes time layouts, mime parameters, and header parsing cheap, at the
  price of changing value passing in the SSA, three emitters, wasm, and the
  llrt ABI.

## Layout decision (owner, 2026-10-21): option C

Xo Lang 2 noted that a Str (and Bytes) is already two words in the emitters
(object pointer, length; data at obj+16). Option C packs a view's offset
into the length word (len | off << 32; data = obj + 16 + (w1 >> 32)): a
slice allocates nothing (a new pair, the parent's count + 1), owned values
are unchanged (offset 0), and the header and call ABI stay as they are. In a
hot byte loop (2 index ops per iteration, 1M iterations) the emulated cost
over the loop's baseline was 17.3 M instructions today, 20.8 M for C
(unhoisted mask, shift, add) and 25.2 M for a one-word view object (B).

The owner accepted C with a **4 GiB cap** on one Str or Bytes value (a
language-visible limit; spec core states it with the change; a larger value
faults at construction). Supersedes items 1 and 5 of the proposed decision
(the three-word value and the Bytes-first staging): Str and Bytes share one
representation, so they move together. Owner: Xo Lang Lib 1, with Xo Lang 2
reviewing every emitter and runtime change.

## Implementation note (step 3, 2026-10-22)

Slicing is `rt.str_view` after the prelude's `llrt_str_slice_chk` /
`llrt_bytes_slice_chk` (range and UTF-8 boundary checks): the object gets one
more count and the value is (object, (hi - lo) | ((off + lo) << 32)). Views
are always on (STR_VIEWS) on arm64, x86 and LLVM; wasm and no-prelude builds
copy as before. Builders and in-place paths see only off 0: a value with
off != 0 is never unique (obj.xo reserve), and the inline push compares the
packed word unsigned against the capacity so a view takes the call. The cap
faults at concat, append and push (cabi, len_over) and at repeat (raw.xo
str_repeat, its own message). A small view keeps
its whole parent alive (a 10 byte token keeps a 100 MB read buffer), as in
Go. C ABI exports (cabi*.xo, `xo_x_` names) are the runtime's own: none returns a Str today, and a Str result would carry the packed word to its Xo caller unchanged (checked 2026-10-22; there is no user level export), so the export wrappers need no copy. The SSA builder refuses an export with a Str result (the export declarations in ssa/build.xo), so a future one cannot hand C the packed word.

## Alternatives

- **Keep copying and rewrite hot ports with byte indexing** (what was done
  today). No runtime change; code stays C-like and every new port repeats
  the workaround.
- **Views only for `Bytes`** (stage a only). Smaller change, solves
  streaming parsers; `Str` slicing stays O(n).
- **A separate `BytesView` type.** No ABI change, but a second type in every
  API and conversions at each boundary.

## Implementation map (step 1, 2026-10-21, for Xo Lang 2's review)

Layout C: a Str or Bytes is (object, w1) with w1 = len | (off << 32);
data = object + 16 + off (wasm: + 8 + off), len = w1 & 0xffffffff. Owned
strings, builders, and literals have off = 0, so their w1 is the length.
`rt.cast` Bytes/Str stays free. Slicing returns (object, new w1) and counts
the parent once (the Retain of a copy).

**Compiler (Str length and data readers).** Verified by reading and `xo refs`:

- SSA: the single length call is `Str.len` (also `Bytes.len`, `StrBuilder.len`,
  `BytesBuilder.len` map to it, ssa/call.xo:2168 and 2208); the string calls are
  `str.eq`, `str.cmp`, `str.concat`, `str.concat_int`, `str.concat_float`,
  `str.of_int`, `str.of_float`, `str.of_bool` (ssa/expr.xo and others
  build them). The layout table is `layout_value` (ssa/call.xo:1571: data
  offset 16 native, 8 wasm). Range facts: `Str.len` is 0 to MAX_STR_LEN =
  2^48 in arm64/ranges.xo:410 and llvm/ranges.xo:18 (a 4 GiB cap only
  tightens it).
- arm64: `Str.len` is `g.words(x)[1]` (calls.xo:989) -> becomes `and w, w1,
  #0xffffffff`; `str.eq`/`str.cmp` add 16 to word 0 for the data (calls.xo:896
  to 915) -> add the shifted offset too; the same shape in closures.xo (21
  `words(` uses), isel.xo (41), rc.xo (14, release only needs word 0), byref.xo
  (16), x86rt.xo (14, the x86 retargeting of the same ops), x86win.xo, gen.xo (12),
  inplace.xo, fieldlicm.xo:452 and opt.xo:581 (both match `Str.len` and must keep
  working on the masked form), lsr.xo, shrink.xo, debug.xo (8: DWARF names).
- LLVM: `str_parts` (llvm/call.xo:8) is the one place that GEPs the data
  (obj + 16) and takes the length (field 1); `Str.len` at call.xo:445;
  str.eq/str.cmp at call.xo:323 and 335; gen.xo (8 `Kind.Str` sites), func.xo,
  noalias.xo.
- wasm: rt2.xo and rt.xo implement `$str_eq`, `$str_cmp`, `$sapp*` over (ptr,
  len) i32 pairs; wasm/call.xo:301 to 319 route the calls; home.xo (3
  `Kind.Str`), gen.xo (2), func.xo (1). A wasm Str has a 32 bit length, so its
  packing is len | (off << 16)?? -- to decide: wasm32 values are 32 bit;
  options are a 16/16 split (64 KiB strings) or a two word w1 on wasm. Open
  question for the review.

**Runtime (llrt, Xo code).** Functions that take a `Str` read the value's
length through `s.len()` (-> `Str.len`), so they follow the compiler's
change for free; those that touch the object directly must add the offset:
`raw.bytes_addr`/`data` (raw.xo:13 to 15: `obj_addr + layout(0)`) and the 34
`rt.layout(0)` readers in std/native (digest.xo, dwline.xo, raw.xo and the
`obj_addr` users: 245 lines across aesgcm, bigmod, ed25519, fsops, h2srv,
hclient, hconn, hserve, mlkem, netconn, nistec, proc, sha256, sha3, tasks,
testworld). System call buffers and hashing take (address, length), so the
fix is in `bytes_addr` plus the callers that compute `obj_addr` by hand.
Header fields of a string object (count, len, cap) are for the owner; a view
never mutates the object, and an object with count > 1 is never modified in
place, so `rs_*` builders (unique) are untouched.

**Not affected.** The Go backend (strings and byte slices are views already),
the C ABI exports taking (address, length), List, Map.

**Hazards found reading the string runtime (obj.xo).** Anything that
treats the object as the value's own buffer must see only `off == 0`:

- `reserve`/`reserve_slow` (obj.xo:767 to 810), behind `rs_concat`, `rs_append`,
  `rs_push_*`, `rs_concat_int`: they append in place when the count is 1 and
  `cap >= need`, copy from `a + S_HDR`, and take the length as an argument.
  A unique view (its parent already dropped) has off > 0, so its bytes are not
  at `a + S_HDR`: the entry points must receive the packed word, decode (off,
  len), copy from `a + S_HDR + off`, and grow in place only when off == 0.
  The generated code's `xo_x_obj_rs_concat` calls pass (obj, len) today; they
  pass (obj, w1) after step 2.
- Casts that give a Str's object to a builder (`rt.rs_set_len`,
  `raw.bytes_len` shorten a unique object in place): applied only to values
  that came from `rs_with_cap`/`rs_new` (off 0); a debug assert on w1 >> 32
  in `bytes_len` and `rs_set_len` catches a view reaching them.
- Every site that passes a Str's second word to a runtime function as a
  length (Stdio.print, str_eq/cmp, fault messages, syscall buffers): it must
  pass the masked length; these are the `ws[1]` uses next to a `#16` data
  add in calls.xo (554, 896 to 920, 1315 to 1476, 2614 to 2625, 2854 to
  2864), closures.xo (290, 424), isel.xo:1351 and the matching LLVM and
  wasm sites.
- Hashing and equality of Map/Set keys (`obj.str_eq`, string hash) take
  (data, len): fine once callers pass them.
- Equality shortcut `a[0] == b[0] && a[1] == b[1]` (same object, same word):
  correct for views (same off and len means same bytes) and stays valid.

**Review points (Xo Lang 2, 2026-10-21), adopted.**

- Rule: a value with off != 0 is never unique. The uniqueness tests (the
  runtime's `count == 1` in reserve, rs_set_len, bytes_len, and the
  emitters' inline ones, including 0322's uniq_var_params reuse) also test
  the w1 high half; step 1 lists them all (`xo refs` on rc.claim, reserve,
  and the Reuse op) before any edit. Golden `views_unique`: slice a big
  Bytes, drop the parent, push to the view, check both contents.
- Whole-word uses of w1 (closures.xo equality, isel Str compares, Map key
  hash and compare, switch on string, literal dedup) mask the length.
- fieldlicm.xo:452 and opt.xo:581 match the helper's AND after step 1; the
  maxby and lexer rows must not regress.
- wasm keeps copying (off always 0); shared llrt code reaches w1 through the
  helper, which folds to the length on wasm.
- The cap fault has a clear message, a spec line in core, and a std
  change-log entry; x86 edits (x86rt.xo, isel) run the 42 golden Docker
  amd64 sample.
- Threshold measured (instructions per slice of an 8 KB Bytes, native
  arm64): 16 bytes 166, 32 174, 64 176, 128 188, 256 205. A copy costs the
  same ~160 instructions of allocation plus 0.15 per byte; a view costs one
  count increment now and a decrement later (atomic once threads run, 20 to
  40 cycles shared). In instructions the break-even is below 16 bytes; in
  cycles under sharing it is the atomic against the allocation and free (a
  heap round trip of hundreds of cycles), so views win at every size; the
  compaction rule (item 3) bounds the pinning. Decide the threshold from
  the mime/multipart rows after step 3, starting at 0.

## Steps (each gated by identity, the codegen patterns, the wasm sample,
arm64 and LLVM goldens, and the mime/multipart/time baseline)

1. `Str.len`, `str.eq`/`cmp` and the data address through two helper
   functions in each emitter (so every other site changes by calling them),
   offset always 0: no output change; identity proves it.
2. `raw.bytes_addr`/`data` and the llrt readers add the offset.
3. The slice call (`llrt_str_slice_at`, `llrt_bytes_slice_at`) returns a
   view when the range is inside the object and the result is >= 16 bytes
   (small slices keep copying: a 16 byte copy is cheaper than a count
   bump), with the cap fault at construction.
4. Hot ports drop their byte-index workarounds one by one, each with its
   timing row.

## Implementation note (step 4, 2026-10-22)

The cost of step 3 in `Layout.format` was not the view offset and length
mask in loops (`hoist_invariants` moves them, and a function computes a
Str's data address once per block): it was the cap check, a call per
pushed byte. The check is now in line in every runtime entry. Figures:
bench/results/views-cap-inline-2026-10-22.md.
