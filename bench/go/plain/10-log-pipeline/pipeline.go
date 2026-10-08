package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// DefaultQueueSize is the capacity of the queue between reader and workers.
const DefaultQueueSize = 1000

// TopN is how many most frequent messages are kept per service in a flush.
const TopN = 5

// Config controls a pipeline run.
type Config struct {
	Workers   int
	Flush     time.Duration
	OutDir    string
	QueueSize int // 0 means DefaultQueueSize

	// Ticks, if set, drives interval flushes instead of a real ticker. Tests
	// use it as a fake clock.
	Ticks <-chan time.Time

	hooks hooks
}

// hooks are test only observation points. All are optional.
type hooks struct {
	onRead      func(read int64) // after a line is accepted by the reader
	onQueueFull func()           // reader found the queue full and is about to block
	beforeParse func()           // worker received a line, before parsing
	afterParse  func()           // worker finished a line
	onFlush     func(path string)
}

// Stats is the summary of a run.
type Stats struct {
	Read      int64 `json:"read"`
	Parsed    int64 `json:"parsed"`
	Malformed int64 `json:"malformed"`
	Flushes   int64 `json:"flushes"`
}

// Entry is one parsed log line.
type Entry struct {
	TS      int64
	Level   string
	Service string
	Message string
}

var errMalformed = errors.New("malformed line")

// ParseLine parses `<unix_millis> <level> <service> <message>`.
func ParseLine(line string) (Entry, error) {
	line = strings.TrimRight(line, "\r\n")
	parts := strings.SplitN(line, " ", 4)
	if len(parts) != 4 {
		return Entry{}, errMalformed
	}
	ts, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil || ts < 0 {
		return Entry{}, errMalformed
	}
	if parts[1] == "" || parts[2] == "" || strings.TrimSpace(parts[3]) == "" {
		return Entry{}, errMalformed
	}
	return Entry{TS: ts, Level: strings.ToUpper(parts[1]), Service: parts[2], Message: parts[3]}, nil
}

// MessageCount is a message and how often it occurred.
type MessageCount struct {
	Message string `json:"message"`
	Count   int64  `json:"count"`
}

// ServiceSummary is the JSON object written per service in each flush.
type ServiceSummary struct {
	Service     string           `json:"service"`
	Total       int64            `json:"total"`
	Levels      map[string]int64 `json:"levels"`
	FirstTS     int64            `json:"first_ts"`
	LastTS      int64            `json:"last_ts"`
	TopMessages []MessageCount   `json:"top_messages"`
}

type serviceAgg struct {
	total   int64
	levels  map[string]int64
	first   int64
	last    int64
	msgs    map[string]int64
}

// Aggregator holds per service metrics. It is safe for concurrent use.
type Aggregator struct {
	mu       sync.Mutex
	services map[string]*serviceAgg
}

// NewAggregator returns an empty aggregator.
func NewAggregator() *Aggregator {
	return &Aggregator{services: make(map[string]*serviceAgg)}
}

// Add records one entry.
func (a *Aggregator) Add(e Entry) {
	a.mu.Lock()
	defer a.mu.Unlock()
	s, ok := a.services[e.Service]
	if !ok {
		s = &serviceAgg{levels: make(map[string]int64), msgs: make(map[string]int64), first: e.TS, last: e.TS}
		a.services[e.Service] = s
	}
	s.total++
	s.levels[e.Level]++
	s.msgs[e.Message]++
	if e.TS < s.first {
		s.first = e.TS
	}
	if e.TS > s.last {
		s.last = e.TS
	}
}

// Snapshot returns the current summaries sorted by service name.
func (a *Aggregator) Snapshot() []ServiceSummary {
	a.mu.Lock()
	defer a.mu.Unlock()
	out := make([]ServiceSummary, 0, len(a.services))
	for name, s := range a.services {
		levels := make(map[string]int64, len(s.levels))
		for k, v := range s.levels {
			levels[k] = v
		}
		out = append(out, ServiceSummary{
			Service:     name,
			Total:       s.total,
			Levels:      levels,
			FirstTS:     s.first,
			LastTS:      s.last,
			TopMessages: topMessages(s.msgs, TopN),
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Service < out[j].Service })
	return out
}

func topMessages(m map[string]int64, n int) []MessageCount {
	all := make([]MessageCount, 0, len(m))
	for k, v := range m {
		all = append(all, MessageCount{Message: k, Count: v})
	}
	sort.Slice(all, func(i, j int) bool {
		if all[i].Count != all[j].Count {
			return all[i].Count > all[j].Count
		}
		return all[i].Message < all[j].Message
	})
	if len(all) > n {
		all = all[:n]
	}
	return all
}

// WriteFileAtomic writes data to a temp file in the same directory, syncs it,
// and renames it over path, so readers never see a partial file.
func WriteFileAtomic(path string, data []byte) error {
	dir := filepath.Dir(path)
	f, err := os.CreateTemp(dir, ".tmp-"+filepath.Base(path)+"-*")
	if err != nil {
		return err
	}
	tmp := f.Name()
	ok := false
	defer func() {
		if !ok {
			_ = os.Remove(tmp)
		}
	}()
	if _, err := f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		return err
	}
	ok = true
	return nil
}

type pipeline struct {
	cfg   Config
	agg   *Aggregator
	stats struct {
		read, parsed, malformed, flushes atomic.Int64
	}
	flushMu  sync.Mutex
	flushSeq int
}

// flush writes the current aggregates as JSON lines, one object per service.
func (p *pipeline) flush() error {
	p.flushMu.Lock()
	defer p.flushMu.Unlock()
	snap := p.agg.Snapshot()
	var buf strings.Builder
	enc := json.NewEncoder(&buf)
	for _, s := range snap {
		if err := enc.Encode(s); err != nil {
			return err
		}
	}
	p.flushSeq++
	path := filepath.Join(p.cfg.OutDir, fmt.Sprintf("flush-%06d.jsonl", p.flushSeq))
	if err := WriteFileAtomic(path, []byte(buf.String())); err != nil {
		p.flushSeq--
		return err
	}
	p.stats.flushes.Add(1)
	if p.cfg.hooks.onFlush != nil {
		p.cfg.hooks.onFlush(path)
	}
	return nil
}

func (p *pipeline) worker(queue <-chan string) {
	for line := range queue {
		if p.cfg.hooks.beforeParse != nil {
			p.cfg.hooks.beforeParse()
		}
		e, err := ParseLine(line)
		if err != nil {
			p.stats.malformed.Add(1)
		} else {
			p.agg.Add(e)
			p.stats.parsed.Add(1)
		}
		if p.cfg.hooks.afterParse != nil {
			p.cfg.hooks.afterParse()
		}
	}
}

// readLines reads lines from in and sends them on the returned channel. It
// stops when ctx is done or input ends. A line handed over on the channel is
// the point where it becomes accepted by the pipeline.
func readLines(ctx context.Context, in io.Reader) (<-chan string, <-chan error) {
	lines := make(chan string)
	errc := make(chan error, 1)
	go func() {
		defer close(lines)
		br := bufio.NewReaderSize(in, 64*1024)
		for {
			s, err := br.ReadString('\n')
			if s != "" {
				select {
				case lines <- s:
				case <-ctx.Done():
					return
				}
			}
			if err != nil {
				if !errors.Is(err, io.EOF) {
					errc <- err
				}
				return
			}
		}
	}()
	return lines, errc
}

// Run executes the pipeline until input ends or ctx is cancelled. It always
// drains the queue and writes a final flush before returning.
func Run(ctx context.Context, in io.Reader, cfg Config) (Stats, error) {
	if cfg.Workers < 1 {
		return Stats{}, fmt.Errorf("workers must be at least 1")
	}
	if cfg.Ticks == nil && cfg.Flush <= 0 {
		return Stats{}, fmt.Errorf("flush interval must be positive")
	}
	if cfg.QueueSize <= 0 {
		cfg.QueueSize = DefaultQueueSize
	}
	if err := os.MkdirAll(cfg.OutDir, 0o755); err != nil {
		return Stats{}, err
	}
	p := &pipeline{cfg: cfg, agg: NewAggregator()}

	queue := make(chan string, cfg.QueueSize)
	var wg sync.WaitGroup
	for i := 0; i < cfg.Workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			p.worker(queue)
		}()
	}

	// Interval flusher.
	ticks := cfg.Ticks
	var ticker *time.Ticker
	if ticks == nil {
		ticker = time.NewTicker(cfg.Flush)
		ticks = ticker.C
	}
	stopFlusher := make(chan struct{})
	flusherDone := make(chan struct{})
	var flushErrMu sync.Mutex
	var flushErr error
	go func() {
		defer close(flusherDone)
		for {
			select {
			case <-stopFlusher:
				return
			case <-ticks:
				if err := p.flush(); err != nil {
					flushErrMu.Lock()
					if flushErr == nil {
						flushErr = err
					}
					flushErrMu.Unlock()
				}
			}
		}
	}()

	// Reader: owns the queue and closes it when done.
	readCtx, cancelRead := context.WithCancel(ctx)
	defer cancelRead()
	lines, readErrc := readLines(readCtx, in)
	var readErr error
readLoop:
	for {
		// Prefer stopping as soon as a signal arrives.
		select {
		case <-ctx.Done():
			break readLoop
		default:
		}
		select {
		case <-ctx.Done():
			break readLoop
		case s, ok := <-lines:
			if !ok {
				break readLoop
			}
			n := p.stats.read.Add(1)
			if cfg.hooks.onRead != nil {
				cfg.hooks.onRead(n)
			}
			// Blocking send: backpressure, never drop. Workers keep draining
			// even after cancellation, so this always completes.
			select {
			case queue <- s:
			default:
				if cfg.hooks.onQueueFull != nil {
					cfg.hooks.onQueueFull()
				}
				queue <- s
			}
		}
	}
	cancelRead()
	select {
	case readErr = <-readErrc:
	default:
	}
	close(queue)
	wg.Wait()

	close(stopFlusher)
	<-flusherDone
	if ticker != nil {
		ticker.Stop()
	}

	finalErr := p.flush()
	st := Stats{
		Read:      p.stats.read.Load(),
		Parsed:    p.stats.parsed.Load(),
		Malformed: p.stats.malformed.Load(),
		Flushes:   p.stats.flushes.Load(),
	}
	flushErrMu.Lock()
	defer flushErrMu.Unlock()
	return st, errors.Join(finalErr, flushErr, readErr)
}
