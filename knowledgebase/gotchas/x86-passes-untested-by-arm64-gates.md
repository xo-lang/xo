# The x86_64 MIR passes are not run by -short or the fixed point

Status: Observed (2026-10-20, Xo lang 4)

Symptom: a change to x86_sink_loads passed -short and the fixed point
(scripts/gate.sh short fp), then every amd64 build of bench/perf/rsa spun
at 100% CPU for 29 minutes in the heavy slot until the lead noticed.

Cause: the self build and the fixed point compile for the host, arm64;
-short builds amd64 for no program, so a pass that runs only for x86_64
(x86_*.xo, the `if g.x86` branch of isel) is first exercised by the Docker
goldens or a cross build. The pass itself moved two loads read by one
instruction past each other forever (each sank below the other).

Fix: internal/arm64 TestAmd64PassesFinish (-short) cross assembles
testdata/x86sink for linux/amd64 with a 180 second limit; a pass over a
block's instructions walks them in an order that makes progress (last to
first for a pass that moves instructions down) and bounds its scan. For a
change to an x86 pass, also cross build one real program
(`xo build --backend=amd64 --targets=linux/amd64`) before gating.
