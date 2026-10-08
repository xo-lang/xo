package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"
)

// Defaults.
const (
	DefaultQueueSize = 1000
	DefaultWorkers   = 4
	DefaultMaxLine   = 64 * 1024
)

// Hooks are optional callbacks used by tests to observe the pipeline
// deterministically. All may be nil. They can be called concurrently.
type Hooks struct {
	// BeforeProcess runs in a worker before a line is parsed.
	BeforeProcess func(line string)
	// AfterProcess runs in a worker after a line is parsed and counted.
	AfterProcess func()
	// ReaderBlocked runs in the reader when the queue is full, just before
	// it blocks on the send.
	ReaderBlocked func()
	// Accepted runs in the reader after a line was accepted, with the
	// running total of lines read.
	Accepted func(total int64)
	// Flushed runs after a flush file was renamed into place.
	Flushed func(path string, seq int)
}

// Config configures a pipeline run.
type Config struct {
	Workers   int
	QueueSize int
	MaxLine   int
	OutDir    string
	// Ticks triggers periodic flushes. A nil channel disables them.
	Ticks <-chan time.Time
	// Now stamps flushes. Nil means time.Now.
	Now   func() time.Time
	Hooks Hooks
}

// Summary is reported at exit.
type Summary struct {
	Read          int64
	Parsed        int64
	Malformed     int64
	Flushes       int64
	ReaderBlocked int64
}

func (s Summary) String() string {
	return fmt.Sprintf("lines read: %d, parsed: %d, malformed: %d, flushes written: %d",
		s.Read, s.Parsed, s.Malformed, s.Flushes)
}

type stats struct {
	read, parsed, malformed, flushes, blocked atomic.Int64
}

type pipeline struct {
	cfg   Config
	agg   *Aggregator
	queue chan string
	st    stats

	flushMu sync.Mutex
	seq     int
}

// Run ingests lines from in until end of input or until ctx is done, then
// drains the queue, writes a final flush, and returns the summary. Every
// line accepted into the queue is aggregated before the final flush.
func Run(ctx context.Context, in io.Reader, cfg Config) (Summary, error) {
	if cfg.Workers <= 0 {
		cfg.Workers = DefaultWorkers
	}
	if cfg.QueueSize <= 0 {
		cfg.QueueSize = DefaultQueueSize
	}
	if cfg.MaxLine <= 0 {
		cfg.MaxLine = DefaultMaxLine
	}
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	if cfg.OutDir == "" {
		return Summary{}, errors.New("output directory is required")
	}
	p := &pipeline{cfg: cfg, agg: NewAggregator(), queue: make(chan string, cfg.QueueSize)}

	var workers sync.WaitGroup
	for range cfg.Workers {
		workers.Go(p.work)
	}

	stopFlusher := make(chan struct{})
	flusherDone := make(chan struct{})
	var flushErrMu sync.Mutex
	var periodicErr error
	go func() {
		defer close(flusherDone)
		for {
			select {
			case <-stopFlusher:
				return
			case _, ok := <-cfg.Ticks:
				if !ok {
					<-stopFlusher
					return
				}
				if err := p.flush(); err != nil {
					flushErrMu.Lock()
					periodicErr = errors.Join(periodicErr, err)
					flushErrMu.Unlock()
				}
			}
		}
	}()

	readErr := p.read(ctx, in)

	close(p.queue)
	workers.Wait()
	close(stopFlusher)
	<-flusherDone
	finalErr := p.flush()

	flushErrMu.Lock()
	err := errors.Join(readErr, periodicErr, finalErr)
	flushErrMu.Unlock()
	return p.summary(), err
}

func (p *pipeline) summary() Summary {
	return Summary{
		Read:          p.st.read.Load(),
		Parsed:        p.st.parsed.Load(),
		Malformed:     p.st.malformed.Load(),
		Flushes:       p.st.flushes.Load(),
		ReaderBlocked: p.st.blocked.Load(),
	}
}

type rawLine struct {
	text    string
	tooLong bool
}

// read accepts lines into the queue until end of input or ctx is done. The
// blocking input read happens in a separate goroutine so a signal can stop
// the pipeline even while stdin is idle. A line handed over by that
// goroutine is always enqueued (blocking if needed), so it is never lost.
func (p *pipeline) read(ctx context.Context, in io.Reader) error {
	lines := make(chan rawLine)
	quit := make(chan struct{})
	defer close(quit)
	scanErr := make(chan error, 1)
	go func() {
		scanErr <- scanLines(in, p.cfg.MaxLine, lines, quit)
		close(lines)
	}()

	for {
		select {
		case <-ctx.Done():
			return nil
		case rl, ok := <-lines:
			if !ok {
				return <-scanErr
			}
			if rl.tooLong {
				p.st.malformed.Add(1)
				p.accepted()
				continue
			}
			select {
			case p.queue <- rl.text:
			default:
				p.st.blocked.Add(1)
				if h := p.cfg.Hooks.ReaderBlocked; h != nil {
					h()
				}
				p.queue <- rl.text
			}
			p.accepted()
		}
	}
}

func (p *pipeline) accepted() {
	n := p.st.read.Add(1)
	if h := p.cfg.Hooks.Accepted; h != nil {
		h(n)
	}
}

// scanLines splits r into lines of at most maxLine bytes. Longer lines are
// skipped and reported as tooLong so they can be counted as malformed.
func scanLines(r io.Reader, maxLine int, out chan<- rawLine, quit <-chan struct{}) error {
	br := bufio.NewReaderSize(r, maxLine)
	for {
		b, err := br.ReadSlice('\n')
		rl := rawLine{}
		if errors.Is(err, bufio.ErrBufferFull) {
			rl.tooLong = true
			for errors.Is(err, bufio.ErrBufferFull) {
				_, err = br.ReadSlice('\n')
			}
		} else {
			rl.text = string(b)
		}
		if rl.tooLong || len(b) > 0 {
			select {
			case out <- rl:
			case <-quit:
				return nil
			}
		}
		if err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			return err
		}
	}
}

func (p *pipeline) work() {
	for line := range p.queue {
		if h := p.cfg.Hooks.BeforeProcess; h != nil {
			h(line)
		}
		if e, ok := ParseLine(line); ok {
			p.agg.Add(e)
			p.st.parsed.Add(1)
		} else {
			p.st.malformed.Add(1)
		}
		if h := p.cfg.Hooks.AfterProcess; h != nil {
			h()
		}
	}
}

// flush writes the current aggregates, one JSON object per service per
// line, to a new file in the output directory.
func (p *pipeline) flush() error {
	p.flushMu.Lock()
	defer p.flushMu.Unlock()
	seq := p.seq + 1
	snap := p.agg.Snapshot(seq, p.cfg.Now().UTC())
	path := filepath.Join(p.cfg.OutDir, fmt.Sprintf("flush-%06d.json", seq))
	err := writeAtomic(p.cfg.OutDir, path, func(w io.Writer) error {
		enc := json.NewEncoder(w)
		for _, s := range snap {
			if err := enc.Encode(s); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("flush %d: %w", seq, err)
	}
	p.seq = seq
	p.st.flushes.Add(1)
	if h := p.cfg.Hooks.Flushed; h != nil {
		h(path, seq)
	}
	return nil
}

// writeAtomic writes to a temp file in dir, syncs it, and renames it to
// name, so readers never see a partial file. On failure the temp file is
// removed.
func writeAtomic(dir, name string, write func(io.Writer) error) (err error) {
	f, err := os.CreateTemp(dir, ".flush-*.tmp")
	if err != nil {
		return err
	}
	tmp := f.Name()
	closed := false
	defer func() {
		if err == nil {
			return
		}
		if !closed {
			if cErr := f.Close(); cErr != nil {
				err = errors.Join(err, cErr)
			}
		}
		if rmErr := os.Remove(tmp); rmErr != nil && !errors.Is(rmErr, fs.ErrNotExist) {
			err = errors.Join(err, rmErr)
		}
	}()
	bw := bufio.NewWriter(f)
	if err = write(bw); err != nil {
		return err
	}
	if err = bw.Flush(); err != nil {
		return err
	}
	if err = f.Sync(); err != nil {
		return err
	}
	closed = true
	if err = f.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, name)
}
