# 0711: std/grpc client health checking

Status: Accepted (xo-4c, 2026-11-17)

## Context

grpc-go's client health checking (health/client.go clientHealthCheck, clientconn.go startHealthCheck) runs a
grpc.health.v1.Health/Watch stream on each subchannel when the service config has a healthCheckConfig, the
health package is linked and the balancer asks for it (round_robin does; pick_first does not). A subchannel whose
Watch reads other than SERVING is TRANSIENT_FAILURE, so the picker leaves it out; an Unimplemented Watch means
the server has no Health service and the subchannel stays READY; any other failure restarts the stream after the
DefaultExponential backoff (1 s, times 1.6, at most 120 s, 0.2 jitter; reset by an answer). std/grpc had the
balancers (decision 0707) and read healthCheckConfig past (task #236, map.txt grpc.ServiceConfig).

A std/grpc client has no background goroutines of its own: every task it runs belongs to a scope the program
owns (structured concurrency, core 7). A Watch stream per address needs a long running task.

## Decision

- `ServiceConfig.health_service: Str?` from `healthCheckConfig.serviceName` ("" when the config has no name, None
  without healthCheckConfig), with grpc-go's validation (not an object, a name that is not a string: error).
- `Client.with_health_watch(tasks: task.Group, clock: Clock) -> Client`: the program gives the task group the
  watches run in and the clock their backoff sleeps on. Without it there is no health checking, which is also
  how grpc.WithDisableHealthCheck maps.
- Watching runs only with RoundRobin and a health_service, as in grpc-go. The first call (after the resolver's
  first answer) starts one watch per address, and later calls start one for each address a resolver adds; a
  watch ends when its address leaves the list, or the scope of the group ends.
- The watch is a plain transport stream: no interceptors, stats handler, retries or balancing on it.
- Readings: SERVING healthy; any other status, a failed stream or a stream that ends, unhealthy; Unimplemented
  healthy and the watch stops. The backoff is grpc-go's without the jitter, reset after any answer.
- RoundRobin turns over the addresses not read unhealthy (an address not read yet counts as healthy); when every
  address reads unhealthy the call fails Unavailable "connection active but health check failed.
  status=NOT_SERVING" (grpc-go's message for the TRANSIENT_FAILURE picker).

## Consequences

- grpc-go's health checked round_robin deployments port with one more line (with_health_watch).
- Tests: std/grpc_test.xo "with_health_watch sends calls only to addresses whose health is SERVING, as grpc-go's"
  and the golden grpc_health_watch (std/grpchealth servers).
- The first call after the client starts can still go to an unhealthy address, before its first reading comes
  in; grpc-go waits for the first reading in CONNECTING. A later stage may wait too, bounded.
- No jitter: many clients started together retry a failing watch at the same times.

## Alternatives

- A client that spawns its own background tasks: rejected, every task belongs to a program scope.
- Calling grpchealth from std/grpc: rejected, std/grpchealth imports std/grpc (a cycle); the Watch request and
  response are two protobuf fields, encoded by hand.
- Health checking under pick_first too: rejected, grpc-go's pick_first does not check.
