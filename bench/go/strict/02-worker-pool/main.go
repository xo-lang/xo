// Command benchtask reads URLs from standard input, one per line, fetches
// each with a bounded worker pool, and prints one line per URL followed by a
// summary line.
package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"time"

	"benchtask/pool"
)

const (
	fetchWorkers = 8
	fetchTimeout = 30 * time.Second
	maxLineBytes = 1 << 20
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	err := run(ctx, os.Stdin, os.Stdout, http.DefaultClient, fetchWorkers, fetchTimeout)
	stop()
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

// readURLs returns the non empty, trimmed lines of r.
func readURLs(r io.Reader) ([]string, error) {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), maxLineBytes)
	var urls []string
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		urls = append(urls, line)
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("reading urls: %w", err)
	}
	return urls, nil
}

// fetcher returns a pool function that issues a GET and returns the status
// code. The body is not read; only the status matters here.
func fetcher(client *http.Client) pool.Func[string, int] {
	return func(ctx context.Context, url string) (int, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return 0, err
		}
		resp, err := client.Do(req)
		if err != nil {
			return 0, err
		}
		if err := resp.Body.Close(); err != nil {
			return 0, fmt.Errorf("closing body: %w", err)
		}
		return resp.StatusCode, nil
	}
}

// run reads URLs from in, fetches them and writes the report to out.
func run(ctx context.Context, in io.Reader, out io.Writer, client *http.Client, workers int, timeout time.Duration) error {
	if client == nil {
		return errors.New("nil http client")
	}
	urls, err := readURLs(in)
	if err != nil {
		return err
	}
	jobs := make([]pool.Job[string], len(urls))
	for i, u := range urls {
		jobs[i] = pool.Job[string]{ID: u, Input: u}
	}

	report, err := pool.Run(ctx, jobs, workers, timeout, fetcher(client))
	if err != nil {
		return err
	}

	lines := make([]string, len(urls))
	for _, r := range report.Results {
		lines[r.Index] = fmt.Sprintf("%s %d", r.JobID, r.Value)
	}
	for _, f := range report.Failures {
		lines[f.Index] = fmt.Sprintf("%s error: %v", f.JobID, f.Err)
	}

	var b strings.Builder
	for _, l := range lines {
		b.WriteString(l)
		b.WriteByte('\n')
	}
	fmt.Fprintf(&b, "summary: %d urls, %d ok, %d failed, elapsed %s\n",
		len(urls), len(report.Results), len(report.Failures), report.Elapsed.Round(time.Millisecond))
	if _, err := io.WriteString(out, b.String()); err != nil {
		return fmt.Errorf("writing output: %w", err)
	}
	return nil
}
