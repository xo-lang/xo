# 0704: a heap record gives back the magazines of size classes it has not used for a period

Status: Proposed (xo-lang-1, 2026-11-17, task #213, lead decision #71)

## Context

Stub; filled in with the measurements (sse, bufio and otlp stress RSS, host 1 --ab of json, map and http).
std/sse's 30 minute stress keeps about 47 MB on free lists on both backends, about 1 MB in each of about 40
size classes: each of 18 heap records keeps up to two 64 KiB magazines a class and never returns them.

## Decision

## Consequences

## Alternatives
