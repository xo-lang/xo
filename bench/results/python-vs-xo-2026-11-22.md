# Python against Xo: 2026-11-22

Written by bench/perf/python/run.sh (twins/lib.sh). Ratios are Xo over Python (below 1: Xo is faster or smaller).

## Machine and versions

- Linux 7.0.14-linuxkit aarch64, , 12 CPUs, load average at start 2.08
- xo 0.55.0 commit 9599fc7d key ff3b0011426efd26, NOT THIS CHECKOUT (its key is 13809f7545321ed6)
- go version go1.26.11 linux/arm64
- Python 3.13.5 (virtual environment of bench/perf/python from /usr/bin/python3.13)
- packages (requirements.txt, hashes): httptools==0.9.0 starlette==0.48.0 uvicorn==0.37.0 uvloop==0.23.0 

Run on the linux/arm64 timing host (host 1) in two leases of at most 30
minutes (compute and json, then concurrency; the load line of each table
is its start), with the published xo 0.55.0 release binary against the
tree of task #687 (hence "NOT THIS CHECKOUT": the Xo programs are built
by 0.55.0 with its own std). Every program runs its row REPEAT times in
one process. The host's python3.13 has no venv package (ensurepip), so
the virtual environment was made with `--without-pip` and the packages
above were not installed: the compute, concurrency and JSON programs use
only the standard library. The HTTP rows, which need them, were not
rerun; the 2026-10-23 file still holds them. The pmap row runs with
--max-load (its own processes lift the load over xo bench's default
limit).

## Compute (recursive fib 40, 3 rounds)

| fib, repeat 5 | load 1.99 |
    wall_ms               38557       1105        0.026% 0.114%     0.029   0.029 to 0.029    0.029 0.029     better (n=4)
    cpu_ms                38555       1105        0.027% 0.113%     0.029   0.029 to 0.029    0.029 0.029     better (n=4)
    max_rss_kb            7848        964.0       0% 0%             0.123   0.123 to 0.123    0.123 0.123     better (n=4)
    instructions, cycles, ipc: n/a (only macOS time -l reports them)

## Concurrency (asyncio, threading, multiprocessing; no rwmutex row: the standard library has no read write lock)

| spawn, repeat 58 | load 1.62 |
    wall_ms               20433       928.8       0.516% 1.245%     0.045   0.045 to 0.046    0.045 0.045     better (n=10)
    cpu_ms                20432       944.2       0.521% 1.297%     0.046   0.046 to 0.047    0.046 0.046     better (n=10)
    max_rss_kb            21132       3612        0% 16.83%         0.171   0.144 to 0.183    0.153 0.177     better (n=10)
    instructions, cycles, ipc: n/a (only macOS time -l reports them)
| fanout, repeat 70 | load 1.01 |
    wall_ms               8150        902.3       0.643% 0.259%     0.111   0.11 to 0.111     0.111 0.111     better (n=10)
    cpu_ms                8149        908.0       0.643% 0.445%     0.111   0.111 to 0.112    0.112 0.111     better (n=10)
    max_rss_kb            20740       3164        0.617% 7.08%      0.153   0.147 to 0.165    0.162 0.149     better (n=10)
    instructions, cycles, ipc: n/a (only macOS time -l reports them)
| pingpong, repeat 65 | load 1.06 |
    wall_ms               21081       940.6       0.63% 1.14%       0.045   0.044 to 0.047    0.045 0.045     better (n=4)
    cpu_ms                21080       942.7       0.627% 1.297%     0.045   0.044 to 0.047    0.045 0.045     better (n=4)
    max_rss_kb            19098       932.0       0.728% 0%         0.049   0.048 to 0.049    0.049 0.049     better (n=4)
    instructions, cycles, ipc: n/a (only macOS time -l reports them)
| mutex, repeat 278 | load 1.01 |
    wall_ms               3902        1174        0.354% 1.778%     0.301   0.296 to 0.304    0.3 0.303       better (n=10)
    cpu_ms                3913        2063        0.361% 2.591%     0.527   0.518 to 0.534    0.527 0.529     better (n=10)
    max_rss_kb            19084       1316        0.021% 9.726%     0.069   0.062 to 0.069    0.069 0.069     better (n=10)
    instructions, cycles, ipc: n/a (only macOS time -l reports them)
| pmap, repeat 19 | load 2.24, max load 15 |
    wall_ms               53233       1074        0.834% 1.761%     0.02    0.02 to 0.021     0.02 0.02       better (n=4)
    cpu_ms                550718      11165       0.786% 0.714%     0.02    0.02 to 0.021     0.02 0.021      better (n=4)
    max_rss_kb            20508       2460        0.239% 2.602%     0.12    0.114 to 0.126    0.123 0.117     better (n=4)
    instructions, cycles, ipc: n/a (only macOS time -l reports them)

## JSON (json.loads and json.dumps against std/json; 1000 users, 100 rounds)

| json decode, Python, repeat 45 | load 1.03 |
    wall_ms               3927        1301        2.63% 1.474%      0.331   0.326 to 0.337    0.328 0.332     better (n=10)
    cpu_ms                3926        1301        2.63% 1.472%      0.331   0.326 to 0.337    0.328 0.332     better (n=10)
    max_rss_kb            10512       1544        0.086% 0%         0.147   0.147 to 0.147    0.147 0.147     better (n=10)
    instructions, cycles, ipc: n/a (only macOS time -l reports them)
| json decode, Go, repeat 45 | load 1.01 |
    wall_ms               4730        1311        2.637% 0.642%     0.277   0.27 to 0.283     0.279 0.278     better (n=10)
    cpu_ms                4861        1311        2.593% 0.675%     0.27    0.264 to 0.276    0.272 0.27      better (n=10)
    max_rss_kb            11132       1544        2.515% 0%         0.139   0.136 to 0.141    0.141 0.137     better (n=10)
    instructions, cycles, ipc: n/a (only macOS time -l reports them)
| json encode, Python, repeat 85 | load 1.18 |
    wall_ms               7972        1095        2.306% 1.041%     0.137   0.135 to 0.139    0.138 0.136     better (n=10)
    cpu_ms                7971        1095        2.308% 1.042%     0.137   0.135 to 0.139    0.138 0.136     better (n=10)
    max_rss_kb            10384       1416        0.029% 0%         0.136   0.136 to 0.136    0.136 0.136     better (n=10)
    instructions, cycles, ipc: n/a (only macOS time -l reports them)
| json encode, Go, repeat 85 | load 1.03 |
    wall_ms               1997        1091        3.519% 0.78%      0.546   0.533 to 0.559    0.546 0.536     better (n=10)
    cpu_ms                2114        1091        3.257% 0.767%     0.516   0.506 to 0.529    0.516 0.511     better (n=10)
    max_rss_kb            10984       1416        3.833% 0%         0.129   0.127 to 0.132    0.129 0.129     better (n=10)
    instructions, cycles, ipc: n/a (only macOS time -l reports them)
