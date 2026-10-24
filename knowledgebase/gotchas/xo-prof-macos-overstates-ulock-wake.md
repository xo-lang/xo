# XO_PROF on macOS puts 31% of CPU in _ulock_wake that the call does not cost

Status: Observed 2026-10-24.

Symptom: `XO_PROF=cpu XO_PROF_HZ=1000` of the rwrep program (64 tasks, a
contended readers and writer lock, 20 rounds; 680 ms wall, 1,334 samples)
showed `_ulock_wake` as the top flat frame (416 ms, 31%), reached from
pool.futex_wake (callers go_idle, push, unlock, park_signal).

Cause: not a cost of the wake. Timing pool.futex_wake inside the runtime
(sysio.mono_ns around the call, summed into a stat counter) gave 2.4 to 3.0
thousand wakes and 2.0 to 2.5 ms in total in three runs of the same program
(about 0.8 us a call; one call in a run over 50 us). A C ping-pong of
__ulock_wake measured 0.6 to 1.0 us a call (max 0.3 to 0.5 ms), a pthread
condition signal 1.4 to 2 us. The ITIMER_PROF signal evidently lands in the
syscall's tail more than its CPU time warrants (the profile is the
signal's, by design not a trace).

Fix or workaround: before acting on a syscall frame at the top of a macOS
profile, time the call itself in the runtime (a pair of mono_ns reads and a
stat add, as above) or compare with `xo bench --ab`; the counts in
XO_RTSTATS (futex_wake) say how often it is called. The same profile's
other top frame, clock reads in pool.find_work's grace wait (34%), is real:
that loop is a time-bounded spin of GRACE_NS (20 us), so it burns CPU by
design (it is off the critical path).
