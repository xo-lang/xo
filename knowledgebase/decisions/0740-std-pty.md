# 0740: std/pty: pseudo-terminals and a child with a controlling terminal

Status: Proposed (task #691, owner 2026-11-22, from #263; xo-4c). Stub:
the design follows.

## Context

xw's password login to hosts needs a child whose controlling terminal is
one the program holds (ssh reads a password from /dev/tty, not standard
input); later users are xo debug and tests of interactive programs. Xo can
spawn a child (decisions 0107, 0161, 0739) but cannot open a
pseudo-terminal pair or give a child a controlling terminal.

## Decision

To be written: the std/pty API (open a pair, spawn with the slave as the
controlling terminal while the standard streams may stay pipes, window
size, raw and cooked modes) on darwin, linux, freebsd and windows
(ConPTY), shaped after github.com/creack/pty.

## Consequences

To be written.

## Alternatives

To be written.
