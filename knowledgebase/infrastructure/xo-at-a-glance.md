# Xo at a glance: what already exists

Read this before reporting a language or std feature as missing. Each line
names the spec section (spec/core.md, or std.md for std); the spec is the
source of truth, and `xo doc NAME` shows a type's methods (prelude types
included). Two features were reported missing on 2026-10-21 that Xo had
(match guards, `Set`); check here, then the spec, then ask.

Last verified: 2026-10-24 (spec/core.md and spec/std.md at main).

## Types (core 2)

- Integers `Int` (64-bit), `Int8`..`Int64`, `UInt8`..`UInt64`; overflow
  faults. `Float` (64-bit) and `Float32`. `Rune`, `Bool`, `()`, `Never`.
- `Str` (always valid UTF-8) and `Bytes`; `Duration` and `Time`.
- Collections, all immutable persistent values: `List[T]`, `Map[K, V]`
  and `Set[T]` (both insertion ordered); literals `[1, 2]`, `{"a": 1}`,
  `#{1, 2}`, and empty `[]`, `{}` (a Map), `#{}` (a Set) (core 1.6);
  `${xs}`, `${m}`, `${s}` print their Debug text when the elements have Display (decision 0525);
  `Buf[T]` is the mutable, move-only buffer (5.3). `ByteBuffer` is
  Go's bytes.Buffer for parsers: write at the back, `next`, `peek`,
  `read_until` (resumes its search), `discard`, `find` at the front
  (decision 0480, std.md 2.7).
- `T?` (Option, 2.5) and `Result[T, E]` written `T ! E` (2.6); `?`
  propagates, `??` supplies a default and since 0321 an early exit
  (`x ?? return v`, `?? break`, `?? continue`), and on a Result names
  the error: `r ?? e => return Err(Wrap(e))`, `r ?? _ => continue`.
- Tuples `(A, B)` with `t.0`, `t.1`; structs with field defaults; enums
  (sum types) with payloads; valued enums `enum E: Str { A = "a" }` with
  `value()`, `E.from(v)`, `E.all()` (0269) and an optional catch-all last
  variant `Unknown(value: Str)` that keeps unlisted values (0493);
  interfaces; generics (2.7); type aliases and
  newtypes (2.8); `Secret[T]` (redacted everywhere).
- Derives (2.9): `derive Debug, Eq, Hash, Ord, Json, SqlRow, Xml` (one
  table, compiler/check/derivables.xo); wire names `as "x"` plus per-format
  `json "..."`, `sql "..."`, `xml "..."` (0315); enum variant wire names
  `Say as "say"` and `derive Json(case: snake)` on an enum, tagged enums
  `derive Json(tag: "type"[, content: "data"])`, and a type's own form
  `derive Json(via: Str, to: f, from: g)` on a struct, enum or newtype
  (0478).

## Statements and expressions (core 3)

- `let`, `var`, `const` (including `embed("path")`, 0304); `let` and
  `var` take irrefutable patterns, `var (a, b) = f()` binding every name
  mutable (0384).
- A top-level `const` may call effect-free functions: computed once, at
  its first read, shared by every task; a fault in it is raised at every
  read (0332, XO0415, XO0416, XO-F009).
- Functions with default parameter values (3.2); closures (3.3, they
  capture by value at creation); methods and `var` receivers (3.4).
- `if`/`else` and `match` are expressions; `match` is exhaustive, takes
  ranges (`'a'..='z'`, `200..=299`), alternatives (`429 | 503`) and
  **guards** (`Pat if cond =>`); `x is Pattern`; `if let`, `while let`;
  `for x in v` over a type with `next() -> T?`, `for x? in v` when it can
  fail (0494).
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
- Shared state: `Mutex[T]` (`with`, `with_read`, `reset`; an `Err` from
  the closure undoes its writes, 5.4, and `XO0540` warns when a write is
  followed by a path returning `Err`; a fault poisons the mutex, `XO-F010`,
  until `reset`, decision 0482) and `Atomic[T]`.
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

http (server, router, client with keep-alive, HTTP/2 and h2c with
`with_protocols`/`with_h2_config` (0434), trailers and full duplex
streams (0433), mutual TLS (`with_client_auth`, `Client.with_cert`),
streaming bodies, multipart, ServeContent, gzip), tls, websocket, json (derive, Value,
streaming decoder), xml (tokens, tree, derive), sql (Postgres driver),
csv, toml, yaml (derive Yaml, decode/encode, Node with comments, to_json; a port of yaml.v3, 0512), regexp, time, url, netip, udp, mime, mail, smtp, multipart,
template, embed, compress, big, base64/base32, binary, uuid, path, png (decode and encode, 0466), jpeg (decode, 0467), cbor (0468),
crypto, archive, log, task, testing, cli, term, go (`use go` bindings).
`time` layouts are named fields (`"{year}-{month}-{day}"`), checked at
compile time when constant; `time.layout(text)` gives a `Layout` parsed
once (`const ISO: Layout = layout(...)`, 0332).
Prelude collections include `sort`, `sort_by`, `group_by`,
`fold`, `contains`, `indexed` (List and Bytes), `to_set`, `reverse`, `join`,
`split`; Bytes has `indexed`, `find`, `find_any`, `contains_any`,
`contains_rune`, `find_where`, `rfind_where`, `contains_where`, `split_after`,
`split_where`, `cut`, `cut_prefix`, `cut_suffix`, `trim_where`,
`trim_start_where`, `trim_end_where` and `map_runes`; `xo doc List`
lists the rest.

## Tools (spec 10, agent-tools.md)

`xo def`, `refs`, `type`, `doc`, `symbols`, `edit rename`, `vet`, `fmt`,
`test`, `prof`, `dbg`, `scripts/xocheck`, `scripts/identity`,
`xo dev dump --fn/--stage`.
- `xo vet --cost` (0327): notes on costs the source does not show:
  `XO0907` a loop copying a map's entries first, `XO0908` an element
  counted up and down each iteration (including callback walks such as
  `xs.any(fn ...)`), `XO0909` a list update that copies the list each
  iteration, `XO0910` a value over 8 words copied into a closure or an
  Option in a loop, `XO0911` a view of a parameter's text or bytes (a
  slice, trim, cut, a piece of split, lines or split_where, or a
  trim*_where made in a loop) stored in a field or a returned collection,
  which keeps the whole input alive (`clone()` it). `XO0915` (a warning,
  std's own code: `--cost --cost-std`) a raw address (`raw.data`,
  `raw.bytes_addr`, `rt.obj_addr`) used after its value was released in
  the same block (gotchas/raw-address-outlives-bytes.md). `XO0916` (a
  warning, `xo vet`) `time.format` or `time.parse` with a compile time
  string layout: use a `const Layout` (decision 0464).
- `xo debug PROG`: xo's own debugger on a live native program, macOS
  arm64 (decision 0253 stages 2 to 4; spec 11.5): break, run,
  continue, step, next, finish, locals, print, backtrace, frame,
  tasks, task N [bt] (the runtime's task tree, as xo dump), threads;
  `-p PID` attaches to a running (hung) process; `--batch -c CMD`,
  `--json`, and `--dap` for editors (0504).
- `xo prof FILE|URL`: a URL is a running server's std/debug
  `/debug/profile?seconds=N` or `/debug/heap` (0325), fetched then
  summarized like a file; `/debug/sched` is printed.
- Targets: `xo build --targets=wasip1/wasm` (a preview 1 module) and
  `--targets=wasip2/wasm` (core 10, decision 0447): a `wasi:http`
  component for a program that calls `http.serve_component` (served by
  `wasmtime serve`, Spin), else a `wasi:cli/command` component (`wasmtime
  run`) with arguments, environment, stdin, output, files
  (`wasi:filesystem`, preopens `.` and `/`) and TCP (`wasi:sockets`:
  listen, dial, lookup_host, deadlines, Net.only; `wasmtime run -S
  inherit-network=y -S allow-ip-name-lookup=y` when it imports them);
  processes, and files or sockets in a `wasi:http` component, are XO1515;
  `http.Server` serves HTTP/1.1 and HTTP/2 (h2c, and over TLS by ALPN)
  in a command (the native server's code, decision 0447 R2a, R2d, R2f;
  no idle parking); `http.Client`, std/tls's client and std/websocket
  work in a command over sockets, https and HTTP/2 included (decision
  0447 R2b, R2e, R2f, R2g; idle connections pooled as natively; no system
  roots); std/udp and std/dns's records work in a command too
  (decision 0447 R2h, R2i); `os.proc.request_stop()` starts the
  shutdown on wasm (R2j: stop_requested, stopping, the grace period).
- `--targets=wasip3/wasm` (decision 0447 stage 5 steps 1 and 2,
  2026-11-03): a program that calls `http.serve_component` as a WASI 0.3
  component (async `wasi:http/handler@0.3.0` with a callback; wasmtime
  serve 50, Spin 4): streamed request and response bodies and trailers,
  `http.Client` through `wasi:http/client`'s send, and several requests
  at once per instance (step 3); commands and sockets at 0.3 are not yet
  (XO1515).
