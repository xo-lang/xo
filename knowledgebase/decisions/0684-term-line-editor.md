# 0684: std/term line editor (golang.org/x/term's Terminal)

Status: Proposed (Xo Lang 3, 2026-11-16; task #89, the matrix's term
rows)

## Context

std/term has color (Painter) and strip; the prelude's Stdio covers
x/term's IsTerminal, GetSize, MakeRaw, GetState, Restore and ReadPassword
(map.txt). The 11 rows left are x/term v0.48.0's line editor: Terminal,
NewTerminal, ReadLine, ReadPassword, SetPrompt, SetSize, Write,
SetBracketedPasteMode, History, EscapeCodes and ErrPasteIndicator. It is
what an SSH server's shell, a REPL or an interactive CLI uses for a prompt
with editing keys (arrows, Home and End, word moves and deletes, Ctrl-U,
Ctrl-K, Ctrl-W, Ctrl-T, Ctrl-L), history, bracketed paste, a password
prompt with no echo, and output written above the prompt while a read
waits.

Go's Terminal reads and writes one `io.ReadWriter`. Xo has no Reader or
Writer interface; std/log solved the same shape for output with
`interface Sink[fx: Effects]` and constructors for stdio, files,
connections and functions (decision 0617 C).

## Decision

1. **The I/O.** `interface Port[fx: Effects] { fn read(self, max: Int) ->
   Bytes ! Str uses fx; fn write(self, data: Bytes) -> () ! Str uses fx }`,
   an empty read meaning end of input, with `stdio_port(io: Stdio) ->
   Port[stdio]` (standard input and output; the caller puts the terminal in
   raw mode with `Stdio.raw_input`, as Go's callers call MakeRaw),
   `conn_port(c: Conn) -> Port[net.dial]` (an SSH channel or a socket) and
   `func_port[fx](read, write) -> Port[fx]` (tests, and anything else).
2. **The editor.** `struct Terminal[fx: Effects]` (its state behind a
   Mutex, so `write` from another task while `read_line` waits works as
   Go's lock released around Read does), made by
   `new_terminal[fx](p: Port[fx], prompt: Str) -> Terminal[fx]`, with:
   - `read_line() -> Line ! TermErr uses fx` where `struct Line { text:
     Str, pasted: Bool }`: `pasted` is Go's ErrPasteIndicator (Go returns
     the line and that error together; here it is a field, so a caller
     cannot lose the text by treating it as a failure);
   - `read_password(prompt: Str) -> Str ! TermErr uses fx` (no echo, not
     added to history, the old prompt restored);
   - `write(data: Bytes) -> () ! TermErr uses fx` (Go's Write: the prompt
     and line are erased, data written with LF as CRLF, then repainted);
   - `set_prompt(p: Str)`, `set_size(width: Int, height: Int) -> () !
     TermErr uses fx` (Go's SetSize, including its repaint for a
     narrowing width), `set_bracketed_paste(on: Bool) -> () ! TermErr uses
     fx`;
   - `set_history(h: History)` with `interface History { fn add(self, e:
     Str); fn len(self) -> Int; fn at(self, i: Int) -> Str }` and
     `ring_history(cap: Int = 100) -> History` (Go's default stRingBuffer
     of 100), and `set_autocomplete(f: fn(Str, Int, Char) -> (Str, Int)?)`
     (Go's AutoCompleteCallback, None to leave the key to the editor).
   - `enum TermErr { Eof, Io(msg: Str) } derive Display`: Ctrl-D on an
     empty line and Ctrl-C are Eof, as Go returns io.EOF for both.
3. **Escape codes**: `term.ESCAPE: EscapeCodes` with Go's VT100 values as
   Bytes fields (`black` to `white`, `reset`), and `t.escape()` to read
   them, so a program colors what it writes the way Go's Terminal.Escape
   does. Painter stays the module's ANSI helper for plain output.
4. **Behavior is Go's**: bytesToKey's escape sequences (CSI and SS3
   arrows, Home and End with their variants, Alt-arrows, paste start and
   end), handleKey's edits and cursor arithmetic, visualLength (zero width
   for escape sequences in the prompt), the CRLF handling, and the 4096
   byte line limit. Go's terminal_test.go is ported in full (keyPressTests,
   the ReadPassword, history, SetSize and paste tests) over `func_port`
   with a mock that feeds input and records output, as Go's
   MockTerminal; the output bytes must equal Go's. A timing row against Go
   on a scripted session goes in bench/perf/term.

## Consequences

- std/term gains its first interface and its first effectful API; the
  module stays pure Xo (no native part), so every backend, wasm included,
  runs it.
- map.txt's 11 term rows map to these names; Painter and strip are
  unchanged.
- An SSH server in Xo (std/ssh's channels as a Port) can offer Go's shell
  prompt with the same bytes on the wire.

## Alternatives

- **A sans-I/O editor** (`feed(input) -> output and lines`): simplest to
  test, but a program that writes while a read waits must interleave the
  editor's repaint itself, which Go's Terminal does for it; and every
  caller writes the same read loop. The Port design keeps Go's shape and
  tests just as well through func_port.
- **Go's (line, error) pair for pastes**: an error carrying a valid line
  is easy to drop; the `pasted` field cannot be.
- **The Sink interface from std/log**: write only, and line oriented.
