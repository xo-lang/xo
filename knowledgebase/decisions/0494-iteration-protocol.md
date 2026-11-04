# 0494: `for` over a user type with a `next()` method

Status: Proposed (xo-lang-4, 2026-11-04, for the AWS SDK agent through the
lead). Nothing is built before the owner accepts.

## Context

`for x in v` works over List, Buf, Set, Map, Chan, Range and Bytes (core
3.5). A user type that produces values one at a time cannot be iterated:
std's own readers (`multipart.PartReader.next() -> StreamPart? ! E`,
`sql.Rows.next() -> Row? ! SqlErr`, `xml.Reader.next() -> Token? !
XmlErr`) are written as

```
while let Some(row) = rows.next()? { ... }
```

and the AWS SDK's paginators (aws docs/decisions/0003, 0004, 0006 item
7) follow Go's `has_more_pages()` / `next_page()` pair. `while let`
(decision 0137) already does the work; what is missing is that the
type's natural loop is not `for`, so agents write it three ways, and a
paginator gets two methods where one would do.

Almost every such source can fail on each step (a network page, a
database row, a parse). The design question is how that error leaves the
loop.

## Decision

1. A value whose type has a method `next()` taking no arguments and
   returning `T?` may be iterated: `for x in v { body }` is

   ```
   while let Some(x) = v.next() { body }
   ```

   with v evaluated once (a temporary when it is not a place). v must be
   a `var` place when `next` takes `var self` (the ordinary rule for a
   `var` method call). After the loop, v holds the state the last call
   left. Patterns, `bound N`, `break` and `continue` work as in any
   `for`. A type with both `next()` and a builtin iteration (none today)
   uses the builtin one.

2. When `next()` returns `T? ! E`, the loop must say where the error
   goes, with `?` after the loop's pattern:

   ```
   for page? in pages { body }
   ```

   is `while let Some(page) = pages.next()? { body }`: an error from
   any step propagates as `?` does at that point (converted to the
   function's error type, with its trace frame, core 2.6). `for page in
   pages` without the `?` is a compile error (a new code) whose fix adds
   it, so a failing source is never iterated with its errors dropped. To
   handle the error in the loop instead, write the `loop`/`match` by
   hand (below).

3. Nothing else is added: no interface to implement, no `iter()`
   adapters, no generators. A type is iterable because it has the
   method, structurally, as interfaces are satisfied (core 2.4). std
   documents the prelude interfaces `Next[T]` (`fn next(var self) ->
   T?`) and `TryNext[T, E]` for use as generic constraints.

Worked example: S3 ListObjectsV2 in the SDK.

```
pub struct ListObjectsV2Paginator {
  client: Client
  input: ListObjectsV2Input
  token: Str?
  done: Bool
}

pub fn (var p: ListObjectsV2Paginator) next() -> ListObjectsV2Output? ! Err uses net {
  if p.done { return None }
  var req = p.input
  req.continuation_token = p.token
  let out = p.client.list_objects_v2(req)?
  let next = out.next_continuation_token
  // Go's paginator stops when the token is absent or repeats
  p.done = next == None || next == p.token
  p.token = next
  Some(out)
}
```

and the caller:

```
var pages = s3.ListObjectsV2Paginator.new(client, s3.ListObjectsV2Input{bucket: "logs"})
for page? in pages bound 10000 {
  for obj in page.contents {
    os.stdio.println("${obj.key ?? ""} ${obj.size ?? 0}")
  }
}
```

A throttled page (`SlowDown` after the SDK's retries) leaves the
function through the `?` with the error and its frame. A caller that
wants to keep the pages it has and stop:

```
loop {
  match pages.next() {
    Ok(Some(page)) => keep.push(page)
    Ok(None) => break
    Err(e) => { log.warn("listing stopped: ${e}"); break }
  }
}
```

std's sources become `for` loops with no API change:
`for row? in rows`, `for part? in reader`, `for tok? in xml_reader`.

## Consequences

- The SDK's paginators get one method, `next()`, and the README example
  drops `has_more_pages`. Waiters are unaffected.
- Every backend lowers the loop as the `while let` it is; no runtime or
  codegen change, so the work is in the checker (finding `next`, its
  result type, the `?` rule) and the formatter.
- `for x? in` is new syntax: the pattern grammar (core 15), 3.5's `for`
  rules, the new code in 10's table, xo-at-a-glance.md, and `xo fix`'s
  rewrite of `while let Some(x) = v.next()?` to `for x? in v` (offered,
  not applied by default).
- A `Str`, still not iterable (core 3.5), stays so: it has no `next()`.

## Alternatives

- Bind each step's result (Rust's `for r in reader.lines()` with
  `r: Result<..>`): the error is visible, but each body starts with
  `let page = r?` and Xo's rule that a Result is used (XO0201) makes
  forgetting it an error anyway; the `?` in the header says the same in
  one place.
- `?` after the iterated expression (`for page in pages? { }`): reads
  as "each step may fail", but `pages?` is already an expression when
  pages is an Option or a Result, so `for x in maybe_list?` would change
  meaning by the element type. The pattern position has no `?` today, so
  `for page? in` is unambiguous.
- Swift's `for try await x in seq`: `try` is one of the foreign forms Xo
  rejects with a hint (core 10, `try f()`); `?` is Xo's word for it.
- An `Iterator` interface with adapters (`map`, `filter`, `take`, as
  Rust's): a much larger surface and closures in hot loops; not needed
  for the asks (paginators, readers). A later decision can add adapters
  over `Next[T]`.
- Generators (`yield`): a compiler transformation into a state machine;
  far more than the asks need.
- Keep `has_more_pages()`/`next_page()` (Go): works today; two methods,
  and a caller can call `next_page()` without checking.
