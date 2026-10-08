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
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	code := runCLI(ctx, os.Args[1:], os.Stdin, os.Stdout, os.Stderr)
	stop()
	os.Exit(code)
}

// runCLI parses flags, runs the pipeline, and prints the summary. It returns
// the process exit code.
func runCLI(ctx context.Context, args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	fsFlags := flag.NewFlagSet("logpipe", flag.ContinueOnError)
	fsFlags.SetOutput(stderr)
	workers := fsFlags.Int("workers", DefaultWorkers, "number of parser workers")
	flushEvery := fsFlags.Duration("flush", 10*time.Second, "flush interval")
	out := fsFlags.String("out", "", "output directory for flush files (required)")
	if err := fsFlags.Parse(args); err != nil {
		return 2
	}
	if *out == "" {
		warn(stderr, "--out is required")
		return 2
	}
	if *workers < 1 {
		warn(stderr, "--workers must be at least 1")
		return 2
	}
	if *flushEvery <= 0 {
		warn(stderr, "--flush must be positive")
		return 2
	}
	if err := os.MkdirAll(*out, 0o755); err != nil {
		warn(stderr, fmt.Sprintf("create output directory: %v", err))
		return 1
	}
	ticker := time.NewTicker(*flushEvery)
	defer ticker.Stop()

	sum, err := Run(ctx, stdin, Config{
		Workers:   *workers,
		QueueSize: DefaultQueueSize,
		OutDir:    *out,
		Ticks:     ticker.C,
	})
	if _, werr := fmt.Fprintln(stdout, sum.String()); werr != nil && err == nil {
		err = werr
	}
	if err != nil {
		warn(stderr, fmt.Sprintf("error: %v", err))
		return 1
	}
	return 0
}

// warn writes a diagnostic to stderr. If stderr itself cannot be written
// there is nowhere left to report that failure, and the exit code already
// signals the error, so the write error is deliberately ignored.
func warn(stderr io.Writer, msg string) {
	_, _ = fmt.Fprintln(stderr, msg)
}
