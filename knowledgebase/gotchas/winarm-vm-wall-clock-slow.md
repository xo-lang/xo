# The winarm VM's wall clock runs about 10% slow against QPC

Status: Observed (2026-11-21, task #605).

## Symptom

On the windows/arm64 VM (VMware Fusion, offload-winarm.env), the golden
http_timeout_handler fails on every run: "slow returned near its limit:
false". A probe of `clock.sleep(300ms)` measured with `Clock.now` reads 270
to 285 ms.

## Cause

The guest's wall clock (GetSystemTimePreciseAsFileTime, what `Clock.now`
reads) runs at 0.902 times its QueryPerformanceCounter (frequency 24 MHz),
which xo's timers use. .NET shows the same ratio: a 3 s
`Thread.Sleep` is 3016 to 3040 ms on `Stopwatch` and 2722 to 2743 ms on
`DateTime.UtcNow`. The product is not at fault: a sleep lasts its full
length on the monotonic clock.

## Fix

Read wall-clock elapsed-time checks on this VM as environment failures.
To check the VM before blaming xo, compare a PowerShell `Stopwatch`
against `DateTime.UtcNow` over a 3 s sleep: a ratio far from 1.0 means the
guest clock. Whether native Windows on ARM hardware shows it is unverified.
