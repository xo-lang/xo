"""Python twin of bench/perf/llvm/xo/fib: recursive Fibonacci of 40."""


def fib(n):
    return n if n < 2 else fib(n - 1) + fib(n - 2)


if __name__ == "__main__":
    import sys

    # REPEAT, the second argument (default 1; task #687, as the Xo and Go twins since #686):
    # fib runs that many times and every run must give the first run's line.
    reps = int(sys.argv[2]) if len(sys.argv) > 2 else 1
    first = f"fib {fib(40)}"
    for _ in range(1, reps):
        if f"fib {fib(40)}" != first:
            print("repeat: a run printed something else")
            sys.exit(1)
    print(first)
