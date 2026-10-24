# Xo at a glance: what already exists

Read this before reporting a language or std feature as missing. Each line
names the spec section (spec/core.md, or std.md for std); the spec is the
source of truth, and `xo doc NAME` shows a type's methods (prelude types
included). Two features were reported missing on 2026-10-21 that Xo had
(match guards, `Set`); check here, then the spec, then ask.

Last verified: 2026-10-22 (spec/core.md and spec/std.md at main).

## Types (core 2)

- Integers `Int` (64-bit), `Int8`..`Int64`, `UInt8`..`UInt64`; overflow
  faults. `Float` (64-bit) and `Float32`. `Rune`, `Bool`, `()`, `Never`.
- `Str` (always valid UTF-8) and `Bytes`; `Duration` and `Time`.
- Collections, all immutable persistent values: `List[T]`, `Map[K, V]`
  and `Set[T]` (both insertion ordered); literals `[1, 2]`, `{"a": 1}`,
  `#{1, 2}`, and empty `[]`, `{}` (a Map), `#{}` (a Set) (core 1.6);
  `Buf[T]` is the mutable, move-only buffer (5.3).
- `T?` (Option, 2.5) and `Result[T, E]` written `T ! E` (2.6); `?`
  propagates, `??` supplies a default and since 0321 an early exit
  (`x ?? return v`, `?? break`, `?? continue`), and on a Result names
  the error: `r ?? e => return Err(Wrap(e))`, `r ?? _ => continue`.
- Tuples `(A, B)` with `t.0`, `t.1`; structs with field defaults; enums
  (sum types) with payloads; interfaces; generics (2.7); type aliases and
  newtypes (2.8); `Secret[T]` (redacted everywhere).
- Derives (2.9): `derive Debug, Eq, Hash, Ord, Json, SqlRow, Xml` (one
  table, compiler/check/derivables.xo); wire names `as "x"` plus per-format
  `json "..."`, `sql "..."`, `xml "..."` (0315).

## Statements and expressions (core 3)

- `let`, `var`, `const` (including `embed("path")`, 0304).
- A top-level `const` may call effect-free functions: computed once, at
  its first read, shared by every task; a fault in it is raised at every
  read (0332, XO0415, XO0416, XO-F009).
- Functions with default parameter values (3.2); closures (3.3, they
  capture by value at creation); methods and `var` receivers (3.4).
- `if`/`else` and `match` are expressions; `match` is exhaustive, takes
  ranges (`'a'..='z'`, `200..=299`), alternatives (`429 | 503`) and
  **guards** (`Pat if cond =>`); `x is Pattern`; `if let`, `while let`.
- Loops: `for x in xs`, `for (k, v) in m`, `for x in ch`, ranges `0..n`
  and `0..=n`, `xs.indexed()`, `while`, `while { }`, labeled loops with
  `break label`, and `bound N` loop contracts.
- `defer stmt` / `defer { ... }` at block exit (3.5).
- Strings: interpolation `"${x}"`, Debug interpolation `"${x:?}"`, raw
  multi-line strings `r"""..."""` and `r#"""..."""#` (0320).
- Slices of `Str` and `Bytes` are O(1) views on native targets (0330):
  `s[a..b]`, `split`, `lines`, `trim*`, `trim_prefix/suffix`,
  `split_where` and `trim*_where` share the
  buffer, so a small piece keeps a large source alive (copy with
  `s.clone()`, also on Bytes; no automatic compaction, design note in
  bench/results/views-retention-2026-10-22.md); a Str or
  Bytes holds at most 4294967295 bytes (concat, append, push fault with
  "Str or Bytes longer than 4 GiB", `repeat` with its own message); a C ABI
  export may not return a Str. wasm and the Go backend copy.
- Indexing (3.6): `xs[i]`, `xs.get(i)` and slice bounds take any integer
  type (`UInt8`, `UInt32`, ...), widened to `Int` before the bounds check
  (0331); `Map` cannot be read by index (`m.get(k)`).
- Holes (3.7).
- Evaluation order (5.2): arguments left to right, then the receiver
  when it is a variable or field path, `var` or not (0142, 0326):
  `t.size(t.add(v))` sees the item `add` pushed; an assignment computes
  its value before it reads the binding it stores into (0140).

## Values, memory, concurrency (core 5, 7)

- Value semantics with copy-on-write; checked moves (XO0510);
  `var` parameters make mutation visible in signatures.
- Shared state: `Mutex[T]` (`with`, `with_read`; an `Err` from the
  closure undoes its writes, 5.4, and `XO0540` warns when a write is
  followed by a path returning `Err`) and `Atomic[T]`.
- Structured concurrency: `scope`, tasks, cancellation (7.1-7.2);
  channels `Chan[T]` with `select` (7.3); deterministic scheduling and
  `xo test --explore=N` (7.4); signals and graceful shutdown (7.5).
- Native preemption (signals, 0318; LLVM too) and `--race` (0273): arm64
  and amd64 on darwin and linux, LLVM, and windows/amd64 (ThreadSanitizer's
  Go flavor, untested with threads until a Windows CI run).

## Effects, faults, contracts, tests (core 6, 8, 9)

- Effects in signatures (`uses (...)`), capability values (`Os`, `Fs`,
  `Net`, `Proc`, `Env`, `Clock`, `Rand`, `Stdio`), user values standing
  in for any of them (0307), fakes for tests (6.4).
- Faults are separate from errors (8); `requires`/`ensures` contracts
  (9.1); `test` blocks, property and concurrency tests (9.2).

## Std (spec/std.md)

http (server, router, client with keep-alive, HTTP/2, streaming bodies,
multipart, ServeContent, gzip), tls, websocket, json (derive, Value,
streaming decoder), xml (tokens, tree, derive), sql (Postgres driver),
csv, toml, regexp, time, url, netip, mime, mail, smtp, multipart,
template, embed, compress, big, base64/base32, binary, uuid, path,
crypto, archive, log, task, testing, cli, term, go (`use go` bindings).
`time` layouts are named fields (`"{year}-{month}-{day}"`), checked at
compile time when constant; `time.layout(text)` gives a `Layout` parsed
once (`const ISO: Layout = layout(...)`, 0332).
Prelude collections include `sort`, `sort_by`, `group_by`,
`fold`, `contains`, `indexed` (List and Bytes), `to_set`, `reverse`, `join`,
`split`; Bytes has `indexed` and `find`; `xo doc List` lists the rest.

## Tools (spec 10, agent-tools.md)

`xo def`, `refs`, `type`, `doc`, `symbols`, `edit rename`, `vet`, `fmt`,
`test`, `prof`, `dbg`, `scripts/xocheck`, `scripts/identity`,
`xo dev dump --fn/--stage`.
- `xo vet --cost` (0327): notes on costs the source does not show:
  `XO0907` a loop copying a map's entries first, `XO0908` an element
  counted up and down each iteration (including callback walks such as
  `xs.any(fn ...)`), `XO0909` a list update that copies the list each
  iteration, `XO0910` a value over 8 words copied into a closure or an
  Option in a loop, `XO0911` a view of a parameter's text (a slice, trim,
  a piece of split or split_where, or a trim*_where made in a loop) stored
  in a field or a
  returned collection, which keeps the whole input alive (`clone()` it).
- `xo prof FILE|URL`: a URL is a running server's std/debug
  `/debug/profile?seconds=N` or `/debug/heap` (0325), fetched then
  summarized like a file; `/debug/sched` is printed.
