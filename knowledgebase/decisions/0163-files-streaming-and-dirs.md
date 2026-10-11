# 0163: Files: handles, append, copy, modes, links, globs, locks, user dirs

Status: Accepted, implemented on the Go backend and MemFs (2026-10-11).
Native builds report the new methods as XO1201 (planned). Spec: std.md
3.1, 3.5, 8.3.

## Context

`Fs` read and wrote whole files only (std.md 3.1 said "streaming file
handles are planned"). Command line tools need more: process a log
larger than memory line by line, append to a file, copy a file with its
executable bit, create and read symbolic links, find files by pattern,
keep two runs of a tool from writing the same state at once, and find
where to keep configuration and caches the way each OS expects.

Constraints: capabilities stay the only authority (decision 0004), and a
sub capability's root must hold (decision 0024: lexical rooting); test
fakes (MemFs) must implement everything deterministically.

## Decision

1. **Handles.** `Fs.open(path)` (read), `create(path)` (write,
   truncate), `open_append(path)` return a `File` handle with `read(max)
   -> Bytes?`, `read_line() -> Str?`, `write(data)`, `close()`. One
   direction per handle, like the common case and simpler to fake. No
   seek, no read-write handles (not needed by the tools in view; can be
   added). Writes go straight to the OS (no Xo buffering, so no flush
   step to forget); reads are buffered for `read_line`.
2. **Whole-file helpers**: `append(path, data)`, `copy(from, to)`
   (contents and permission bits).
3. **Permission bits** as an `Int` (`0o750`): `mode(path)`,
   `set_mode(path, mode)`. Not a field of `FileInfo`: adding fields to a
   std struct breaks code that builds one with a literal (tests do), and
   the bits are wanted rarely.
4. **Symbolic links**: `symlink(target, link)`, `read_link(path) ->
   Str?` (None for a regular file, so it doubles as "is a link"). In a sub
   capability a link whose target is absolute or climbs out of the root
   is `Denied`: creating one through the capability would turn a
   lexical root into an escape hatch for whoever holds write access.
   Existing links are still followed by the OS (the known limit of
   lexical rooting, decision 0024; `os.Root`-style resolution is a later
   step).
5. **Globs**: `glob(pattern) -> List[Str]` sorted, `*`, `?`, classes,
   and `**` segments, hidden names only for patterns that start with
   `.`, implemented once over `read_dir` and `exists`, so every `Fs`
   (real, sub, read only, MemFs) gets the same matching and attenuation.
6. **Advisory locks**: `lock(path) -> FileLock` waits (a cancellation
   point, polling the OS lock every 10 ms so cancellation is prompt),
   `try_lock(path) -> FileLock?` does not; `FileLock.unlock()`. The OS
   lock (`flock`, `LockFileEx`) excludes other processes and other
   holders in the same process. MemFs locks use the runtime's semaphore,
   so waits between tasks are scheduled and reported like a `Mutex`'s.
7. **User directories on `Env`**: `config_dir()`, `cache_dir()`. They
   are computed from variables (`XDG_CONFIG_HOME`, `HOME`, `AppData`),
   so they belong to `Env` and its attenuation; the result is a path
   for `os.fs.sub`. `XDG_*` is honored on every OS when absolute (many
   tools do so on macOS; Go's `os.UserConfigDir` does not), else the
   OS's place (Linux `~/.config` and `~/.cache`, macOS `~/Library/...`,
   Windows `%AppData%` and `%LocalAppData%`).
8. **Effects**: reads `fs.read`, writes `fs.write`, `copy` `fs`; a
   `File`'s methods carry the effect of their direction.

## Consequences

- `Fs` gains 12 methods, the prelude `File` and `FileLock`, `Env` two;
  user implementations of `Fs` or `Env` (rare) must add them.
- Record and replay records the whole-file helpers, `mode`, links, and
  `glob`; handle operations and locks are not recorded yet.
- Native backends: `xo_fs.c` has whole-file reads and writes only. The
  plan is descriptors as handles in the C runtime (`open`, `read`,
  `write` on the event loop like sockets), `flock`, `symlink`,
  `readlink`, `chmod`, and the glob in Xo over `read_dir` (it is
  backend neutral already); `Env.config_dir` can move to the native
  prelude unchanged.

## Alternatives

- `FileInfo.mode` and `FileInfo.is_link` fields: more discoverable, but
  break struct literals of `FileInfo` and every MemFs listing format.
- A `Reader`/`Writer` interface pair shared with sockets and processes:
  attractive for composition (copy a `Child`'s output into a file), but a
  larger design; the three byte stream APIs (Conn, Child, File) already
  share the `read(max) -> Bytes?` shape, so an interface can be added
  over them later without breaking them.
- Blocking `lock` in a goroutine instead of polling: `flock` cannot be
  interrupted portably, which would make cancellation leak a goroutine
  and a lock acquired after the task gave up.
- Following Go's `os.UserConfigDir` exactly (no XDG on macOS): rejected
  because users of command line tools set `XDG_CONFIG_HOME` on macOS
  expecting it to work, and tests become portable by setting it.
