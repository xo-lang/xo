# 0705: std/grpc service config and retries

Status: Accepted (owner, 2026-11-17, xw decision 77, option 1 of ask 74; task #236)

## Context

scripts/stdaudit/map.txt has ten grpc gap rows for service config and
retries: ServiceConfig, MethodConfig, ClientConn.GetMethodConfig,
WithDefaultServiceConfig, WithDisableServiceConfig, WithDisableRetry,
WithMaxCallAttempts, MaxRetryRPCBufferSize, MaxRetryRPCBufferSizeCallOption
and WithDisableHealthCheck. grpc-go reads a service config (JSON, from a
resolver or WithDefaultServiceConfig) with per-method timeouts, message
limits and a retry policy, and retries a call by it with a backoff drawn
from a global random source and the global clock. Xo has no globals: a
client that sleeps and draws random numbers needs a Clock and a Rand.

## Decision

Typed config with grpc-go's retry rules.

- `ServiceConfig { methods: List[MethodConfig], retry_throttling: RetryThrottling? }`,
  `MethodConfig { names: List[MethodName], timeout: Duration?, retry: RetryPolicy?,
  max_request_bytes: Int?, max_response_bytes: Int? }`, `MethodName { service, method }`,
  `RetryPolicy { max_attempts, initial_backoff, max_backoff, backoff_multiplier,
  retryable_codes: Set[Code] }`.
- `service_config_json(Str) -> ServiceConfig ! Str` parses grpc-go's
  JSON (`methodConfig`, `retryThrottling`; durations as "1.5s"; codes by
  name or number) and rejects what grpc-go rejects (a retry policy with
  max_attempts below 2, a backoff not above 0, a multiplier not above 0,
  no retryable codes).
- `Client.with_service_config(cfg, clock, rand)` (WithDefaultServiceConfig;
  the clock and rand are captured at the call, their effects counted
  there as decision 0703's interceptors), `with_disable_retry()`,
  `with_max_call_attempts(n)` (the cap on max_attempts, 5 by default),
  `with_max_retry_buffer(n)` (MaxRetryRPCBufferSize, 256 KiB), and
  `method_config(method) -> MethodConfig?` (GetMethodConfig: the exact
  service/method name, else the service's, else the default entry).
- Retries as grpc-go's: a call is retried when it failed with a
  retryable code before any response header came, the attempts are
  below the cap, the request messages fit the retry buffer, and
  throttling (if configured) has tokens; the wait is a uniform random
  time up to initial_backoff x multiplier^(n-1), capped at max_backoff,
  or the server's `grpc-retry-pushback-ms` (a negative or malformed one
  stops retrying). A per-method timeout applies to a call without one
  of its own. A streaming call is retried only before its first
  response header.
- WithDisableServiceConfig maps to "std/grpc has no resolver that
  supplies one"; WithDisableHealthCheck stays not planned with the
  health checking config of grpc-go's service config.

## Consequences

- A client with retries carries a Clock and a Rand; tests use the fakes
  to make backoff deterministic.
- grpc-go's hedging policy and load balancing config are not parsed
  (balancing waits for target dialing, task #236's unix and dns work).

## Alternatives

- Retry only, without the JSON (option 2), or no retries at all,
  mapped not planned (option 3): the owner chose option 1.
- Global random and clock as grpc-go: not available in Xo.
