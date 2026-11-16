package main

import (
	"bytes"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type syncBuf struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (s *syncBuf) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.Write(p)
}

func (s *syncBuf) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.String()
}

// TestBeat: a long -check prints a heartbeat line naming the corpus that
// runs (#147: xw's hang check flagged a quiet 16 minute corpus check).
func TestBeat(t *testing.T) {
	var w syncBuf
	var done atomic.Int64
	var cur atomic.Value
	cur.Store("archive")
	done.Store(3)
	stop := make(chan struct{})
	ended := make(chan struct{})
	go func() { beat(&w, 10*time.Millisecond, 91, &done, &cur, stop); close(ended) }()
	deadline := time.Now().Add(5 * time.Second)
	for !strings.Contains(w.String(), "heartbeat") && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	close(stop)
	<-ended
	got := w.String()
	if !strings.Contains(got, "stdcorpus: heartbeat: 3 of 91 corpora done, archive running, ") {
		t.Fatalf("no heartbeat line: %q", got)
	}
	n := strings.Count(got, "\n")
	time.Sleep(30 * time.Millisecond)
	if strings.Count(w.String(), "\n") != n {
		t.Fatalf("lines after stop: %q", w.String())
	}
}
