# Paper test notes: 04-file-sync (spec v0.2)

Files: `main.xo` (CLI, platform rules, capability attenuation, `main`),
`sync.xo` (pure `plan`, `apply`, `preview`, `copy_file`), `sync_test.xo`.

Design in one paragraph: `main` turns `os.fs` into two attenuated roots
(`src` via `sub(...)?.read_only()`, `dst` via `sub(...)`, or read only in dry
run) and never passes `os.fs` further. `sync` walks both roots, a pure `plan`
turns the two listings into a `List[Action]`, and then either `apply` (real
run) or `preview` (dry run) executes the actions and builds a `Report`.
Platform rules come from a `match target.os` in `main`, but `plan` takes a
`Platform` value so tests can exercise macOS rules on any host.

## 1. Spec gaps

1. **Root of the real `os.fs` (core 6.3, std 3.1).** std says paths are
   relative to the capability root and `..` escapes fail with `Denied`, but
   never says what the root of `main`'s `os.fs` is (cwd? filesystem root?)
   or whether an absolute path like `/home/me/a` is accepted. A CLI taking
   user paths needs this. Guessed that command line paths resolve as the
   user expects (GUESS in `main.xo`).
2. **`Fs.sub` on a missing directory (std 3.1).** "dir must exist" forces a
   dry run against a missing `dst` to have no `Fs` at all. I modeled it as
   `Fs?`. Not a guess, but a spec consequence worth knowing: there is no way
   to get a read only view of a path that does not exist yet.
3. **`walk` output shape (std 3.1).** "recursive, parents first" but silent
   on whether the root `"."` itself is included, and whether `path` is
   `"a/b"` or `"./a/b"`. Guessed: root excluded, no `./` prefix.
4. **MemFs modified times (std 8.3).** Silent on what `modified` a plain
   `write` sets (virtual clock now?) and whether `rename` keeps the temp
   file's mtime. I sidestepped with explicit `set_modified` in the tests and
   in `copy_file` (set on the temp file before rename, assuming rename keeps
   it).
5. **`fail_next` path matching (std 8.3).** For `FsOp.Rename` there are two
   paths; which one matches? And is the path the one seen by the MemFs root
   or by the `sub` capability that made the call? Guessed: target path, as
   seen by the MemFs root (GUESS in `sync_test.xo`).
6. **XO0520 and `defer` (core 3.4, 5.2).** `var committed = false`, set to
   `true` at the end, read only inside a `defer`. If the lost update check
   does not count reads in deferred code, this idiom is rejected. Guessed it
   counts (GUESS in `sync.xo`).
7. **Value of a block that ends in a statement (core 3.2).** "The last
   expression of a block is its value" and "an empty body has type ()", but a
   body ending in an assignment is not covered. I wrote an explicit `()`
   tail (GUESS in `sync.xo`).
8. **Tail `match` arms of different shapes in a `-> T ! E` function (core
   2.6).** Can one arm yield `T` and another `Err(e)` in a tail `match`? The
   return rule is described for "a tail expression or `return x`", not per
   arm. I avoided it with `_ => return Err(...)` in `parse_args`.
9. **`uses` clause punctuation (core 6.2 vs 15).** The grammar says
   `FnClause = "uses" EffectList` (no parens), but the 6.2 example writes
   `uses (clock, fx)` on a function declaration. Parens are only described as
   required in function types. I ended up not writing `uses` at all (all my
   functions are non pub, effects inferred), but I hesitated.
10. **Enum prefix omission inside nested constructors (core 2.3).** "Omitted
    when the expected type is known (in a match arm, an argument, a return,
    or a typed binding)". Is `Some(MakeDir(path: p))` in return position
    "known"? I assumed yes, since `Some`'s argument is an argument whose
    expected type flows from the return type. Same for `Err(UnknownFlag(a))`
    and `Err(NotEmpty(_))` in patterns.
11. **Constructing std structs (std 3.1, 1.5).** `FileInfo` fields are not
    marked `pub` in std. My macOS test builds `FileInfo{...}` literals to test
    `plan` directly. If std fields are read only outside std, that test needs
    MemFs plus `set_case_sensitive(false)` instead.
12. **Names of `std/testing` types in user code (std 8).** `MemFs`, `FsOp`
    live in `std/testing`, which is predeclared as `testing` in test blocks.
    Unclear if helpers outside test blocks (in a `_test.xo` file) can name
    `testing.MemFs`. I avoided naming it: helpers take `Fs`, and `FsOp.Rename`
    is written bare as `Rename` in argument position.
13. **WASI case sensitivity.** The task covers Linux, macOS, Windows only, but
    `match target.os` must be exhaustive. Guessed Linux behavior (GUESS).
14. **`Fs.case_sensitive()` vs task wording.** std offers
    `case_sensitive()` of the underlying filesystem; the task specifies by OS.
    I followed the task (`target.os`) rather than probing; unclear which the
    spec authors consider idiomatic.
15. **`Err("...")` from `main` (core 2.6, std 1).** `fn main ... !` has error
    type `Error`. I return `Err("{n} operation(s) failed")`, relying on `Str`
    implementing `Display` and on implicit interface conversion of the
    constructor argument. core 12.2 does `.or_err("APPDATA not set")?`, which
    suggests it works, but it is never stated that `Str` is `Display`.

## 2. Habits

1. Started to write `if err != nil` style cleanup after each step in
   `copy_file`. Changed to a block scoped `defer` with a `committed` flag.
   Then hesitated because the flag is very Go; considered a
   `match write_and_rename(...) { Err(e) => { cleanup; Err(e) } ... }` helper
   instead. Kept `defer` because it also runs on faults.
2. Wanted `filepath.Dir` / Rust `Path::parent`. There is no path module, and
   no `rfind` on `Str`. Wrote `parent_dir` with `split("/")`, a slice
   `parts[..parts.len() - 1]`, and `join`.
3. Wanted `flag` package style parsing (Go) or `clap` (Rust). Wrote a loop
   with `match a { "--dry-run" => dry_run = true ... }` and finished with a
   list pattern `[src, dst]`, which is nicer than an index check.
4. Started to write `os.Exit(1)` after the summary. std says prefer returning
   `Err` from `main`, so `return Err("...")`. Kept `os.proc.exit(2)` only for
   usage errors, where its `Never` type makes the `match` arm typecheck.
5. Wanted to pass `os.fs` into `sync` and check paths with
   `strings.HasPrefix` like a Go sandbox. Changed to attenuating in `main`
   (`sub(...)?.read_only()`) so the restriction is structural.
6. Wrote `r.failed++` first. No `++`; changed to `r.failed += 1`, then
   pulled it into a `var` receiver method `r.fail(out, msg)`.
7. Wanted `Option<Fs>` as a Rust style `if let Some(d) = dst` everywhere;
   used a `match dst { Some(d) if !opts.dry_run => ..., _ => ... }` with a
   guard instead, which reads better than nested ifs.
8. Reached for `;` to put two statements on one arm line
   (`out.println(..); r.copied += 1`). Not allowed; every two statement arm
   became a block.

## 3. Friction

1. **Lost update check vs deferred cleanup.** The idiomatic "clean up unless
   committed" needs a mutable flag whose only read is in a `defer`. Whether
   that passes XO0520 is unspecified:
   ```
   var committed = false
   defer {
     if !committed {
       let _ = dst.remove(tmp)
     }
   }
   ...
   dst.rename(tmp, path)?
   committed = true
   ```
   A `defer_on_err` or `errdefer` form would remove the flag entirely.
2. **Cleanup cannot run after cancellation.** Cancellation is sticky (core
   7.2), so a cancelled task's deferred `dst.remove(tmp)` itself returns
   `Canceled` and the temp file stays. "Always clean up the temp file on
   failure" is not achievable for the cancel case with the given API. There
   is no "shielded" or non cancellable cleanup.
3. **No arm level statement sequencing.** Every arm that prints and counts
   needs a block, which tripled `apply` and `preview`:
   ```
   Copy(path, _) => {
     out.println("would copy {path}")
     r.copied += 1
   }
   ```
4. **`Fs.sub` requires an existing directory.** Dry run against a missing
   `dst` forced `dst: Fs?` through `open_dst` and `sync`, and a separate
   `None => []` branch.
5. **Capability attenuation is all or nothing per root.** "Read only under
   src, write only under dst" works, but `main` still has to call
   `os.fs.create_dir_all(dst)` with the full capability before it can `sub`.
6. **Path helpers missing.** No path join, parent, or base name in std, and
   no `rfind`/`last_index_of` on `Str`. `parent_dir` is hand rolled.
7. **`const` must be compile time.** I wanted `const T0 = Time.from_unix(...)`
   in the tests; had to store the Int and call `Time.from_unix(T0)` in each
   test.
8. **Testing failure injection is under specified.** `fail_next` paths
   (gap 5) made the most important test (no temp file left) the least
   certain one.

## 4. Confidence

| File | Estimated remaining spec violations |
|---|---|
| `main.xo` | 0 to 1 (the tail `match` in `parse_args` with a `return` arm; the `Err("...")` conversion in `main`) |
| `sync.xo` | 1 to 2 (XO0520 on `committed`; nested prefix omission like `Some(MakeDir(...))`; possibly the explicit `()` tail is redundant but legal) |
| `sync_test.xo` | 1 to 3 (constructing `FileInfo` literals; `fail_next` path semantics; bare `Rename` for `testing.FsOp`) |
