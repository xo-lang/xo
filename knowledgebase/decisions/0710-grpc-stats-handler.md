# 0710: std/grpc stats handlers (per-message and connection events)

Status: Proposed (xo-4c, 2026-11-17)

## Context

grpc-go's `stats.Handler` (`grpc.StatsHandler`, `grpc.WithStatsHandler`) sees each call begin and end, every
header, payload and trailer, and each connection open and close. std/grpc has interceptors (decision 0703),
`with_on_finish` and message interceptors (decision 0709), which cover a call's begin and end and its messages,
but no events with sizes and no connection events (map.txt rows grpc.StatsHandler and WithStatsHandler are
partial; task #236).

## Decision

To be written: a `Stats` event enum after grpc-go's stats package and a handler on Client and Server.

## Consequences

To be written.

## Alternatives

To be written.
