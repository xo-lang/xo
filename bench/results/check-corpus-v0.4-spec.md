# Type checker over the v0.3 paper corpus under spec 0.4 (2026-10-07)

Checker: `xo check` (P1, `internal/check`) brought in line with
`spec/core.md` and `spec/std.md` draft 0.4 (decisions 0028 to 0035).
Corpus: every module under `bench/paper/v0.3/` (tasks 01 to 08, 13 modules,
written blind against spec 0.3 by other agents). The results under spec 0.3
are in [check-corpus-v0.3.md](check-corpus-v0.3.md). Reproduce with:

```
go run ./cmd/xo check bench/paper/v0.3
go test ./internal/check -run TestCorpusV03 -v                 # same, as data
go test ./internal/check -run TestCorpusV03 -corpus-report     # rewrites the generated section below
go test ./internal/check -run TestCorpusMutations -mutations -v
```

The corpus test never fails on diagnostics; they are data.

## Summary

| Category | Diagnostics |
|---|---|
| (a) real violation of spec 0.4 in the solution | 4 (3 XO0206, 1 XO0601) |
| (b) checker bug | 0 |
| (c) spec gap | 0 diagnostics; the readings the checker had to choose for 0.4 are listed below |

The 3 XO0206 hits are new and expected: the corpus was written against 0.3,
where `let _ = <Result>` was the documented way to discard an error. Under
0.4 (decision 0031) each must become `.ignore_err("reason")`. The XO0601 is
the same `FsOp` hit as under 0.3, still a real violation (std.md 8.4 now says
std enums are named with their module, `testing.FsOp.Rename`).

No other 0.4 rule fires on the corpus: the widened XO0520 (decision 0034)
finds no `.or_err(...)?` copy that is mutated without a write back and no
local mutated after being stored, and nothing in the corpus uses task groups
or effect elision yet (it was written before they existed).

## Generated counts

<!-- generated:begin -->
Modules checked: 13. Diagnostics: 4.

| Code | Count | Meaning |
|---|---|---|
| XO0206 | 3 | `let _ =` discards a Result; use `.ignore_err("reason")` (core 2.6) |
| XO0601 | 1 | name does not resolve (core 4.3) |

| File | Diagnostics by code |
|---|---|
| `bench/paper/v0.3/04-file-sync/sync.xo` | XO0206 x1 |
| `bench/paper/v0.3/04-file-sync/sync_test.xo` | XO0601 x1 |
| `bench/paper/v0.3/06-job-queue/worker.xo` | XO0206 x1 |
| `bench/paper/v0.3/08-notes-service/store/store.xo` | XO0206 x1 |

| Diagnostic |
|---|
| `bench/paper/v0.3/04-file-sync/sync.xo:145:7` XO0206: `let _ =` discards a Result, so its error is ignored without a reason |
| `bench/paper/v0.3/04-file-sync/sync_test.xo:84:19` XO0601: name `FsOp` not found |
| `bench/paper/v0.3/06-job-queue/worker.xo:76:7` XO0206: `let _ =` discards a Result, so its error is ignored without a reason |
| `bench/paper/v0.3/08-notes-service/store/store.xo:91:5` XO0206: `let _ =` discards a Result, so its error is ignored without a reason |
<!-- generated:end -->

## Judgment per diagnostic

### XO0206 (`let _ =` discards a Result): 3, all (a)

| Site | Code | Judgment |
|---|---|---|
| `04-file-sync/sync.xo:145:7` | `let _ = dst.remove(tmp)` in a `defer` that cleans up a temp file | (a) real violation under 0.4: a best effort cleanup, exactly the core 2.6 example; write `dst.remove(tmp).ignore_err("best effort temp file cleanup")` |
| `06-job-queue/worker.xo:76:7` | `let _ = q.nack(l.job.id, l.token, "canceled")` in a `defer` after cancellation | (a) real violation under 0.4: a deliberate discard (the lease expires anyway); it needs a reason |
| `08-notes-service/store/store.xo:91:5` | `let _ = s.fs.remove(tmp)` after a failed write, before `res?` | (a) real violation under 0.4: best effort cleanup whose own error is less useful than `res`; it needs a reason |

Each diagnostic carries the fix `discard with a reason`, which appends
`.ignore_err("TODO: reason")` (parenthesizing the value when it is not a
postfix expression).

### XO0601 (name does not resolve): 1, (a)

`04-file-sync/sync_test.xo:84:19`: `FsOp.Rename` must be `testing.FsOp.Rename`
or the bare variant `Rename` (the expected type is known). Unchanged from the
0.3 run; the diagnostic has a note and a `qualify as testing.FsOp` fix.

## Mutation testing (is the clean part really checked?)

| Mutation | Sites | Detected | Under 0.3 | Not detected, and why |
|---|---|---|---|---|
| remove a postfix `?` | 220 | 218 | 217 | 2 legal: the value is then returned as is from a `-> T ! E` function. The third 0.3 miss (`let _ = f()?` becoming `let _ = f()`) is now XO0206. |
| delete one match arm | 158 | 146 | 146 | 12 legal: a catch all remains, or the arm had a guard |
| `var` to `let` | 75 | 75 | 75 | 0 |
| delete a numeric conversion | 4 | 4 | 4 | 0 |
| delete a `derive` clause | 15 | 15 | 15 | 0 |
| typo in a member name (every third site) | 233 | 220 | 220 | 13, all in comments or doc text |
| delete a `uses` clause | 10 | 8 | 8 | 2 legal: non pub functions, whose effects are then inferred |

## Readings chosen for spec 0.4

Places where spec 0.4 is silent, contradictory, or unimplementable as
written. Each is implemented as stated and covered by a test in
`internal/check/rules04_test.go` or `internal/parser/parser_test.go`.

1. **`testing.run_main` and effects.** std.md 8.4 types the parameter as
   `main: fn(Os) -> () ! Error`, a pure function type. Every real `main`
   performs effects on its `os`, so passing it would be XO0701. Declared as
   `run_main[fx: Effects](main: fn(Os) -> () ! Error uses fx, os: TestOs,
   args: List[Str] = []) -> Int uses fx` (test blocks may perform any effect).
2. **Elided effect arguments in a result type.** Core 6.2 says an omitted
   effect argument in a function signature becomes an implicit effect
   variable of the function. In a result type that variable is chosen by the
   caller, so a body that builds the value (`fn new_service(c: http.Client)
   -> Service { Service{cache: new_cache(...)} }`) could never satisfy it,
   and even passing a parameter through would fail (two different
   variables). Reading: omitted effect arguments in parameter and receiver
   types are implicit effect variables; in the result and error types they
   are inferred from the body, like a local binding.
3. **Implicit effect variables are performable.** A function cannot name an
   implicit effect variable in `uses`, so a pub function given
   `c: Cache[Int, Item]` could never call `c.get`. Reading: implicit effect
   variables from elision (parameters, and receivers written without the
   effect argument) are part of the function's declared effects. An effect
   variable the code names (`fn (c: Cache[K, V, fx])`) must still be
   declared with `uses fx`.
4. **Which arguments may be omitted.** Reading: either every effect argument
   is written, or every one is omitted; the written arguments then go to the
   type parameters in order.
5. **Group task failures (XO0801).** Core 7.1 says a failing group task fails
   the creating scope, and the error must convert. The creating scope is
   only visible when the group is a local bound to `s.group()`, or `os.tasks`
   of `main` or of a test; then the rule is that of `s.spawn` (convert to
   that function's error type, unless the handle is bound and awaited). For
   a group from a field or a parameter the scope is not visible, so the
   task must not fail unless its handle is bound and awaited, the rule P1
   already used for scope handles received from elsewhere.
6. **Which values escape with a group (XO0802).** `os.tasks` is also a
   `task.Group` and may be stored anywhere. Reading: XO0802 applies to values
   derived from `s.group()` of the scope (directly, through bindings, or
   captured by a closure) that become the scope's value, are returned, are
   assigned or pushed into a binding declared outside the scope, or are sent
   on a channel. Struct fields, enum variant fields, tuples, and type
   arguments are searched at any depth for the `Group`.
7. **Result type on the next line.** std.md 7 writes `map_concurrent` with
   `-> List[Result[R, E]] uses fx` on its own line, which core 1.3's list of
   continuation lines does not include. Reading: a line that starts with `->`
   continues a function declaration (a line cannot otherwise start with
   `->`). A spec inconsistency between std.md 7 and core 1.3.
8. **Names in an `is` pattern.** Core 3.5 says `x is Pattern` binds no names
   but not what happens if the pattern has one. Reading: a parse error
   (XO0165) with a fix to `_`. A struct pattern after `is` in an `if`,
   `while`, `match`, or `for` head must be parenthesized, like a struct
   literal (core 1.4), and gets XO0163 with the parenthesize fix. Core 1.4's
   list of contextual words does not mention `is` (3.5 does).
9. **Bitwise compound assignments.** The lexer has `&= |= ^= <<= >>=`, but
   core 3.5 and 15 (AssignOp) list only `= += -= *= /= %=`. The parser now
   rejects the others (XO0160).
10. **Unused local `const`.** Core 3.1 makes unused local bindings XO0110;
    a local `const` is treated as a binding. Its UPPER_SNAKE name cannot take
    a `_` prefix, so an unused local constant must be removed.
11. **"Stored into a collection" (XO0520).** Reading: the value is passed to
    `push`, `put`, `insert`, `set`, `add`, or `update` on a `List`, `Map`, or
    `Set`, or assigned with `m[k] = v`. A whole reassignment `v = ...` starts
    over. A store later in an enclosing loop counts as storing again (one
    snapshot per iteration is accepted), as does a store in a `defer`. Reads
    after the mutation do not matter, as the spec says.
12. **`let _` in a nested pattern.** XO0206 covers `let _ = <Result>` and
    `let _: T = <Result>`; a `_` inside a tuple pattern
    (`let (_, n) = (f(), 2)`) is not covered by core 2.6 and is accepted.
13. **Interface methods with a body.** Core 15 (FnSig) has no body; the
    parser now reports one (XO0160) instead of accepting it silently.
14. **Effect arguments at a call.** `f[Int, net](x)` takes type arguments,
    but `f[net](x)` is read as an index followed by a call (as `fns[i](x)`
    must be); effect arguments of functions are inferred, so this costs
    nothing in practice.
