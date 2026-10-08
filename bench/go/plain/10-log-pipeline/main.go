package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdin, os.Stderr))
}

func run(args []string, stdin io.Reader, stderr io.Writer) int {
	fs := flag.NewFlagSet("logpipe", flag.ContinueOnError)
	fs.SetOutput(stderr)
	workers := fs.Int("workers", 4, "number of parser workers")
	flush := fs.Duration("flush", 10*time.Second, "flush interval")
	out := fs.String("out", "out", "output directory for flush files")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *workers < 1 || *flush <= 0 || *out == "" {
		fmt.Fprintln(stderr, "invalid flags: workers >= 1, flush > 0, out non empty")
		return 2
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	st, err := Run(ctx, stdin, Config{Workers: *workers, Flush: *flush, OutDir: *out})
	fmt.Fprintf(stderr, "lines read: %d, parsed: %d, malformed: %d, flushes written: %d\n",
		st.Read, st.Parsed, st.Malformed, st.Flushes)
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return 1
	}
	return 0
}
