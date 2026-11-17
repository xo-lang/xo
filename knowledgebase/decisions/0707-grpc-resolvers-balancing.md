# 0707: std/grpc name resolution and load balancing

Status: Accepted (owner, 2026-11-17, xw decision #89: option 1; built by xo-4c, task #236)

## Context

scripts/stdaudit/map.txt keeps these grpc rows open: WithResolvers,
WithLocalDNSResolution, PickFirstBalancerName, ServiceConfig's
loadBalancingConfig, and behavior.name_resolution. A std/grpc Client
sends every call to one base URL: `dns:///h:p` and `passthrough:///h:p`
targets become `http://h:p`, and std/http's dialer picks one address of
h per connection. grpc-go resolves the target into a list of addresses
(a resolver: dns, passthrough, unix, manual, or a plug-in from
WithResolvers), and a balancer picks the address of each call:
pick_first (the default: the first address that connects, the next on
failure) or round_robin (each call the next ready address), chosen by
the service config's loadBalancingConfig. http.Client.with_dial_addr
(task #236, pushed with unix: targets) dials a fixed address while the
request keeps the target's host, which is what a per-address
connection needs.

## Decision

Typed resolvers and two balancers, as grpc-go's built-in ones.

- `enum Balancer { PickFirst, RoundRobin }`; `Client.with_balancer(b)`
  (default PickFirst). ServiceConfig gains `balancer: Balancer?`, read
  from `loadBalancingConfig` (the first policy std/grpc knows, as
  grpc-go) and `loadBalancingPolicy`.
- `Client.with_addrs(addrs: List[Str])`: a fixed address list (grpc-go's
  resolver/manual), each "host:port" or "unix:/path". The target stays
  the :authority and the TLS server name.
- `Client.with_dns(n: Net, clock: Clock)` (the clock times the 30 s interval; added when built): a `dns:` or bare target is resolved by
  `n.lookup_host` (Net's effects are counted where with_dns is called,
  as decision 0703's interceptors), again after every address failed
  and at most every 30 s (grpc-go's min resolution interval).
  Without it nothing changes: std/http's dialer resolves as today.
- Each address gets its own http.Client (`with_dial_addr`), kept in the
  Client's shared state; pick_first uses the first that connects and
  moves on when a call fails Unavailable before any byte went out;
  round_robin turns per call over the addresses whose last connect did
  not fail (all of them when none is ready), as grpc-go's picker.
- Plug-in resolvers (grpc-go's resolver.Builder, WithResolvers) are a
  `fn() -> List[Str] ! Status` given to `Client.with_resolver(f, clock)`, run
  where with_dns would run.

## Consequences

- Closes the five rows; behavior.name_resolution moves to done for
  dns, passthrough, unix and manual targets with both balancers.
- Interop: TestGrpcInterop gains a case with two grpc-go servers and an
  Xo client in round_robin (calls alternate) and pick_first (all calls
  to the first; the second after the first stops).
- xds, grpclb, weighted and outlier balancers stay not planned.

## Alternatives

- A balancer interface users implement (grpc-go's balancer.Builder):
  larger API, no user asked; the two built-ins cover the map rows.
- Resolving inside std/http (a pool over all addresses of a host):
  changes every http.Client, and Go's net/http does not do it.
