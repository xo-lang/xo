package main

import (
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// TopN is how many of the most frequent messages are reported per service.
const TopN = 5

// Entry is one parsed log line.
type Entry struct {
	TS      int64
	Level   string
	Service string
	Message string
}

// ParseLine parses `<unix_millis> <level> <service> <message>`. Fields are
// separated by single spaces; the message is everything after the third
// space and must not be blank. The level is upper cased.
func ParseLine(line string) (Entry, bool) {
	line = strings.TrimRight(line, "\r\n")
	tsStr, rest, ok := strings.Cut(line, " ")
	if !ok || tsStr == "" {
		return Entry{}, false
	}
	level, rest, ok := strings.Cut(rest, " ")
	if !ok || level == "" {
		return Entry{}, false
	}
	service, msg, ok := strings.Cut(rest, " ")
	if !ok || service == "" || strings.TrimSpace(msg) == "" {
		return Entry{}, false
	}
	ts, err := strconv.ParseInt(tsStr, 10, 64)
	if err != nil || ts < 0 {
		return Entry{}, false
	}
	return Entry{TS: ts, Level: strings.ToUpper(level), Service: service, Message: msg}, true
}

type serviceAgg struct {
	levels   map[string]int64
	first    int64
	last     int64
	messages map[string]int64
}

// MessageCount is a message and how often it was seen.
type MessageCount struct {
	Message string `json:"message"`
	Count   int64  `json:"count"`
}

// ServiceSnapshot is the JSON object written for one service in a flush.
type ServiceSnapshot struct {
	Service      string           `json:"service"`
	Total        int64            `json:"total"`
	CountByLevel map[string]int64 `json:"count_by_level"`
	FirstTS      int64            `json:"first_ts"`
	LastTS       int64            `json:"last_ts"`
	TopMessages  []MessageCount   `json:"top_messages"`
	FlushSeq     int              `json:"flush_seq"`
	FlushedAt    time.Time        `json:"flushed_at"`
}

// Aggregator keeps cumulative per service metrics. It is safe for
// concurrent use by many workers.
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
		s = &serviceAgg{
			levels:   make(map[string]int64),
			first:    e.TS,
			last:     e.TS,
			messages: make(map[string]int64),
		}
		a.services[e.Service] = s
	}
	s.levels[e.Level]++
	s.messages[e.Message]++
	if e.TS < s.first {
		s.first = e.TS
	}
	if e.TS > s.last {
		s.last = e.TS
	}
}

// Snapshot returns a copy of the current aggregates sorted by service name.
func (a *Aggregator) Snapshot(seq int, at time.Time) []ServiceSnapshot {
	a.mu.Lock()
	defer a.mu.Unlock()
	out := make([]ServiceSnapshot, 0, len(a.services))
	for name, s := range a.services {
		levels := make(map[string]int64, len(s.levels))
		var total int64
		for l, c := range s.levels {
			levels[l] = c
			total += c
		}
		out = append(out, ServiceSnapshot{
			Service:      name,
			Total:        total,
			CountByLevel: levels,
			FirstTS:      s.first,
			LastTS:       s.last,
			TopMessages:  topMessages(s.messages, TopN),
			FlushSeq:     seq,
			FlushedAt:    at,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Service < out[j].Service })
	return out
}

func topMessages(m map[string]int64, n int) []MessageCount {
	all := make([]MessageCount, 0, len(m))
	for msg, c := range m {
		all = append(all, MessageCount{Message: msg, Count: c})
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
