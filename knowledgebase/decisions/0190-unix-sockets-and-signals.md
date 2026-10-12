# 0190: Unix domain sockets and signals to other processes

Status: Accepted, implemented (2026-10-12) on the Go backend, arm64, and
LLVM, with the self hosted SSA builder mirrored.

## Context

Stage 5 of decision 0178 moves `xo serve`, `xo dump`, `xo lsp`,
`xo mcp`, and `xo replay` to Xo. Two of them need something no Xo
program could do:

- `xo serve` listens on a Unix domain socket (internal/serve: a socket
  file under the user cache directory, `XO_SERVE_SOCKET`), and the CLI's
  `serve stop|status` and the forwarding of `check` and `query` dial it.
  `Net` was TCP only (decision 0071).
- `xo dump <pid>` sends SIGQUIT to a running Xo program, which writes
  its task tree to a file (core 11.4). `Proc` could kill only children
  it started (`Child.kill`, decision 0161).

Both are authority over things outside the program, so they must be
capabilities (decision 0004) that attenuation can withhold (decision
0156), and they must behave the same on every backend (decisions 0172,
0175).

## Decision

1. **Sockets are addresses, not new methods.** `Net.listen` and
   `Net.dial` take `unix:PATH`. A new method pair (`listen_unix`) would
   duplicate `Listener` and `Conn` for no gain: once connected, a
   stream is a stream. The address form is what Go's `net` spells as a
   network argument; `unix:` cannot be a TCP host (no port), so the two
   never collide.
   - `Listener.addr()` is the address as given. Closing the listener
     removes the socket file, once (Go's `UnixListener` does; the
     native runtime keeps a flag so a second close cannot unlink a new
     server's file).
   - Errors are Go's texts: `listen unix PATH: bind: address already in
     use`, `dial unix PATH: connect: no such file or directory`, a file
     with no listener is `Refused`, a path longer than `sun_path` is
     `invalid argument`. The C runtime's `err_text` gained the lower
     case texts of ENOENT, EINVAL, ENOTDIR, EPROTOTYPE.
   - Attenuation: an entry `unix:PATH` admits exactly that address in
     every list. Entries are trimmed; a `unix:` entry keeps the case of
     its path (host entries are lowered). Host entries never admit a
     socket; `unix:` entries never admit a host.
   - FakeNet listens on `unix:` addresses in memory (dial stays
     Refused, as for TCP).
2. **`Proc.kill(pid: Int, signal: Str = "TERM") -> () ! ProcErr uses
   proc`.** Signals by name, from the portable set `HUP INT QUIT KILL
   TERM USR1 USR2 CONT STOP` (numbers differ between Linux and macOS for
   USR1 and USR2; names do not). Checked in this order: the attenuation
   lists, the name (`Io("unknown signal \"X\"")`), the pid (`<= 0` is
   `Io("invalid process id N")`: kill(2) would signal a process group or
   every process the user owns, which no Xo program should do by
   accident), then kill(2) (`Io` with Go's errno text: `no such
   process`, `operation not permitted`).
   - Attenuation: an attenuated Proc sends a signal only when every list
     names `"kill"`, else `Denied("kill")`; it reads like the command
     line `kill` it replaces, and it needs no new list kind.
   - The Go backend records it as the effect `proc.kill` (record and
     replay do not send it again). Where there are no Unix signals
     (Windows), only `KILL` is sent (`os.Process.Kill`).
   - FakeProc.kill checks the name and pid and records `"NAME PID"`;
     `FakeProc.killed() -> List[Str]` returns them.
3. **Implementation.** Go: std/rt prockill.go (plus `_unix`, `_other`),
   caps.go listen and dial, attenuate.go. Native: the native prelude's
   `proc_kill` (capImpls `Proc.kill`, in internal/ssa and selfhost/ssa)
   checks lists, name, and pid in Xo and calls `rt.proc_kill(pid,
   index)`; `xo_proc_kill` and `xo_proc_killed` live in xo_proc.c, so the
   existing `xo_proc_` IOFlags rule includes them (no new runtime
   define); `unix_listen` and `unix_dial` in xo_rt.c. Test mode:
   `Proc.killed` lowers to `rt.proc_killed` in both builders.

Tests: the golden `unix_kill` (internal/llvm/testdata/golden: an echo
over a socket file, file removal on close, busy and missing paths,
attenuation with `unix:` entries and case, signals TERM, KILL, and QUIT
to children with their statuses 143, 137, 131, unknown names, pids 0
and -1, a pid that does not exist, attenuated Procs) is identical on Go,
arm64, and LLVM; `internal/gen/testdata/golden/test_unix_kill` runs
FakeProc.kill, killed, and a FakeNet socket on all three (cmd/xo
TestNoGCTests).

## Consequences

- `xo serve` and `xo dump` can be written in Xo (decision 0178 stage 5).
- Any program can signal a process of the same user given an
  unattenuated `os.proc`; that was already true through `Proc.run("kill",
  ...)`, so no authority is new, only cheaper.
- A socket path is not checked against `Fs` sub capabilities: a `Net`
  with a `unix:` entry is the authority, as a host entry is for TCP.

## Alternatives

- **`Net.listen_unix` / `dial_unix`**: two more methods on Net and on
  every fake and attenuated Net, for the same Listener and Conn.
- **A `Signal` enum**: type safe, but a new prelude enum costs a
  std_gen type and a native lowering; names are checked at run time
  with a clear error, and a typo in a literal is rare.
- **Signals by number**: USR1 is 30 on macOS and 10 on Linux.
- **`Proc.kill` allowed on any attenuated Proc**: attenuation exists to
  withhold authority; `kill` in the list states it.
