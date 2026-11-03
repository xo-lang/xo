# 0462: native dial races IPv6 and IPv4 (Go's dual stack dial)

Status: Accepted (xo-lang-4, 2026-11-02, a fix the Lead assigned: Go parity)

## Context

Lang 1's Windows concurrency stress (2026-11-02, main 02e6ffa1a) failed
the golden tls_coalesced_finish in 128 of 128 runs: the 60 s timeout, no
output. It is not a hang. Alone and idle on Windows the golden took 84 s
(8 copies at once: 83 s each), against well under a second on Linux.
Timestamps put about 2.04 s of every connection between the client's
`post` and the proxy's accept: the client dials `localhost:PORT`,
Windows resolves localhost to `::1` first and then `127.0.0.1`, the
proxy listens on 127.0.0.1 only, and a connect that is refused on
Windows takes about 2 s (it retries the SYN after the reset), where
Linux and macOS refuse at once. A probe on the same host: dial
127.0.0.1 505 us, `[::1]` refused in 2.02 s, `localhost` 2.02 s. Xo's
native dial tried the addresses one at a time, so every dial to
localhost on Windows with an IPv4-only listener cost 2 s (40
connections: 84 s; under 32 copies and CPU burners past 60 s).

Go dials such a name in parallel (net/dial.go dialParallel, RFC 6555
Happy Eyeballs): the first family's addresses in order, and after a 300
ms fallback delay the other family's in a second race, the first
connection winning.

## Decision

`netconn.dial_timeout` keeps its one-at-a-time loop for a name of one
family and races a name with both (`dial_race`):

- the primaries are the addresses of the first one's family, the
  fallbacks the others, each in the resolver's order, each chain one
  attempt at a time;
- the fallback chain starts 300 ms after the first attempt started, or
  at once when the primaries are used up;
- with two attempts in flight the task waits on both sockets at once:
  `poll.pd_wait2_w` puts the task's one park in both descriptors' write
  slots (readiness handed to the side not returned is kept for its next
  wait), with the deadline on the first's timer;
- the first connection wins, the other attempt is dropped; with none, the
  primaries' last error (the fallbacks' when the primaries got none),
  canceled and the dial's time limit as before.

## Consequences

- tls_coalesced_finish on Windows: 84 s to under a second per run alone
  (12 network goldens in 30 s), and it passes Lang 1's stress; a dial to
  localhost on Windows with a listener on 127.0.0.1 waits 300 ms, as in
  Go, instead of 2 s.
- Golden net_dial_dual_stack: five dials to localhost with the listener
  on 127.0.0.1 must take under 5 s (main's xo on Windows: 10 s, `fast
  false`; with the change it passes) and a listener on ::1 alone still
  connects.
- A name of one family (every IP literal, most DNS names on Linux,
  localhost where hosts maps it to 127.0.0.1 only) dials as before.
- FakeNet, the Go backend (Go's own dial) and wasm are unchanged.

## Alternatives

- Fix the golden only (dial 127.0.0.1): hides a 2 s cost every Windows
  client dialing localhost pays.
- A per-address timeout of 300 ms, one at a time: drops a slow but
  working first address; Go keeps it in the race.
- Shorten Windows' connect retries (TCP_MAXRTMS): changes how long a dial
  to a slow real host waits.
