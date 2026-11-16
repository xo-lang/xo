# 0677: HTTP client trace events (net/http/httptrace)

Status: Proposed (Xo Lang 3, 2026-11-16; task #55)

## Context

Go's net/http/httptrace lets a client request carry a `ClientTrace`, a
struct of optional callbacks that the transport calls as the exchange
moves: GetConn, GotConn (with Reused, WasIdle, IdleTime), PutIdleConn,
GotFirstResponseByte, DNSStart and DNSDone, ConnectStart and ConnectDone,
TLSHandshakeStart and TLSHandshakeDone, WroteHeaderField, WroteHeaders,
Wait100Continue, Got100Continue, Got1xxResponse and WroteRequest.
client_golang's promhttp builds on it: `InstrumentRoundTripperTrace`
observes the time to each event, and `InstrumentTrace` is that struct of
observers. Both are open std/metrics gaps (task #55), and std/http has no
way to see inside an exchange today.

On the native backends the exchange runs in std/native/llrt (http1.xo
client_do, hclient.xo cl_start: the pool, the dial, the TLS handshake),
which keeps its state in raw records and calls no Xo closures. On the Go
backend std/rt's client is net/http itself, so Go's hooks are at hand.

## Decision (proposed)

1. A request carries trace events, not callbacks:
   `Request.with_trace(on: fn(TraceEvent) uses sync) -> Request`. One
   function receives every event, each an enum variant with the time it
   happened and Go's fields:
   `GetConn(host_port)`, `GotConn(reused, was_idle, idle: Duration)`,
   `DnsStart(host)`, `DnsDone(addrs, err)`, `ConnectStart(network, addr)`,
   `ConnectDone(network, addr, err)`, `TlsHandshakeStart`,
   `TlsHandshakeDone(version, alpn, err)`, `WroteHeaders`,
   `WroteRequest(err)`, `GotFirstResponseByte`, `Got100Continue`,
   `PutIdleConn(err)`. A `TraceEvent` also carries `at: Time`, the
   instant the transport saw it.
2. Native backends record each event's kind, time and fields in the
   exchange's record as llrt reaches it; std/http calls `on` for each,
   in order, when the exchange returns (or fails). The times are the
   transport's, so durations are exact; only the moment the function
   runs is later than Go's.
3. The Go backend calls `on` from Go's ClientTrace hooks as they fire,
   with the same events and `at`.
4. std/metrics' `instrument_round_tripper_trace` and `InstrumentTrace`
   map onto `with_trace`: an observer per event kind, fed the time since
   the request started.

## Consequences

- promhttp's trace instrumentation and request timing breakdowns
  (DNS, connect, TLS, time to first byte) become possible on every
  backend, with exact times.
- A hook that acts during the exchange (cancel when DNS is slow, log the
  connection the moment it is chosen) is not possible on native backends:
  events arrive after the exchange. Go's own tests check order and
  fields, which this keeps.
- WroteHeaderField, Wait100Continue and Got1xxResponse are left out at
  first (no promhttp observer uses them); they can be added as variants.

## Alternatives

- Go's struct of optional callbacks called live from llrt: llrt would
  need to call Xo closures from inside the exchange (its sockets, its
  locks, the scheduler's preemption); larger and riskier, for hooks whose
  common use is timing.
- A channel of events the caller reads while the request runs: live, but
  every caller has to drain it or the exchange blocks.
- Only durations (a `Timings` value on the response): simpler, but not
  Go's event model, and promhttp's observers need the events.
