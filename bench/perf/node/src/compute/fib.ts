// Node twin of bench/perf/llvm/xo/fib: recursive Fibonacci of 40.
function fib(n: number): number {
  return n < 2 ? n : fib(n - 1) + fib(n - 2);
}
// REPEAT, the second argument (default 1; task #687, as the Xo and Go twins since #686):
// fib runs that many times and every run must give the first run's line.
const reps = Number.parseInt(process.argv[3] ?? "1", 10) || 1;
const work = (): string => `fib ${fib(40)}`;
const first = work();
for (let i = 1; i < reps; i++) {
  if (work() !== first) {
    console.log("repeat: a run printed something else");
    process.exit(1);
  }
}
console.log(first);
