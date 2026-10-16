// The Rust versions of the Xo programs in ../xo, as ../go/main.go:
// `concperf <name>` prints the same line. Idiomatic Rust: lightweight
// tasks on tokio's multi-threaded runtime (one worker per CPU, as Go's
// scheduler and Xo's pool) for spawn, fanout and pingpong; OS threads in
// std::thread::scope with std's Mutex and RwLock for the lock workloads
// and pmap, as a Rust programmer would write CPU bound work over a few
// dozen workers.
use std::env;
use std::process;
use std::sync::atomic::{AtomicI64, Ordering};
use std::sync::{Arc, Mutex, RwLock};
use std::thread;

// tokio's multi-threaded runtime, or with CONCPERF_RT=current its
// current-thread runtime (run.sh's rust-ct: every task on one thread,
// as Xo's XO_THREADS=1)
fn rt() -> tokio::runtime::Runtime {
    if env::var("CONCPERF_RT").as_deref() == Ok("current") {
        tokio::runtime::Builder::new_current_thread().build().unwrap()
    } else {
        tokio::runtime::Builder::new_multi_thread().build().unwrap()
    }
}

fn spawn() {
    let count = Arc::new(AtomicI64::new(0));
    rt().block_on(async {
        for _ in 0..200 {
            let mut hs = Vec::with_capacity(1000);
            for i in 0..1000i64 {
                let c = Arc::clone(&count);
                hs.push(tokio::spawn(async move {
                    c.fetch_add(i, Ordering::Relaxed);
                }));
            }
            for h in hs {
                h.await.unwrap();
            }
        }
    });
    println!("spawn {}", count.load(Ordering::Relaxed));
}

fn fanout() {
    let total = rt().block_on(async {
        let mut total = 0i64;
        for r in 0..100i64 {
            let mut res = Vec::with_capacity(1000);
            for i in 0..1000i64 {
                let (tx, rx) = tokio::sync::oneshot::channel();
                res.push(rx);
                tokio::spawn(async move {
                    let _ = tx.send(i * 2 + r);
                });
            }
            let mut acc = 0;
            for rx in res {
                acc += rx.await.unwrap();
            }
            total += acc;
        }
        total
    });
    println!("fanout {}", total);
}

fn pingpong() {
    let v = rt().block_on(async {
        let (ping_tx, mut ping_rx) = tokio::sync::mpsc::channel::<i64>(1);
        let (pong_tx, mut pong_rx) = tokio::sync::mpsc::channel::<i64>(1);
        tokio::spawn(async move {
            while let Some(v) = ping_rx.recv().await {
                if pong_tx.send(v + 1).await.is_err() {
                    break;
                }
            }
        });
        let mut v = 0;
        for _ in 0..100000 {
            ping_tx.send(v).await.unwrap();
            v = pong_rx.recv().await.unwrap();
        }
        v
    });
    println!("pingpong {}", v);
}

fn mutex() {
    let c = Mutex::new(0i64);
    thread::scope(|s| {
        for _ in 0..8 {
            s.spawn(|| {
                for _ in 0..25000 {
                    *c.lock().unwrap() += 1;
                }
            });
        }
    });
    println!("mutex {}", c.into_inner().unwrap());
}

// rwmutex: 64 threads, 4,000 accesses each, one in four a write that
// holds the lock for a Collatz step count, the others reads.
fn rwmutex() {
    let c = RwLock::new(0i64);
    let odd = Mutex::new(0i64);
    thread::scope(|s| {
        for t in 0..64i64 {
            let (c, odd) = (&c, &odd);
            s.spawn(move || {
                for i in 0..4000i64 {
                    if (i + t) % 4 == 0 {
                        let mut g = c.write().unwrap();
                        *g += 1 + steps(27 + *g % 2) / 1000;
                    } else {
                        let v = *c.read().unwrap();
                        if v < 0 {
                            *odd.lock().unwrap() += 1;
                        }
                    }
                }
            });
        }
    });
    println!("rwmutex {} {}", c.into_inner().unwrap(), odd.into_inner().unwrap());
}

fn steps(x: i64) -> i64 {
    let (mut n, mut c) = (x, 0);
    while n > 1 {
        n = if n % 2 == 0 { n / 2 } else { 3 * n + 1 };
        c += 1;
    }
    c
}

fn pmap() {
    const SIZE: i64 = 200000;
    const CHUNKS: i64 = 64;
    let mut total = 0i64;
    for r in 0..20i64 {
        let out: Vec<i64> = thread::scope(|s| {
            let hs: Vec<_> = (0..CHUNKS)
                .map(|k| s.spawn(move || (k * SIZE / CHUNKS..(k + 1) * SIZE / CHUNKS).map(|i| steps(i + r)).sum::<i64>()))
                .collect();
            hs.into_iter().map(|h| h.join().unwrap()).collect()
        });
        total += out.iter().sum::<i64>();
    }
    println!("pmap {}", total);
}

fn main() {
    let w = env::args().nth(1).unwrap_or_default();
    match w.as_str() {
        "spawn" => spawn(),
        "fanout" => fanout(),
        "pingpong" => pingpong(),
        "mutex" => mutex(),
        "rwmutex" => rwmutex(),
        "pmap" => pmap(),
        _ => {
            eprintln!("usage: concperf spawn|fanout|pingpong|mutex|rwmutex|pmap");
            process::exit(2);
        }
    }
}
