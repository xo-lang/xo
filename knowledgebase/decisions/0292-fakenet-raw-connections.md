# 0292: FakeNet raw connections

Status: Accepted (owner, 2026-10-20)

## Context

Tests of anything that speaks a protocol over a raw connection (std/sql's
Postgres driver, SMTP after decision 0290, a Redis or NATS client later)
cannot be unit tests today: FakeNet's in memory listeners serve only
`http.server`, and `os.net.dial` in a test always fails ("raw
connections are not faked", std/rt testing.go). So the Postgres driver's
fake backend is a golden program on real local sockets
(internal/llvm/testdata/golden/pg_fake), not a `test` block: slower, not
on the virtual clock, no deadlock report, and not runnable under
`xo test --watch`.

## Decision

spec/std.md 8.1 gains raw connections between a test's dials and its in
memory listeners:

- `os.net.dial(addr)` in a test connects in process to the in memory
  listener at `addr`: the listener's `accept()` returns a `ServerConn`
  paired with the dialer's `Conn`; what one side writes the other reads,
  in order, with the bytes the writer gave (a read returns what is
  buffered, at most the size asked, and waits while nothing is). Each
  direction buffers 64 KiB; a write waits for room. Closing one side ends
  the other's reads (`None`, the end of the stream, after the buffered bytes)
  and fails its writes with `NetErr.Io("write tcp: connection reset by peer")`.
- `os.net.listen(addr)` takes a host name too: `listen("db.test:5432")`
  listens at exactly that address, so a program dialing its configured
  host reaches the fake unchanged; `":0"` and `"127.0.0.1:0"` pick ports
  from 49152 as now. A second listener on an address in use fails with
  `NetErr.Io("address already in use")`.
- A dial to an address with no listener fails at once with
  `NetErr.Refused` (as a real refused connection); a dial to a listener
  returns at once, the connection waiting in the listener's backlog until
  `accept()` returns its other end (as a real connect completes before
  accept); closing the listener resets the connections still waiting.
- Waits are task waits: the virtual clock moves while every context
  waits, and cancellation applies to accept, reads, and writes as on
  real connections. `tls.client` passes the bytes through, as on every
  FakeNet connection (std.md 27), so a driver's TLS upgrade (Postgres's
  SSLRequest, SMTP's STARTTLS) runs against a plain fake server;
  certificate checks stay with golden `tls_client` and the Docker
  differential tests.
- `FakeNet.only(hosts)` applies to dials as it does to HTTP requests.
- Same on the Go backend (std/rt: a buffered pipe per direction) and the
  native backends (std/native/llrt/twrun.xo: the in memory listener gains
  a queue of pending connections and two ring buffers per connection).

### Example (what pg_fake becomes)

```
test "the pool reconnects after the server goes away" {
  let l = os.net.listen("db.test:5432")?
  os.tasks.spawn(fn() { fake_backend(l, script: [trust_login(), select_one(), close()]) })
  let db = sql.open(os.net, "postgres", "postgres://app@db.test:5432/app?sslmode=disable")?
  expect(db.query_one("select 1")?.int(0)) == 1
  ...
}
```

## Consequences

- pg_fake becomes `test` blocks in std/native/llrt/pg_test.xo, and the
  std/smtp tests of decision 0290 use the same fakes.
- A dial in a test no longer always fails: a program that dialed a fake
  address and expected the failure now gets `Refused` (no listener) or a
  connection (a listener).
- std/rt's FakeNet.dial and the native test world (twrun.xo) implement
  the same buffers, closing, and errors; a golden per backend checks
  them.

## Alternatives

- Keep raw protocol tests as goldens on real sockets: works, but each is
  a separate program, runs on wall time, and cannot use the test fakes
  (FakeClock for timeouts, MemFs for certificates).
- A `FakeNet.handle_tcp(addr, fn(ServerConn))` callback instead of
  listen and dial: shorter for one connection, but a second API next to
  listen, and the callback form cannot express the server reading
  several connections (the pool).
