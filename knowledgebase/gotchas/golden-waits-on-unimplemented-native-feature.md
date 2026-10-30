# A golden that waits on an unimplemented native feature hangs

Status: Observed 2026-10-30

## Symptom

`http_h2_config`'s keepalive probe (a client that reads until the server
sends a PING and closes) ran for 20 minutes in a gate and held a heavy.sh
slot: the native h2 server had no PING keepalive, so the connection stayed
open and the blocking read never returned. The Go backend passed it.

## Cause

A probe that reads "until the server does X" has no end when the native
backend never does X. The golden had no deadline of its own, and `xo run`
has none.

## Fix

Give every such probe a read deadline (`c.set_read_deadline(os.clock,
Some(os.clock.now() + 5s))`) so the missing feature prints a failing row
in seconds. Stop a hung run by its own PIDs, never by name.
