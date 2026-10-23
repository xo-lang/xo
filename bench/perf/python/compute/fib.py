"""Python twin of bench/perf/llvm/xo/fib: recursive Fibonacci of 40."""


def fib(n):
    return n if n < 2 else fib(n - 1) + fib(n - 2)


print("fib", fib(40))
