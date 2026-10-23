// Node twin of bench/perf/llvm/xo/fib: recursive Fibonacci of 40.
function fib(n: number): number {
  return n < 2 ? n : fib(n - 1) + fib(n - 2);
}
console.log("fib", fib(40));
