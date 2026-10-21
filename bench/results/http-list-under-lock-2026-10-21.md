# GET /users encoded under the read lock (2026-10-21)

Owner's decision (2026-10-21): both CRUD servers (`bench/perf/http/xo`,
`bench/perf/http/go`) encode the GET /users answer while holding the
store's read lock, in id order (Xo: `Store.list_json`, Xo Lang 2's
xo-mutexr idiom; Go: `Store.ListJSON` under `RLock`). Before, GET /users
copied the users out under the lock (Xo `users.values().sort_by(id)`, Go
cloned users and `sort.Slice`) and encoded after releasing it. The
answer's bytes are identical old and new on both servers (checked with
creates, a delete, an update, and `<>&` in names).

Run: `bench/perf/http/listlock-docker.sh` (linux/arm64 in Docker Desktop
on the Mac, 12 CPUs: servers on 0-5, client on 6-11), base 2a0e1a8b
(old) against 62088442 (new), Xo servers built with `--backend=arm64`
and the same std, 1024 connections, 3 interleaved rounds of 10 s,
medians. list: every connection loops GET /users over 1,032 preloaded
users (`bench/perf/http/listload`); mixed: loadgen's CRUD cycle (64
preloaded users, a list every 8th cycle).

| load | server | req/s old | req/s new | p50 old / new | p99 old / new | server CPU per request old / new |
|---|---|---|---|---|---|---|
| list | Go | 5,317 | 5,003 | 30 / 18 ms | 1.175 / 1.388 s | 0.797 / 0.682 ms |
| list | Xo | 5,366 | 13,562 | 170 / 65 ms | 556 / 227 ms | 1.081 / 0.363 ms |
| mixed | Go | 51,159 | 50,755 | 17 / 16 ms | 75 / 76 ms | 39.1 / 37.4 us |
| mixed | Xo | 19,487 | 18,757 | 59 / 64 ms | 174 / 182 ms | 77.0 / 74.6 us |

List CPU per request is the server's CPU seconds (/proc, the whole run)
over the requests listload counted; mixed is loadgen's `cpu_us_per_req`.
Rounds (req/s, low to high): list Go old 5,036 5,317 5,815, new 4,498
5,003 6,301; Xo old 5,327 5,366 5,627, new 13,123 13,562 14,868. Mixed
Go old 41,159 51,159 58,806, new 47,405 50,755 55,107; Xo old 16,158
19,487 19,526, new 15,527 18,757 23,485.

Reading:

- Xo's list-only rate is 2.5x (CPU per request -66%): the copy and sort
  of 1,032 users (a List of User values with their strings) and then the
  encode of the copy were the cost; encoding in place under the read
  lock does one pass and keeps nothing.
- Go's list-only rate is unchanged within its spread (5.0k to 6.3k), CPU
  per request -14%: Go's copy was cheap (a slice of structs), and its
  latency is dominated by the scheduler's unfairness at 1024 connections
  (p99 over a second either way).
- The mixed CRUD load is unchanged within noise for both (a list is one
  request in 40; its writers now wait for an encode under the lock
  instead of a copy).

Raw lines: the run's output (OUT/results.txt) is below.

```
machine: aarch64 12 CPUs; servers on 0-5, client on 6-11; CONC 1024; 3 rounds of 10s
list round 1 go-old: 53174 p50=30ms p99=1.175s p999=1.878s max=2.687s server_cpu_s 42.4
list round 1 go-new: 50027 p50=18ms p99=1.388s p999=2.158s max=3.2s server_cpu_s 34.1
list round 1 xo-old: 53273 p50=178ms p99=465ms p999=646ms max=927ms server_cpu_s 50.4
list round 1 xo-new: 135615 p50=65ms p99=227ms p999=326ms max=500ms server_cpu_s 49.2
list round 2 go-old: 58149 p50=28ms p99=1.195s p999=1.748s max=3.543s server_cpu_s 42.0
list round 2 go-new: 63007 p50=13ms p99=1.088s p999=1.826s max=3.222s server_cpu_s 37.0
list round 2 xo-old: 56271 p50=170ms p99=443ms p999=586ms max=952ms server_cpu_s 50.3
list round 2 xo-new: 148683 p50=60ms p99=201ms p999=286ms max=396ms server_cpu_s 49.5
list round 3 go-old: 50364 p50=29ms p99=1.32s p999=1.81s max=2.779s server_cpu_s 38.8
list round 3 go-new: 44982 p50=21ms p99=1.448s p999=2.062s max=3.503s server_cpu_s 32.4
list round 3 xo-old: 53664 p50=170ms p99=556ms p999=803ms max=1.169s server_cpu_s 58.0
list round 3 xo-new: 131231 p50=66ms p99=238ms p999=329ms max=426ms server_cpu_s 49.1
mixed round 1 go-old: {"requests":511651,"all_requests":668537,"errors":0,"seconds":10.001263295,"rps":51158.63715494754,"p50_us":16519.625,"p99_us":74875.042,"p999_us":99659.375,"peak_rss_kb":67696,"cpu_us_per_req":39.08914474905746} server_cpu_s 27.0
mixed round 1 go-new: {"requests":507557,"all_requests":684661,"errors":0,"seconds":10.000114589,"rps":50755.118402173735,"p50_us":16361,"p99_us":75760.042,"p999_us":132800.792,"peak_rss_kb":69916,"cpu_us_per_req":37.43421921084725} server_cpu_s 24.8
mixed round 1 xo-old: {"requests":195265,"all_requests":253654,"errors":0,"seconds":10.000004046,"rps":19526.492099581294,"p50_us":57523.917,"p99_us":169067.75,"p999_us":214915.125,"peak_rss_kb":94752,"cpu_us_per_req":66.57619132973139} server_cpu_s 18.1
mixed round 1 xo-new: {"requests":187569,"all_requests":257222,"errors":0,"seconds":10.000018963,"rps":18756.86443135798,"p50_us":64271.291,"p99_us":181684.292,"p999_us":231809.25,"peak_rss_kb":94092,"cpu_us_per_req":74.63919944127228} server_cpu_s 17.6
mixed round 2 go-old: {"requests":588099,"all_requests":733129,"errors":0,"seconds":10.000684213,"rps":58805.87642548733,"p50_us":14046.333,"p99_us":66007.792,"p999_us":96570.875,"peak_rss_kb":67504,"cpu_us_per_req":32.307485644423814} server_cpu_s 25.9
mixed round 2 go-new: {"requests":551233,"all_requests":713571,"errors":0,"seconds":10.002988713,"rps":55106.83015003418,"p50_us":15357.583,"p99_us":61971.25,"p999_us":97878.875,"peak_rss_kb":65432,"cpu_us_per_req":32.6540682433744} server_cpu_s 23.7
mixed round 2 xo-old: {"requests":194871,"all_requests":264657,"errors":0,"seconds":10.000133754,"rps":19486.83935572888,"p50_us":58855.5,"p99_us":173616.834,"p999_us":215418.417,"peak_rss_kb":100712,"cpu_us_per_req":76.97399818341364} server_cpu_s 19.6
mixed round 2 xo-new: {"requests":155297,"all_requests":204070,"errors":0,"seconds":10.001809213,"rps":15526.890854721607,"p50_us":62974.125,"p99_us":222181.375,"p999_us":271362.5,"peak_rss_kb":93720,"cpu_us_per_req":96.589116338371} server_cpu_s 20.0
mixed round 3 go-old: {"requests":411593,"all_requests":546442,"errors":0,"seconds":10.000001963,"rps":41159.291920430995,"p50_us":18168.5,"p99_us":123040.876,"p999_us":208405.751,"peak_rss_kb":64884,"cpu_us_per_req":53.45086043737381} server_cpu_s 27.5
mixed round 3 go-new: {"requests":474055,"all_requests":625977,"errors":0,"seconds":10.000087463,"rps":47405.085380901735,"p50_us":16975.791,"p99_us":95404.917,"p999_us":219669,"peak_rss_kb":73888,"cpu_us_per_req":46.408117201590535} server_cpu_s 28.9
mixed round 3 xo-old: {"requests":161582,"all_requests":215645,"errors":0,"seconds":10.000095879,"rps":16158.045078279594,"p50_us":59104.291,"p99_us":245689.75,"p999_us":320553.583,"peak_rss_kb":91344,"cpu_us_per_req":92.83212238987016} server_cpu_s 19.8
mixed round 3 xo-new: {"requests":234853,"all_requests":306584,"errors":0,"seconds":10.00030238,"rps":23484.58987297142,"p50_us":53125.459,"p99_us":137722.041,"p999_us":182138.667,"peak_rss_kb":96352,"cpu_us_per_req":51.09579183574406} server_cpu_s 15.7
```
