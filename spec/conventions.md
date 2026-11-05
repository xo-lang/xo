# Xo Conventions

Status: draft for the owner's review (Xo Lang 2, 2026-11-05). Not mirrored on the website until the owner
approves it (scripts/sync-content.py lists the spec files it copies).

How to write Xo that reads like the rest of Xo and runs fast. spec/core.md defines the language; this file
collects the habits that the compiler, std, the gotchas and the measurements agree on. Every rule names its
source. **Enforced** rules are checked by the compiler, `xo fmt` or `xo vet` (the code is given); **advisory**
rules are not checked, and recurring ones may become vet checks later.

Sources are paths in this repository: `core` is spec/core.md, `std` is spec/std.md, `gotcha X` is
knowledgebase/gotchas/X.md, `perf guide` is knowledgebase/std-performance-guide.md.

## 1. Naming

1.1 **Types, variants and interfaces are UpperCamel; functions, methods, variables, fields and modules are
snake_case; constants are UPPER_SNAKE.** Enforced (XO0101). Source: core 1.5.

1.2 **Do not reuse predeclared names** (`Str`, `Time`, `Err`, `Result`, `ok`, `err`) for your own types or
variables; name what the thing is (`TimeRes`, `LevelErr`). Enforced at top level (XO0603). Source: gotcha
predeclared-names-collide.

1.3 **An error enum is `<Thing>Err`, implements `Display`, and a blocking API's has a `Canceled` variant.**
Advisory (std's convention). Source: std section 0; core 2.3.

```xo
enum BufErr {
  Net(err: NetErr)
  TooLong
  UnexpectedEof
}
```

1.4 **Do not let a local shadow a parameter of the same function.** Only same-block shadowing is an error, so
the checker allows it. Advisory. Source: gotcha xo-local-shadows-parameter (a `var us` hid the parameter `us`).

1.5 **Name the arguments when a call takes two or more of the same type.** Advisory. Source: core 3.2.

```xo
transfer(from: a, to: b, amt: 5)
```

1.6 **Convert numbers by calling the type: `Float(n)`.** Enforced (XO0601, with an `xo fix` rewrite). Source:
gotcha numeric-conversion-is-call.

## 2. Effects and capabilities

2.1 **A `pub` function declares its effects in `uses`, and only the ones it performs.** Enforced: a missing
effect is XO0701 (error), an unused one XO0703 (vet warning). Source: core 6.1, 6.2.

```xo
pub fn fetch_user(db: Db, id: Int) -> User ! LoadErr
  uses net
  requires id > 0
```

2.2 **Library code takes the capabilities it needs (`Fs`, `Net`, `Clock`) as parameters and narrows them**
with `sub`, `read_only` or `only`; only `main` creates them. Advisory (`xo audit` reports the reach). Source:
core 6.3.

## 3. Errors, Option and Result

3.1 **Propagate with `?`; convert errors with `from` variants or `.map_err`.** Enforced (XO0202 when the error
does not convert). Source: core 2.6. For Go habits, see knowledgebase/guides/option-result-for-go.md.

3.2 **Never drop a Result silently: `.ignore_err("reason")`, never `let _ =`.** Enforced (XO0201, XO0206; vet
note XO0208 lists every reason for review). Source: core 2.6.

```xo
Err(_) => os.fs.remove(tmp).ignore_err("another process stored it")
```

(compiler/cli/core/check.xo:528)

3.3 **Do not turn an I/O error into a default** (`.ok() ?? []`): match it, keep the default for `NotFound`
only, or mark a deliberate case `// xo:allow XO0903 <reason>`. Enforced (vet warning XO0903). Source: gotcha
io-error-defaulted-away.

3.4 **Prefer the short forms of `??` to a four-line match.** Enforced (vet note XO0906). Source: core 2.5, 2.6,
decision 0321.

```xo
let n = fields.get(k) ?? continue
let v = fetch(id) ?? e => return Err(LoadErr.Fetch(e))
```

3.5 **Turn an Option into a Result with `.or_err(E)?`**; a bare `opt?` in a Result function is an error.
Enforced (XO0204, XO0205). Source: core 2.5.

3.6 **Check inputs with `requires ... else E`; for a value that cannot be missing, use `or_fault` or
`get_or_fault` with a reason.** Advisory. Source: core 9.1, 3.6; std.

3.7 **Read a map with `get`, `get_or_fault` or `update`, never `m[k]`.** Enforced (XO0410). Source: core 3.6.

3.8 **Inside `Mutex.with`, return a Result as the closure's value and apply `?` outside**: an `Err` returned
from inside undoes the write. Enforced (checker warning XO0540). Source: core 5.4; gotcha
mutex-with-err-undoes-state (a WebSocket flake).

```xo
let outcome = q.state.with(fn(var st) -> Result[Job, QueueErr] { ...; Ok(st.ack(id)) })
outcome?
```

3.9 **Keep a pure method's result: `xs = xs.reverse()`, or use the in-place form (`sort_in_place`).**
Enforced (XO0209). Source: core 2.6; gotchas list-reverse-returns-copy, list-sort-returns-a-copy; std
(sort_in_place, decisions 0374, 0508).

## 4. Values, collections and strings

Values have value semantics: a copy is independent, and a write to a shared value copies it first. Most
rules here avoid copies the program never needed.

4.1 **Change a map value with `Map.update`, not get, change and put.** A forgotten put loses the update, and
the copy costs time. Enforced in part (XO0520 error for a mutation never stored; vet note XO0532 for the long
form). Source: core 5.2; gotcha value-semantics-lost-update. A closure that captures still allocates its
environment per call (gotcha map-update-copied-value).

```xo
us.update(a, [], fn(var l) { l.push(x) })
```

(compiler/arm64/byref.xo:532)

4.2 **Change list elements in place (`xs[i].push(v)`), not by copying out and storing back.** Enforced (vet
note XO0536). Source: core 5.1; gotcha element-copy-store-back-quadratic (a PE link ran past 24 minutes and
did not finish, then took 276 s). Measured: `g.blocks[g.cur].insts.push(x)` 63,580 µs before in-place updates,
104 µs after (bench/results/nested-update-2026-10-11.md:19).

4.3 **Clone a Str or Bytes slice before keeping it**: a slice is a view of its parent (decision 0330) and keeps
all of it alive. Enforced in part (vet --cost note XO0911). Source: core 3.6; gotchas slice-view-of-reused-buffer,
slice-facts-pin-parent. Measured: 2,000 kept views held 2.1 GB, cloned 4.9 MB
(bench/results/views-retention-2026-10-22.md:14-15).

```xo
out.push(NlMsg{ty: ty, body: buf[off + 16..off + len].clone()})
```

(std/native/llrt/ifaces.xo:165)

4.4 **Size a List when the size is known: `List.with_capacity(n)`** (also `BytesBuilder` and `Buf`). Advisory.
Source: std; bench/results/capacity-hints-2026-11-05.md:55-56 (0.85 and 0.862 of the time);
bench/results/buf-with-capacity-2026-11-05.md:16 (0.938). Map, Set and StrBuilder take no hint.

4.5 **Build text by appending to a value nothing else holds (`s += piece`, `"${s}${n}"`); do not build a piece
only to append it.** Advisory. Source: perf guide 3 (template render 0.934, 37% fewer allocations);
bench/results/nested-update-2026-10-11.md (`rows[0].text += "x"` 2,455 µs to 172 µs). A StrBuilder did not pay
for small results (perf guide 8); there is no current A/B of `s = s + x` in a loop against a builder, so this
rule's builder half is open.

4.6 **Append unchanged runs in one call, not one element at a time.** Advisory. Source: gotcha
std-port-costly-go-idioms (map_runes 1.20 to 1.38x Go before, 0.86 to 0.90 after).

4.7 **Prefer a List indexed by a dense id to a Map; do not memoize cheap work in a Map.** Advisory. Source:
gotcha memo-map-slower-than-recompute (memos took cancel's CPU from 3.8 s to 7.6 s).

4.8 **Sort by an Int key, not a tuple holding a Str.** Advisory. Source: gotcha
sort-by-tuple-key-and-map-list-append (7.2 s of a 174.7 s compile).

4.9 **Use `Bytes` for byte data.** A `List` of a sized integer takes 8 bytes per element on native targets
today (10M UInt32 and 10M Int both used 82,706,432 bytes). Advisory. Source: gotcha
sized-int-lists-take-eight-bytes (may change with decision 0369, Proposed).

4.10 **Write `()` for an empty arm body**: `{}` in expression position is an empty Map. Advisory. Source:
core 1.6; gotcha fmt-empty-block-is-map.

4.11 **In a pattern a bare name is a variant or a new binding, never a constant; compare constants with
`==`.** Enforced (XO0302, XO0430). Source: gotcha const-in-match-pattern.

4.12 **Parenthesize `??` inside a slice bound: `digits[0..(Int.parse(a) ?? 9)]`.** Advisory. Source: gotcha
coalesce-binds-looser-than-range.

## 5. Performance

Measured costs from std's ports against Go; each claim carries an A/B result (decision 0340).

5.1 **Compute tables and parsed layouts once in a top-level `const`** (computed lazily, once per process,
decision 0332), not per call; global mutable state stays forbidden (XO0416). Enforced for time layouts (vet
warning XO0916), advisory otherwise. Measured: parsing a layout per call 5,344 M instructions, a Mutex map
cache 1,995 M, a lazy const 1,710 M, Go 983 M (bench/results/time-format-cache-variants-2026-10-22.md:12-15).
This replaces the older advice of no package-level caches.

```xo
const HTTP_DATE_LAYOUT: time.Layout = time.layout("{weekday:short}, {day} ...")
```

(std/native/http.xo:2045)

5.2 **No small List per loop iteration**: use locals, or one list made before the loop. Enforced for std (vet
--cost note XO0914, checked by cmd/xo TestVetCostStd), advisory elsewhere. Source: perf guide 3 (path parts
0.927 with eight locals).

5.3 **In hot loops, read by offset instead of slicing to compare.** Enforced for std (vet --cost note XO0913).
Source: perf guide 4 (path is_match 1.70x Go to 1.00x).

5.4 **Keep values that travel inside other values to a word or two; avoid wide structs passed by value.** A
copy costs one count per heap field. Advisory (vet --cost note XO0910 above 8 words). Source: core 5.1; gotcha
wide-struct-by-value-copies (a 28-word Zone: zone_load 1.744x to 1.001x).

5.5 **Before spawning a task, take only the fields it needs (`let part = c.field`).** Enforced at 32 or more
heap fields (vet note XO0534). Source: core 5.1.

5.6 **Do not write an unchanged value into a collection that may be shared; test first.** Advisory. Source:
gotcha shared-list-write-same-value (5.75 MB of copies).

5.7 **Hoist string compares and mode dispatch out of hot loops; dispatch on an Int.** Advisory. Source:
bench/results/strcompares-2026-10-24.md:22-27 (43% of the run in the dispatch).

5.8 **In word arithmetic, use the wrapping and logical forms where overflow is wanted or impossible**
(`wrapping_add`, `wrapping_shl`, `shr_logical`; mask instead of `% 4`): `+ - * <<` on Int are checked.
Advisory. Source: gotchas std-port-costly-go-idioms (RSA sign4096 7.8%), xo-checked-word-arithmetic,
int-no-wrapping-shr.

5.9 **Function values borrow their arguments, so `any`, `filter` and `fold` with a closure are cheap.** A
measured fact, not a rule to avoid them. Source: bench/results/callbacks-0337-2026-10-22.md:19-21 (31 to 34%
less after decision 0337).

## 6. Memory

6.1 **Break cycles through callbacks**: native memory is reference counted with no cycle collector, so a
callback stored in the object it captures is never freed. Drop such hooks on close or on a final error.
Advisory (the arm64 goldens' leak check catches it). Source: gotcha callback-capturing-its-owner-leaks;
decision 0395. See core 5.5.

6.2 **std and runtime code: keep a value alive while a raw address into it is in use** (`raw.alive(b)` after
the last raw access when the address goes into a field, a block parameter or another function). Enforced in
part (vet --cost warning XO0915 within one block; the compiler keeps it alive within one statement since
decision 0460). Source: gotcha raw-address-outlives-bytes.

## 7. Concurrency

7.1 **Share mutable state only through `Mutex` or `Atomic`**; closures capture by value. Enforced (XO0502).
Source: core 3.3, 5.1.

7.2 **Check and act in one `with`.** Enforced (checker warning XO0804, a split critical section). Source: core
5.4.

7.3 **Start every task in a `scope` or a `task.Group` that does not escape it; use `os.tasks` for work that
lives as long as the program.** Enforced (XO0801, XO0802, XO0803). Source: core 7.1.

7.4 **`defer` belongs to its block, not the function, and runs shielded from cancellation: put cleanup
there.** Advisory (semantics that differ from Go). Source: core 3.5; gotchas defer-differs-from-go,
canceled-sleep-unwinds.

7.5 **Use `clock.sleep_or_cancel` when you must return your own Canceled error**; a `!is_canceled()` loop around
`sleep` never sees the cancel. Advisory. Source: core 7.2.

7.6 **A busy loop must reach a scheduling point** (a channel operation, a sleep, an effect call). Advisory.
Source: gotcha busy-loop-starves-deterministic-scheduler.

7.7 **Under contention use one `Atomic.add`, not your own compare-and-swap loop.** Advisory. Source: gotcha
atomic-cas-loop-contention (1.5 to 2.1x Go's CPU).

## 8. Tests

8.1 **Where tests go** (the owner's rule, 2026-10-13, decision 0209): inline `test` blocks next to the code
for small self-contained functions, where the test reads as an example; `_test.xo` files for substantial
suites and shared helpers. A helper whose signature takes `testing` types belongs in a `_test.xo` file: there
`testing` is predeclared (core 9.2), while in an ordinary file it would need `use std/testing`, which every
build would then load. Advisory for programs; in this repository std/ and compiler/ use `_test.xo` only
(decision 0262, a repotest check). Sources: decisions 0209, 0262; core 9.2.

```xo
fn check_files(os: testing.TestOs, dir: Str, files: List[(Str, Str)]) -> Str ! FsErr {
```

(compiler/check/check_test.xo:41)

8.2 **Builds skip test code** (`_test.xo` files, decision 0348; inline test blocks, decision 0389), so
production code cannot call a test helper and a broken test fails only `xo check` and `xo test`. Enforced.
Source: core 9.2.

8.3 **`opt?` fails a test directly in a test body; in helpers write `.or_err(...)?`.** Enforced (XO0204,
XO0205 in helpers). Source: core 9.2; decision 0347.

8.4 **Test against the fakes on `os: TestOs`**: FakeNet (`handle`, `fail_next`, `listen(":0")`), MemFs
(`fail_next`, `set_disk_usage`) and the virtual clock, which advances when every task is blocked. Real
capabilities need `uses net` and `--real`, and such a test must check something only the real one shows
(`os.fs` is a MemFs). Advisory. Source: core 9.2; std 8; gotcha testos-fs-is-memfs.

```xo
os.fs.fail_next(FsOp.Rename, "dst/a.txt", FsErr.Io(path: "dst/a.txt", msg: "disk full"))
```

8.5 **Give functions contracts where they state the truth**: `xo test` fuzzes functions with `requires` and
`ensures` and shrinks failures; property tests (`test "..." for x: T`) do the same. Contract clauses must be
pure. Advisory. Source: core 9.1, 9.2.

8.6 **In timing tests, prepare every input first and time only the operation.** Advisory. Source: gotcha
timing-test-inline-inputs.

## 9. Layout, comments and formatting

9.1 **Run `xo fmt`**: two-space indentation, one statement, member or arm per line, runs of blank lines
collapsed, a list broken right after its opening bracket laid out one element per line with a trailing comma.
Enforced (`xo build --ci` fails on unformatted files). Source: core 10.5; toolchain.md. To keep a comment next
to a list element, break after the bracket (gotcha fmt-comments-in-joined-lists-move); fmt does not align
tables (gotcha xo-fmt-const-table-one-per-line).

9.2 **Break lines after a binary operator; start a continuation with `.` to split a method chain; one
statement per line.** Enforced (XO0178 with a fix, XO0161, XO0162, XO0175). Source: core 1.3.

9.3 **`use` lines come before the first declaration and name absolute paths.** Enforced (XO0173, XO0158). No
order is required among them. Source: core 4.1.

9.4 **Comment with `//`; document every `pub` item with `///` (Markdown).** House style: a file begins with a
`//` header naming what it implements and its spec section or decision. Advisory. Source: core 1.2; for
example compiler/cli/core/format/format.xo:1-12, std/bufio.xo:1-6.

9.5 **A module is one directory; a one-file module is `NAME.xo` with `NAME_test.xo` beside it.** Advisory.
Source: core 1.1; decision 0262.

9.6 **Run `xo vet` (and `xo vet --cost` for hot code)**; its warnings and notes are listed in core 10.6.
`dbg` left in code fails `--ci` (XO0901).

## Open points for the owner

- 4.5: whether to recommend StrBuilder at all; the evidence on current main favors in-place `+=`, and an A/B
  would settle it.
- 5.2 and 5.3 are enforced for std only; whether `xo vet` should report them for every program by default.
- 8.1: core 9.2 calls test placement "a note, not a rule"; this file states it as a convention. Whether
  core 9.2 should point here.
- Rules left out as std or runtime internal (cold-path splitting and the inline budget; bounded `str_find`):
  they belong in the perf guide, not here.
