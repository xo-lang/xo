# offload jobs outlive their leases

Status: Fixed (2026-11-12, scripts/lead/offload.sh; lead's request after
Lang 3's gates on hosts 1 and 3)

Symptom: a gate leased an offload host's lane and then printed nothing
for 60 to 120 minutes until its lease limit (rc 124), while the same
build takes 89 s on an idle host. Host 2 had memory kills under an
all-slot lease (wasm's go compile "signal: killed", arm64 tests exit 1).
After a TaskStop, `xo dev edges` processes of the stopped gate ran on the
host with PPID 1.

Causes, three:

1. offload.sh runs the job as `heavyd lease ... -- bash offload.sh`, the
   leased command in a process group of its own. When the lease's client
   was killed outright (a TaskStop kills the caller's group, no trap
   runs), the leased offload.sh was reparented to PID 1 and kept its ssh,
   and so its job on the host, running outside any lease. heavyd freed
   the slot; the host ran one more job than its lanes said.
2. On the host, the stop on an ssh drop and the stop of an idle job sent
   TERM only, and nothing stopped what a job left behind when it ended.
   A child that outlives TERM, or an orphan whose parent died, ran on.
3. The idle stop counted a new process as progress. offload-gate-remote's
   memory sampler starts a `sleep 5` every 5 seconds, so a gate stuck
   before its first step never looked idle.

Fix: the leased offload.sh stops itself, and the run on the host, once
its lease parent is gone. The host wrapper's stop_job sends TERM to the
job's recorded process group and session, waits up to 10 s, then KILL,
never by name; it runs on an ssh drop, on an idle stop, and after every
job, and logs what it stopped in /work/offload.runs/stopped.log. Idle
now means under a second of CPU a minute for the whole session (reaped
children included). A heartbeat a minute goes from the host to the Mac,
which stops a run that sends none for OFF_HB_MIN minutes (default 15)
and says where it stood ("waiting for a slot", "running, N s CPU", or
"no contact"), logged in $XO_LEAD_DIR/notes/offload-stalls.log.

Check: scripts/lead/offload_test.sh (an orphan that ignores TERM after
a stop, after a killed caller, and after a job's end; a silent host; a
polling loop with no CPU).

A job that must leave a service running on a host starts it in a session
of its own (`setsid`), or the stop at the job's end takes it too.
