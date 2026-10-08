# Task 04 (file sync), paper test notes, spec v0.3

Files: `main.xo` (args, platform rules, capability wiring, exit),
`sync.xo` (sync logic), `sync_test.xo` (tests).

## 1. Spec gaps

1. **`Fs.create_temp` return value** (std 3.1). The signature says "returns new
   file path" but not whether it is relative to the capability root or to the
   `dir` argument. Guessed: relative to the capability root, like
   `FileInfo.path`, so it can be passed straight to `write` and `rename`.
   Marked `// GUESS` in `sync.xo`.
2. **`return Err(e)` with a concrete error in a `! Error` function** (core 2.6,
   2.10). 2.6 says a returned `Err(e)` needs `e: E`; 2.10 rule 2 says a value
   becomes an interface "where an interface type is expected (... a return)".
   It is not stated whether that reaches inside `Err(...)`. Guessed yes.
   Marked `// GUESS` in `main.xo`.
3. **Write only authority does not exist** (core 6.3, std 3.1). The task says
   the tool may only "write under dst"; it must also read dst to compare. There
   is `read_only()` but no `write_only()`. I used `sub(dst)` (read and write)
   and `sub(src).read_only()`. Not a guess, but the spec cannot express the
   tightest version of the requirement.
4. **Case sensitivity source of truth** (std 3.1 `case_sensitive()` vs the
   task). The task fixes the rule per OS; the spec also offers
   `Fs.case_sensitive()` per filesystem. I followed the task with
   `match target.os`, and injected the result through `Options` so tests can
   choose it.
5. **`walk` on a missing directory** (std 3.1). Not stated that it fails with
   `NotFound`. I assumed so and treat a missing `dst` as empty (needed for dry
   runs, where `dst` cannot be created).
6. **`create_dir_all(".")` on a capability root** (std 3.1). Unclear if it is a
   no op or an error. I avoided calling it for top level files.
7. **Does `MemFs.write` create parent directories?** (std 8.3). Silent. Tests
   call `create_dir_all` first to stay safe.
8. **`set_modified` then `rename` keeps the mtime on a real filesystem?** MemFs
   is documented to keep the source's modified time on rename (std 8.3); the
   real `Fs` is not. I rely on it so the next run skips the file.
9. **Iterating or querying a `Set`** (std 2.4). `Set` has `contains`, `union`,
   `to_list` but no `any`, and core 3.5 does not list `for x in set`. I used
   `opts.skip_names.to_list().any(...)`.
10. **Testing `main`** (core 9.2, std 8). `main` takes `Os`, tests have
    `TestOs`; there is no stated conversion, so the exit status path and
    `platform_rules()` (which depends on the host `target.os`) cannot be tested.
    I moved all logic into `sync` and left `main` thin.
11. **Helpers in `_test.xo` files** (core 9.2). `expect` and `testing` are
    available anywhere in a `_test.xo` file, but `os` only inside test blocks,
    so `setup_dirs` takes an `Fs` parameter. Clear once read, but easy to miss.
12. **Compound assignment on a field** (core 5.2). `u.name = "b"` on a `var u`
    is defined; `report.copied += 1` is not shown. Assumed it works the same.
13. **Unit value in a match arm** (core 1.6). `Ok(_) => ()` uses `()` as the
    unit value; 1.6 says an empty block used as a value is written `()`, which I
    read as covering this.
14. **Exit status for "something failed"** (std 3.7). Returning `Err` from main
    prints the error "with its frames" and exits 1. For an expected outcome
    (some copies failed) frames are noise, but the spec says prefer `Err` over
    `proc.exit`, so I followed it.

## 2. Habits

- Started writing Go `flag.Bool("dry-run", ...)`. There is no flag module, so
  I wrote `parse_args` with a `match` on the string and a list pattern
  `[src, dst]` for the positional arguments.
- Reached for `if err != nil { failed++; continue }` around each copy. Changed
  to `match copy_file(...) { Ok(_) => ..., Err(Canceled) => ..., Err(err) => ... }`.
- Wrote Go's `defer os.Remove(tmp)` and then remembered it must only run on
  failure. Used a `var done = false` flag read inside a `defer` block. Also
  checked that xo `defer` is block scoped (it is at the top block of
  `copy_file`, so the Go intuition still held).
- Wanted `filepath.Dir` and `strings.EqualFold`. Wrote `parent_dir`,
  `ancestors`, and `path_key` (with `to_lower`).
- Wanted `os.Exit(1)` after printing the summary. Changed to
  `return Err(CliErr.Failed(count: ...))` per std 3.7.
- Wrote `Some(d) => ...` in `needs_copy` where `d` is already the parameter
  (Rust habit of shadowing). Renamed to `Some(existing)`.
- Wanted Rust's `.iter().any()` on a `HashSet`; used `to_list().any(...)`.
- Wanted `errors.Is(err, context.Canceled)`; the xo version is a pattern,
  `Err(Canceled) => return Err(FsErr.Canceled)`.
- Started typing `#[cfg(target_os = "macos")]` style thinking for the skip list;
  used `match target.os { Darwin => ... }`.

## 3. Friction

- **Re-propagating cancellation by hand.** Because per file errors are counted
  instead of propagated, every `match` on an fs result needs an extra arm, or a
  cancelled run would keep going and count every remaining file as failed:
  ```
  match dst.remove(e.path) {
    Ok(_) => { ... }
    Err(Canceled) => return Err(FsErr.Canceled)
    Err(err) => { ... report.failed += 1 }
  }
  ```
  This appears three times in `sync.xo`. A helper like `r.reraise_canceled()`
  or an `is_canceled()` on errors would remove it.
- **Success flag for cleanup in `defer`.** There is no "defer on error" form,
  so `copy_file` needs a mutable flag:
  ```
  var done = false
  defer {
    if !done {
      let _ = dst.remove(tmp)
    }
  }
  ...
  dst.rename(tmp, info.path)?
  done = true
  ```
  It works, and the shielding rule makes it correct under cancellation, but it
  is the same boilerplate as Go.
- **Capabilities can be narrowed to read only but not write only.** The task's
  "only write under dst" maps cleanly to `sub(dst)`, but "only read under src"
  is not fully true because `dst` is readable too.
- **Host dependent code is hard to test.** `match target.os` is resolved at
  compile time, so the macOS and Windows rules cannot be exercised on Linux.
  I had to thread `fold_case` and `skip_names` through `Options` instead of
  computing them where they are used.
- **Set has almost no query methods.** `opts.skip_names.to_list().any(...)`
  allocates a list just to test membership with case folding.
- **`main` is untestable.** All logic moved into `sync(src, dst, opts, out)`;
  the exit status rule ("exit 1 if anything failed") has no test.
- Mutation rules were not a problem: `var report = Report{}` with
  `report.copied += 1` read naturally, and value semantics meant passing
  `opts` into closures needed no thought.

## 4. Confidence

- `main.xo`: about 1 to 2 remaining violations (the `return Err(e)` conversion
  guess; possibly the multi line struct literal formatting).
- `sync.xo`: about 2 to 3 (the `create_temp` path guess, `+=` on a field, bare
  `Canceled` patterns relying on inferred scrutinee type, `[]` arm inference in
  the `dst.walk` match).
- `sync_test.xo`: about 2 to 3 (passing `os.fs` and `os.stdio` fakes where
  `Fs` and `Stdio` are expected, `expect(left) == []` typing, `Map` equality
  for `os.fs.files() == before`, whether `MemFs` paths and `sub("dst")` paths
  line up for `fail_next` as I assumed).
