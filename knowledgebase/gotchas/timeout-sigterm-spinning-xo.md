# `timeout N prog` never ends a spinning Xo program

Status: Observed (2026-10-21)

## Symptom

A gate's check that a program hangs (`XO_PREEMPT=0 timeout 10 ./prog`,
expected exit 124) ran for over two hours in a Docker container: the
program at 99% CPU, `timeout` still waiting.

## Cause

`timeout` sends SIGTERM, and Xo's runtime turns SIGTERM into a request
for two-phase shutdown (std/native/llrt stop.xo) that tasks see at their
next poll or runtime call. A task spinning in a loop with preemption off
(XO_PREEMPT=0) or in runtime code that does not poll never sees it, so
the process keeps running. `timeout` waits for the child without a
second signal unless told to.

## Fix

Use `timeout -s KILL N` (exit 137) for a check that expects a hang, or
`timeout -k 5 N` to follow the TERM with a KILL. Any harness that stops
an Xo program with SIGTERM needs the same fallback.
