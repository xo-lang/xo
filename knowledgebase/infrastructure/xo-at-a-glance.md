# Xo at a glance: what already exists

Read this before reporting a language or std feature as missing. Each line
names the spec section (spec/core.md, or std.md for std); the spec is the
source of truth, and `xo doc NAME` shows a type's methods (prelude types
included). Two features were reported missing on 2026-10-21 that Xo had
(match guards, `Set`); check here, then the spec, then ask.

Last verified: 2026-10-21 (spec/core.md and spec/std.md at main).

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
  (`x ?? return v`, `?? break`, `?? continue`).
- Tuples `(A, B)` with `t.0`, `t.1`; structs with field defaults; enums
  (sum types) with payloads; interfaces; generics (2.7); type aliases and
  newtypes (2.8); `Secret[T]` (redacted everywhere).
- Derives (2.9): `derive Debug, Eq, Hash, Ord, Json, SqlRow, Xml` (one
  table, compiler/check/derivables.xo); wire names `as "x"` plus per-format
  `json "..."`, `sql "..."`, `xml "..."` (0315).

## Statements and expressions (core 3)

- `let`, `var`, `const` (including `embed("path")`, 0304).
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
- Indexing (3.6): `xs[i]`, `xs.get(i)` and slice bounds take any integer
  type (`UInt8`, `UInt32`, ...), widened to `Int` before the bounds check
  (0331); `Map` cannot be read by index (`m.get(k)`).
- Holes (3.7).

## Values, memory, concurrency (core 5, 7)

- Value semantics with copy-on-write; checked moves (XO0510);
  `var` parameters make mutation visible in signatures.
- Shared state: `Mutex[T]` (`with`, `with_read`; an `Err` from the
  closure undoes its writes, 5.4) and `Atomic[T]`.
- Structured concurrency: `scope`, tasks, cancellation (7.1-7.2);
  channels `Chan[T]` with `select` (7.3); deterministic scheduling and
  `xo test --explore=N` (7.4); signals and graceful shutdown (7.5).
- Native preemption (signals, 0318) and `--race` (darwin arm64, linux
  arm64, LLVM; 0273).

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
Prelude collections include `sort`, `sort_by`, `group_by`,
`fold`, `contains`, `indexed` (List and Bytes), `to_set`, `reverse`, `join`,
`split`; Bytes has `indexed` and `find`; `xo doc List` lists the rest.

## Tools (spec 10, agent-tools.md)

`xo def`, `refs`, `type`, `doc`, `symbols`, `edit rename`, `vet`
(including `--cost`, in progress), `fmt`, `test`, `prof`, `dbg`,
`scripts/xocheck`, `scripts/identity`, `xo dev dump --fn/--stage`.
