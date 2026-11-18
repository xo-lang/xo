# A wait inside task.with_timeout reported as a deadlock (XO-F001)

Observed 2026-11-18 (std/grpc fuzz, std/websocket fuzz: an intermittent F001).

## Symptom

A program whose tasks all wait on a channel, mutex or `await` inside
`task.with_timeout` faults XO-F001 (deadlock) at once, though the deadline
would end every wait. Four tasks each looping `with_timeout(1ms)` around a
`recv` on a channel nobody sends to faulted on every run. The grpc fuzz
client (call deadlines are `task.with_timeout`, std/grpc.xo) faulted after a
few thousand calls, about one run in 270.

## Cause

The native pool's deadlock check (std/native/llrt/pool.xo `dl_check`)
counts channel, mutex and join waits, and leaves out net waits, sleeps and
`select` with `after` through `DLK_UNCOUNTED`. The deadline of
`with_timeout` is a poll timer (`FN_SCOPE`) that cancels the scope when it
fires, and nothing told the check that such a timer was pending, so the
counted waits it would end looked final. Spec core 11.4: waits a timer will
end do not count.

## Fix

(private commit): `poll.timer_fn` adds one to `DLK_UNCOUNTED` for an `FN_SCOPE`
timer and keeps the pool's record in the timer (`T_HOLD`); the fire (after
its cancel) or `timer_del` releases it once. Golden
internal/llvm/testdata/golden/deadline_chan_wait. A new kind of timer that
ends waits needs the same hold.

Still open: the native runtime faults at once, where spec 11.4 says real
runs report after 200 ms with no progress.
