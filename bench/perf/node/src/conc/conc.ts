// Node twins of bench/perf/conc/xo: `node dist/conc/conc.js <row>` prints the
// same line as the Xo program. Tasks are async functions on the event loop
// (spawn, fanout, pingpong); the rows that run in parallel (mutex, pmap) use
// worker_threads, with a SharedArrayBuffer and Atomics for the mutex. There
// is no rwmutex row: Node has no read write lock, and building one from
// Atomics would measure the lock written for the row, not the platform.
import { Worker, isMainThread, parentPort, workerData } from "node:worker_threads";
import { availableParallelism } from "node:os";

function steps(x: number): number {
  let n = x;
  let c = 0;
  while (n > 1) {
    n = n % 2 === 0 ? n / 2 : 3 * n + 1;
    c++;
  }
  return c;
}

async function spawn(): Promise<void> {
  let count = 0;
  for (let r = 0; r < 200; r++) {
    const ts: Promise<void>[] = [];
    for (let i = 0; i < 1000; i++) {
      ts.push((async () => { count += i; })());
    }
    await Promise.all(ts);
  }
  console.log(`spawn ${count}`);
}

async function fanout(): Promise<void> {
  let total = 0;
  for (let r = 0; r < 100; r++) {
    const res: Promise<number>[] = [];
    for (let i = 0; i < 1000; i++) res.push((async () => i * 2 + r)());
    let acc = 0;
    for (const p of res) acc += await p;
    total += acc;
  }
  console.log(`fanout ${total}`);
}

// An unbuffered channel between two async loops: a send waits for a
// receive and the other way round, as Go's chan int and Xo's Chan[Int].
class Chan<T> {
  private recvq: ((v: T | undefined) => void)[] = [];
  private sendq: { v: T; done: () => void }[] = [];
  private closed = false;
  send(v: T): Promise<void> {
    const r = this.recvq.shift();
    if (r) { r(v); return Promise.resolve(); }
    return new Promise((done) => this.sendq.push({ v, done }));
  }
  recv(): Promise<T | undefined> {
    const s = this.sendq.shift();
    if (s) { s.done(); return Promise.resolve(s.v); }
    if (this.closed) return Promise.resolve(undefined);
    return new Promise((r) => this.recvq.push(r));
  }
  close(): void {
    this.closed = true;
    for (const r of this.recvq.splice(0)) r(undefined);
  }
}

async function pingpong(): Promise<void> {
  const ping = new Chan<number>();
  const pong = new Chan<number>();
  const echo = (async () => {
    for (;;) {
      const v = await ping.recv();
      if (v === undefined) break;
      await pong.send(v + 1);
    }
    pong.close();
  })();
  let v = 0;
  for (let i = 0; i < 100000; i++) {
    await ping.send(v);
    v = (await pong.recv()) as number;
  }
  ping.close();
  await echo;
  console.log(`pingpong ${v}`);
}

// mutex: 8 workers, 25000 lock, increment, unlock each, on shared memory.
const MUTEX_WORKERS = 8;
function lock(m: Int32Array): void {
  // 0 free, 1 locked, 2 locked with waiters (Drepper's futex mutex)
  let c = Atomics.compareExchange(m, 0, 0, 1);
  if (c !== 0) {
    if (c !== 2) c = Atomics.exchange(m, 0, 2);
    while (c !== 0) {
      Atomics.wait(m, 0, 2);
      c = Atomics.exchange(m, 0, 2);
    }
  }
}
function unlock(m: Int32Array): void {
  if (Atomics.sub(m, 0, 1) !== 1) {
    Atomics.store(m, 0, 0);
    Atomics.notify(m, 0, 1);
  }
}

function runWorkers(task: string, n: number, shared: SharedArrayBuffer, extra: unknown[]): Promise<unknown[]> {
  return Promise.all(
    Array.from({ length: n }, (_, k) => new Promise<unknown>((resolve, reject) => {
      const w = new Worker(__filename, { workerData: { task, k, shared, extra } });
      w.once("message", resolve);
      w.once("error", reject);
    })),
  );
}

async function mutex(): Promise<void> {
  const shared = new SharedArrayBuffer(8);
  await runWorkers("mutex", MUTEX_WORKERS, shared, []);
  console.log(`mutex ${new Int32Array(shared)[1]}`);
}

// pmap: 20 rounds of 64 chunks of 200000 Collatz step counts, spread over
// one worker per CPU; the chunks go out as one message per worker per round.
async function pmap(): Promise<void> {
  const size = 200000;
  const chunks = 64;
  const nw = availableParallelism();
  const workers = Array.from({ length: nw }, () => new Worker(__filename, { workerData: { task: "pmap" } }));
  let total = 0;
  for (let r = 0; r < 20; r++) {
    const parts = await Promise.all(
      workers.map((w, wi) => new Promise<number>((resolve) => {
        w.once("message", resolve);
        w.postMessage({ r, from: Math.floor((wi * chunks) / nw), to: Math.floor(((wi + 1) * chunks) / nw), size, chunks });
      })),
    );
    for (const p of parts) total += p;
  }
  for (const w of workers) await w.terminate();
  console.log(`pmap ${total}`);
}

if (isMainThread) {
  const row = process.argv[2] ?? "";
  const rows: Record<string, () => Promise<void>> = { spawn, fanout, pingpong, mutex, pmap };
  const f = rows[row];
  if (!f) {
    console.error("usage: conc spawn|fanout|pingpong|mutex|pmap");
    process.exit(2);
  }
  f();
} else if (workerData.task === "mutex") {
  const m = new Int32Array(workerData.shared);
  const c = new Int32Array(workerData.shared, 4);
  for (let j = 0; j < 25000; j++) {
    lock(m);
    c[0]++;
    unlock(m);
  }
  parentPort!.postMessage(0);
} else if (workerData.task === "pmap") {
  parentPort!.on("message", (m: { r: number; from: number; to: number; size: number; chunks: number }) => {
    let acc = 0;
    for (let k = m.from; k < m.to; k++) {
      for (let i = Math.floor((k * m.size) / m.chunks); i < Math.floor(((k + 1) * m.size) / m.chunks); i++) acc += steps(i + m.r);
    }
    parentPort!.postMessage(acc);
  });
}
