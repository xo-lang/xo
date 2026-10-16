// Hand written Rust versions of the LLVM backend benchmarks in ../xo, the
// same algorithms and sizes as ../go/main.go, as idiomatic Rust: std only,
// i64 arithmetic (a release build does not check overflow, like Go),
// Vec growth by push. Usage: llperf <workload>; each prints the checksum
// line of the Xo program.
use std::env;
use std::process;

fn fib(n: i64) -> i64 {
    if n < 2 {
        n
    } else {
        fib(n - 1) + fib(n - 2)
    }
}

fn sieve(n: usize) -> i64 {
    let mut flags = Vec::new();
    for _ in 0..n + 1 {
        flags.push(true);
    }
    let mut count = 0;
    for i in 2..n + 1 {
        if flags[i] {
            count += 1;
            let mut j = i * i;
            while j <= n {
                flags[j] = false;
                j += i;
            }
        }
    }
    count
}

#[derive(Clone, Copy)]
struct Body {
    x: f64,
    y: f64,
    z: f64,
    vx: f64,
    vy: f64,
    vz: f64,
    mass: f64,
}

const PI: f64 = 3.141592653589793;
const DAYS: f64 = 365.24;

fn solar_mass() -> f64 {
    4.0 * PI * PI
}

fn body(x: f64, y: f64, z: f64, vx: f64, vy: f64, vz: f64, mass: f64) -> Body {
    Body { x, y, z, vx, vy, vz, mass }
}

fn bodies() -> Vec<Body> {
    let sm = solar_mass();
    vec![
        body(0.0, 0.0, 0.0, 0.0, 0.0, 0.0, sm),
        body(4.84143144246472090e+00, -1.16032004402742839e+00, -1.03622044471123109e-01, 1.66007664274403694e-03 * DAYS, 7.69901118419740425e-03 * DAYS, -6.90460016972063023e-05 * DAYS, 9.54791938424326609e-04 * sm),
        body(8.34336671824457987e+00, 4.12479856412430479e+00, -4.03523417114321381e-01, -2.76742510726862411e-03 * DAYS, 4.99852801234917238e-03 * DAYS, 2.30417297573763929e-05 * DAYS, 2.85885980666130812e-04 * sm),
        body(1.28943695621391310e+01, -1.51111514016986312e+01, -2.23307578892655734e-01, 2.96460137564761618e-03 * DAYS, 2.37847173959480950e-03 * DAYS, -2.96589568540237556e-05 * DAYS, 4.36624404335156298e-05 * sm),
        body(1.53796971148509165e+01, -2.59193146099879641e+01, 1.79258772950371181e-01, 2.68067772490389322e-03 * DAYS, 1.62824170038242295e-03 * DAYS, -9.51592254519715870e-05 * DAYS, 5.15138902046611451e-05 * sm),
    ]
}

fn offset(bs: &mut [Body]) {
    let (mut px, mut py, mut pz) = (0.0, 0.0, 0.0);
    for b in bs.iter() {
        px += b.vx * b.mass;
        py += b.vy * b.mass;
        pz += b.vz * b.mass;
    }
    let sm = solar_mass();
    bs[0].vx = 0.0 - px / sm;
    bs[0].vy = 0.0 - py / sm;
    bs[0].vz = 0.0 - pz / sm;
}

fn energy(bs: &[Body]) -> f64 {
    let mut e = 0.0;
    for i in 0..bs.len() {
        let b = bs[i];
        e += 0.5 * b.mass * (b.vx * b.vx + b.vy * b.vy + b.vz * b.vz);
        for c in &bs[i + 1..] {
            let (dx, dy, dz) = (b.x - c.x, b.y - c.y, b.z - c.z);
            e -= b.mass * c.mass / (dx * dx + dy * dy + dz * dz).sqrt();
        }
    }
    e
}

fn advance(bs: &mut [Body], dt: f64) {
    let n = bs.len();
    for i in 0..n {
        for j in i + 1..n {
            let dx = bs[i].x - bs[j].x;
            let dy = bs[i].y - bs[j].y;
            let dz = bs[i].z - bs[j].z;
            let d2 = dx * dx + dy * dy + dz * dz;
            let mag = dt / (d2 * d2.sqrt());
            let mi = bs[i].mass * mag;
            let mj = bs[j].mass * mag;
            bs[i].vx -= dx * mj;
            bs[i].vy -= dy * mj;
            bs[i].vz -= dz * mj;
            bs[j].vx += dx * mi;
            bs[j].vy += dy * mi;
            bs[j].vz += dz * mi;
        }
    }
    for b in bs.iter_mut() {
        b.x += dt * b.vx;
        b.y += dt * b.vy;
        b.z += dt * b.vz;
    }
}

#[derive(Clone, Copy)]
struct Rec {
    key: i64,
    id: i64,
    #[allow(dead_code)]
    weight: f64,
}

fn less(a: &Rec, b: &Rec) -> bool {
    if a.key != b.key {
        a.key < b.key
    } else {
        a.id < b.id
    }
}

fn quicksort(xs: &mut [Rec], lo: i64, hi: i64) {
    if lo >= hi {
        return;
    }
    let pivot = xs[(lo + (hi - lo) / 2) as usize];
    let (mut i, mut j) = (lo, hi);
    while i <= j {
        while less(&xs[i as usize], &pivot) {
            i += 1;
        }
        while less(&pivot, &xs[j as usize]) {
            j -= 1;
        }
        if i <= j {
            xs.swap(i as usize, j as usize);
            i += 1;
            j -= 1;
        }
    }
    quicksort(xs, lo, j);
    quicksort(xs, i, hi);
}

fn sort_workload() -> i64 {
    let mut sum = 0;
    let mut seed: i64 = 42;
    for round in 0..3 {
        let mut xs = Vec::new();
        for i in 0..1_000_000 {
            seed = (seed * 1103515245 + 12345) % 2147483648;
            xs.push(Rec { key: seed % 100000, id: i, weight: (i % 97) as f64 * 0.5 });
        }
        let hi = xs.len() as i64 - 1;
        quicksort(&mut xs, 0, hi);
        let mut ok = 1;
        for i in 1..xs.len() {
            if less(&xs[i], &xs[i - 1]) {
                ok = 0;
            }
        }
        sum += xs[0].key + xs[500000].id + xs[999999].key + ok * (round + 1);
    }
    sum
}

#[derive(Clone, Copy)]
enum Op {
    Push(i64),
    Load(usize),
    Store(usize),
    Add,
    Sub,
    Mul,
    Mod,
    JumpIfZero(usize),
    // in the Go and Xo instruction sets too; the program has no Jump
    #[allow(dead_code)]
    Jump(usize),
    Halt,
}

fn run(prog: &[Op], n: i64) -> i64 {
    let mut stack: Vec<i64> = Vec::new();
    let mut slots = [0i64; 4];
    slots[0] = n;
    let mut pc = 0;
    let mut steps: i64 = 0;
    loop {
        steps += 1;
        match prog[pc] {
            Op::Push(v) => stack.push(v),
            Op::Load(s) => stack.push(slots[s]),
            Op::Store(s) => slots[s] = stack.pop().unwrap_or(0),
            Op::Add => {
                let b = stack.pop().unwrap_or(0);
                let a = stack.pop().unwrap_or(0);
                stack.push(a + b);
            }
            Op::Sub => {
                let b = stack.pop().unwrap_or(0);
                let a = stack.pop().unwrap_or(0);
                stack.push(a - b);
            }
            Op::Mul => {
                let b = stack.pop().unwrap_or(0);
                let a = stack.pop().unwrap_or(0);
                stack.push(a * b);
            }
            Op::Mod => {
                let b = stack.pop().unwrap_or(1);
                let a = stack.pop().unwrap_or(0);
                stack.push(a % b);
            }
            Op::JumpIfZero(t) => {
                if stack.pop().unwrap_or(0) == 0 {
                    pc = t.wrapping_sub(1);
                }
            }
            Op::Jump(t) => pc = t.wrapping_sub(1),
            Op::Halt => break,
        }
        pc = pc.wrapping_add(1);
    }
    slots[1] * 1000 + steps % 1000
}

fn interp_prog() -> Vec<Op> {
    use Op::*;
    vec![
        Load(0), JumpIfZero(19),
        Load(1), Load(0), Load(0), Mul, Add, Push(1000003), Mod, Store(1),
        Load(0), Push(1), Sub, Store(0),
        Push(0), JumpIfZero(0),
        Halt, Halt, Halt, Halt,
    ]
}

fn collatz_steps(start: i64) -> i64 {
    let (mut n, mut s) = (start, 0);
    while n != 1 {
        n = if n % 2 == 0 { n / 2 } else { 3 * n + 1 };
        s += 1;
    }
    s
}

// strings: format! for the words, a String the accumulator appends to
// (Go's version uses a strings.Builder; Xo's `acc = acc + w` appends in
// place).
fn strings_workload() -> i64 {
    let mut total = 0;
    for round in 0..10 {
        let mut words = Vec::new();
        for i in 0..200000 {
            words.push(format!("w{}-{}", i % 1000, round));
        }
        let mut acc = String::new();
        let mut k = 0;
        while let Some(w) = words.pop() {
            if k % 10 == 0 {
                acc.push_str(&w);
            }
            k += 1;
            total += w.len() as i64;
        }
        total += acc.len() as i64;
    }
    total
}

fn main() {
    let w = env::args().nth(1).unwrap_or_else(|| "fib".to_string());
    match w.as_str() {
        "fib" => println!("fib {}", fib(40)),
        "sieve" => {
            let total: i64 = (0..5).map(|_| sieve(10_000_000)).sum();
            println!("sieve {}", total);
        }
        "nbody" => {
            let mut bs = bodies();
            offset(&mut bs);
            let before = energy(&bs);
            for _ in 0..5_000_000 {
                advance(&mut bs, 0.01);
            }
            let after = energy(&bs);
            println!("nbody {} {}", (before * 1e9) as i64, (after * 1e9) as i64);
        }
        "sort" => println!("sort {}", sort_workload()),
        "interp" => println!("interp {}", run(&interp_prog(), 3_000_000)),
        "collatz" => {
            let (mut total, mut best) = (0, 0);
            for i in 1..3_000_000 {
                let s = collatz_steps(i);
                total += s;
                best = best.max(s);
            }
            println!("collatz {} {}", total, best);
        }
        "strings" => println!("strings {}", strings_workload()),
        _ => {
            eprintln!("unknown workload {}", w);
            process::exit(2);
        }
    }
}
