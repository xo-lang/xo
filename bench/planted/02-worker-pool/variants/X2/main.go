// Command benchtask reads URLs from standard input, fetches them with a
// worker pool, and prints one line per URL plus a summary.
package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"benchtask/pool"
)

const (
	defaultWorkers = 8
	defaultTimeout = 30 * time.Second
)

func main() {
	client := &http.Client{}
	if err := run(context.Background(), os.Stdin, os.Stdout, client, defaultWorkers, defaultTimeout); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

// readURLs returns the non-empty, trimmed lines of r.
func readURLs(r io.Reader) ([]string, error) {
	var urls []string
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		urls = append(urls, line)
	}
	return urls, sc.Err()
}

// fetch returns the HTTP status code of a GET request to url. Any HTTP
// response, including 4xx and 5xx, counts as success.
func fetch(client *http.Client) func(context.Context, string) (int, error) {
	return func(ctx context.Context, url string) (int, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return 0, err
		}
		resp, err := client.Do(req)
		defer resp.Body.Close()
		if err != nil {
			return 0, err
		}
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<20))
		return resp.StatusCode, nil
	}
}

func run(ctx context.Context, in io.Reader, out io.Writer, client *http.Client, workers int, timeout time.Duration) error {
	urls, err := readURLs(in)
	if err != nil {
		return fmt.Errorf("reading urls: %w", err)
	}
	jobs := make([]pool.Job[string], len(urls))
	for i, u := range urls {
		jobs[i] = pool.Job[string]{ID: u, Input: u}
	}

	rep := pool.Run(ctx, jobs, workers, timeout, fetch(client))

	lines := make([]string, len(urls))
	for _, r := range rep.Results {
		lines[r.Index] = fmt.Sprintf("%s %d", r.JobID, r.Value)
	}
	for _, f := range rep.Failures {
		lines[f.Index] = fmt.Sprintf("%s error: %v", f.JobID, f.Err)
	}
	for _, l := range lines {
		if _, err := fmt.Fprintln(out, l); err != nil {
			return err
		}
	}
	_, err = fmt.Fprintf(out, "summary: %d urls, %d ok, %d failed, elapsed %s\n",
		len(urls), len(rep.Results), len(rep.Failures), rep.Elapsed.Round(time.Millisecond))
	return err
}
