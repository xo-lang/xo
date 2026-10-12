# The remote-full EC2 instance outlived both of its timers

Status: Observed (2026-10-12), fixed in scripts/remote-full.sh.

Symptom: the first real run of scripts/remote-full.sh left its c8g.8xlarge
spot instance running about 5.5 hours, past the 300 minute limit, until
the lead terminated it by hand (cost about $2).

Cause: two nets, both missed. The user-data `shutdown -h +300` did not
power the instance off on Amazon Linux 2023, and the script's EXIT trap
never ran because the script was still blocked in ssh (the test process
on the instance outlived its own `go test -timeout 240m`).

Fix: user-data also schedules `systemd-run --on-active=<N>m poweroff`;
the script starts a local watchdog (captured PID) that terminates the
instance after the limit whatever the script is doing; ssh gives up after
five missed keepalives. After any run, check that nothing tagged
purpose=xo-remote-full is still running.
