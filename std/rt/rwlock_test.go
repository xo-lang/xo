package rt

import (
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// Tests of the shared read mode of Mutex (Mutex_with_read, decision 0063):
// readers run together, a writer excludes everything, a waiting writer
// stops new readers, and a canceled waiter leaves the lock consistent.
// Real runs (barging) and the deterministic scheduler (FIFO handoff) are
// both covered.

func unit() Result[Unit, Never] { return Ok[Unit, Never](Unit{}) }

// readFn and writeFn adapt Go closures to the generated calling convention.
func readFn(f func(*Task, int64)) func(*Task, int64) Result[Unit, Never] {
	return func(t *Task, v int64) Result[Unit, Never] { f(t, v); return unit() }
}

func writeFn(f func(*Task, *int64)) func(*Task, *int64) Result[Unit, Never] {
	return func(t *Task, v *int64) Result[Unit, Never] { f(t, v); return unit() }
}

// waitFor polls cond for up to 5 s.
func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(time.Millisecond)
	}
}

func lockState(m *Mutex[int64]) (permits, readers, wwait, queued int) {
	m.l.mu.Lock()
	defer m.l.mu.Unlock()
	return m.l.permits, m.l.readers, m.l.wwait, len(m.l.q)
}

// TestMutexReadersConcurrent: n readers are inside with_read at the same
// time (each waits inside until all have entered).
func TestMutexReadersConcurrent(t *testing.T) {
	root := NewRootTask()
	s := NewScope(root)
	body := s.Body()
	m := Mutex_new[int64](body, 7)
	const n = 8
	var inside sync.WaitGroup
	inside.Add(n)
	all := make(chan struct{})
	go func() { inside.Wait(); close(all) }()
	for i := 0; i < n; i++ {
		Spawn(body, s, func(t *Task) Result[Unit, Never] {
			Mutex_with_read(t, m, readFn(func(_ *Task, v int64) {
				if v != 7 {
					panic("wrong value")
				}
				inside.Done()
				select {
				case <-all:
				case <-time.After(5 * time.Second):
					panic("readers did not run together")
				}
			}))
			return unit()
		}, false, true)
	}
	s.Close(nil, true, 0)
	if p, r, w, q := lockState(m); p != 1 || r != 0 || w != 0 || q != 0 {
		t.Fatalf("lock not free after: permits %d readers %d wwait %d queued %d", p, r, w, q)
	}
}

// TestMutexWriterExcludes: a writer waits for the readers inside, and
// readers wait for a writer inside.
func TestMutexWriterExcludes(t *testing.T) {
	root := NewRootTask()
	s := NewScope(root)
	body := s.Body()
	m := Mutex_new[int64](body, 0)
	var active, writers atomic.Int64
	bad := atomic.Bool{}
	for i := 0; i < 16; i++ {
		write := i%4 == 0
		Spawn(body, s, func(t *Task) Result[Unit, Never] {
			for j := 0; j < 200; j++ {
				if write {
					Mutex_with(t, m, writeFn(func(_ *Task, v *int64) {
						if writers.Add(1) != 1 || active.Load() != 0 {
							bad.Store(true)
						}
						*v++
						writers.Add(-1)
					}))
				} else {
					Mutex_with_read(t, m, readFn(func(_ *Task, v int64) {
						active.Add(1)
						if writers.Load() != 0 {
							bad.Store(true)
						}
						active.Add(-1)
					}))
				}
			}
			return unit()
		}, false, true)
	}
	s.Close(nil, true, 0)
	if bad.Load() {
		t.Fatal("a writer ran together with a reader or another writer")
	}
	if m.v != 4*200 {
		t.Fatalf("value %d, want %d", m.v, 4*200)
	}
}

// TestMutexWriterPreference: while a reader is inside and a writer waits,
// a new reader queues behind the writer, so the writer goes first.
func TestMutexWriterPreference(t *testing.T) {
	root := NewRootTask()
	s := NewScope(root)
	body := s.Body()
	m := Mutex_new[int64](body, 0)
	var mu sync.Mutex
	var order []string
	note := func(s string) { mu.Lock(); order = append(order, s); mu.Unlock() }
	release := make(chan struct{})
	entered := make(chan struct{})
	Spawn(body, s, func(t *Task) Result[Unit, Never] {
		Mutex_with_read(t, m, readFn(func(*Task, int64) {
			note("r1")
			close(entered)
			<-release
		}))
		return unit()
	}, false, true)
	<-entered
	Spawn(body, s, func(t *Task) Result[Unit, Never] {
		Mutex_with(t, m, writeFn(func(_ *Task, v *int64) { note("w"); *v = 1 }))
		return unit()
	}, false, true)
	waitFor(t, "the writer to queue", func() bool { _, _, w, q := lockState(m); return w == 1 && q == 1 })
	Spawn(body, s, func(t *Task) Result[Unit, Never] {
		Mutex_with_read(t, m, readFn(func(_ *Task, v int64) {
			if v != 1 {
				panic("the second reader ran before the writer")
			}
			note("r2")
		}))
		return unit()
	}, false, true)
	waitFor(t, "the second reader to queue", func() bool { _, _, _, q := lockState(m); return q == 2 })
	close(release)
	s.Close(nil, true, 0)
	if got := strings.Join(order, " "); got != "r1 w r2" {
		t.Fatalf("order %q, want r1 w r2", got)
	}
}

// TestMutexWaitingWriterCanceled: readers queued behind a waiting writer
// enter as soon as that writer is canceled, without waiting for a release.
func TestMutexWaitingWriterCanceled(t *testing.T) {
	root := NewRootTask()
	s := NewScope(root)
	body := s.Body()
	m := Mutex_new[int64](body, 0)
	release := make(chan struct{})
	entered := make(chan struct{})
	Spawn(body, s, func(t *Task) Result[Unit, Never] {
		Mutex_with_read(t, m, readFn(func(*Task, int64) {
			close(entered)
			<-release
		}))
		return unit()
	}, false, true)
	<-entered
	ws := NewScope(body)
	wbody := ws.Body()
	wrote := atomic.Bool{}
	Spawn(wbody, ws, func(t *Task) Result[Unit, Never] {
		Mutex_with(t, m, writeFn(func(*Task, *int64) { wrote.Store(true) }))
		return unit()
	}, false, true)
	waitFor(t, "the writer to queue", func() bool { _, _, w, _ := lockState(m); return w == 1 })
	r2 := make(chan struct{})
	Spawn(body, s, func(t *Task) Result[Unit, Never] {
		Mutex_with_read(t, m, readFn(func(*Task, int64) { close(r2) }))
		return unit()
	}, false, true)
	waitFor(t, "the second reader to queue", func() bool { _, _, _, q := lockState(m); return q == 2 })
	ws.M_cancel(wbody)
	ws.Close(nil, true, 0)
	select {
	case <-r2:
	case <-time.After(5 * time.Second):
		t.Fatal("the queued reader did not enter after the writer was canceled")
	}
	close(release)
	s.Close(nil, true, 0)
	if wrote.Load() {
		t.Fatal("the canceled writer ran")
	}
	if p, r, w, q := lockState(m); p != 1 || r != 0 || w != 0 || q != 0 {
		t.Fatalf("lock not free after: permits %d readers %d wwait %d queued %d", p, r, w, q)
	}
}

// TestMutexWaitingReaderCanceled: a reader waiting for a writer is
// canceled; the lock is free afterwards and the writer's release wakes
// no one.
func TestMutexWaitingReaderCanceled(t *testing.T) {
	root := NewRootTask()
	s := NewScope(root)
	body := s.Body()
	m := Mutex_new[int64](body, 0)
	release := make(chan struct{})
	entered := make(chan struct{})
	Spawn(body, s, func(t *Task) Result[Unit, Never] {
		Mutex_with(t, m, writeFn(func(*Task, *int64) {
			close(entered)
			<-release
		}))
		return unit()
	}, false, true)
	<-entered
	rs := NewScope(body)
	rbody := rs.Body()
	read := atomic.Bool{}
	Spawn(rbody, rs, func(t *Task) Result[Unit, Never] {
		Mutex_with_read(t, m, readFn(func(*Task, int64) { read.Store(true) }))
		return unit()
	}, false, true)
	waitFor(t, "the reader to queue", func() bool { _, _, _, q := lockState(m); return q == 1 })
	rs.M_cancel(rbody)
	rs.Close(nil, true, 0)
	close(release)
	s.Close(nil, true, 0)
	if read.Load() {
		t.Fatal("the canceled reader ran")
	}
	if p, r, w, q := lockState(m); p != 1 || r != 0 || w != 0 || q != 0 {
		t.Fatalf("lock not free after: permits %d readers %d wwait %d queued %d", p, r, w, q)
	}
}

// TestMutexCanceledWaitForgetsSite: Mutex.with and with_read record the
// holding site in the task before they wait (so the stack walk stays out
// of the critical section); a wait that is canceled must remove it, or a
// scope body (which shares the record with its parent task) would leave
// the parent unable to take the mutex again (XO-F004).
func TestMutexCanceledWaitForgetsSite(t *testing.T) {
	for _, read := range []bool{false, true} {
		root := NewRootTask()
		s := NewScope(root)
		body := s.Body()
		m := Mutex_new[int64](body, 0)
		release := make(chan struct{})
		entered := make(chan struct{})
		Spawn(body, s, func(t *Task) Result[Unit, Never] {
			Mutex_with(t, m, writeFn(func(*Task, *int64) {
				close(entered)
				<-release
			}))
			return unit()
		}, false, true)
		<-entered
		ws := NewScope(body)
		wbody := ws.Body()
		left := make(chan bool, 1)
		Spawn(wbody, ws, func(t *Task) Result[Unit, Never] {
			defer func() {
				_, still := t.held[m]
				left <- still
			}()
			if read {
				Mutex_with_read(t, m, readFn(func(*Task, int64) {}))
			} else {
				Mutex_with(t, m, writeFn(func(*Task, *int64) {}))
			}
			return unit()
		}, false, true)
		waitFor(t, "the waiter to queue", func() bool { _, _, _, q := lockState(m); return q == 1 })
		ws.M_cancel(wbody)
		ws.Close(nil, true, 0)
		if <-left {
			t.Fatalf("read=%v: the canceled wait left its site in the task", read)
		}
		close(release)
		s.Close(nil, true, 0)
		if p, r, w, q := lockState(m); p != 1 || r != 0 || w != 0 || q != 0 {
			t.Fatalf("read=%v: lock not free after: permits %d readers %d wwait %d queued %d", read, p, r, w, q)
		}
	}
}

// schedRun runs body under the deterministic scheduler with the given seed
// and returns its log, or "deadlock" and the report when it gets stuck.
func schedRun(t *testing.T, seed int64, body func(w *World, s *Scope, add func(string))) (string, string) {
	t.Helper()
	w := NewSchedWorld(seed)
	var mu sync.Mutex
	var log []string
	add := func(s string) { mu.Lock(); log = append(log, s); mu.Unlock() }
	done := make(chan struct{})
	go func() {
		defer close(done)
		tk := w.NewTask()
		s := NewScope(tk)
		body(w, s, add)
		s.Close(nil, true, 0)
		w.EndTask(tk)
	}()
	select {
	case <-done:
	case <-w.Stuck():
		return "deadlock", w.DeadlockFault().Error()
	case <-time.After(10 * time.Second):
		t.Fatalf("seed %d: no verdict", seed)
	}
	mu.Lock()
	defer mu.Unlock()
	return strings.Join(log, " "), ""
}

// rwSections: two readers and a writer, each with a scheduling point
// (a virtual sleep) inside its section.
func rwSections(w *World, s *Scope, add func(string)) {
	body := s.Body()
	m := Mutex_new[int64](body, 0)
	for _, name := range []string{"a", "b"} {
		Spawn(body, s, func(t *Task) Result[Unit, Never] {
			Mutex_with_read(t, m, readFn(func(t *Task, _ int64) {
				add(name + "<")
				w.Sleep(t, 1)
				add(name + ">")
			}))
			return unit()
		}, false, true)
	}
	Spawn(body, s, func(t *Task) Result[Unit, Never] {
		Mutex_with(t, m, writeFn(func(t *Task, v *int64) {
			add("w<")
			w.Sleep(t, 1)
			add("w>")
		}))
		return unit()
	}, false, true)
}

// TestSchedReadersTogether: under the deterministic scheduler readers can
// be inside at once and a writer never overlaps anyone; each seed repeats
// its interleaving, and different seeds give different ones.
func TestSchedReadersTogether(t *testing.T) {
	seen := map[string]bool{}
	overlap := false
	for seed := int64(1); seed <= 20; seed++ {
		got, report := schedRun(t, seed, rwSections)
		if report != "" {
			t.Fatalf("seed %d: %s", seed, report)
		}
		if again, _ := schedRun(t, seed, rwSections); again != got {
			t.Fatalf("seed %d: %q then %q", seed, got, again)
		}
		if !strings.Contains(got, "w< w>") {
			t.Fatalf("seed %d: the writer overlapped a reader: %q", seed, got)
		}
		if strings.HasPrefix(got, "a< b<") || strings.HasPrefix(got, "b< a<") || strings.Contains(got, "> a< b<") || strings.Contains(got, "> b< a<") {
			overlap = true
		}
		seen[got] = true
	}
	if !overlap {
		t.Errorf("no seed had two readers inside at once: %v", seen)
	}
	if len(seen) < 2 {
		t.Errorf("20 seeds gave %d interleavings: %v", len(seen), seen)
	}
}

// readerWaitsForReader: reader a waits inside its section for reader b.
// That needs both inside at once, which a waiting writer prevents (writer
// preference): with the writer some seeds deadlock, and the report names
// the reader holding the mutex.
func readerWaitsForReader(writer bool) func(w *World, s *Scope, add func(string)) {
	return func(w *World, s *Scope, add func(string)) {
		body := s.Body()
		m := Mutex_new[int64](body, 0)
		ping := Chan_new[int64](body, 0)
		Spawn(body, s, func(t *Task) Result[Unit, Never] {
			Mutex_with_read(t, m, readFn(func(t *Task, v int64) { add("a"); ping.M_recv(t) }))
			return unit()
		}, false, true)
		Spawn(body, s, func(t *Task) Result[Unit, Never] {
			Mutex_with_read(t, m, readFn(func(t *Task, v int64) { add("b"); ping.M_send(t, v) }))
			return unit()
		}, false, true)
		if writer {
			Spawn(body, s, func(t *Task) Result[Unit, Never] {
				Mutex_with(t, m, writeFn(func(*Task, *int64) { add("w") }))
				return unit()
			}, false, true)
		}
	}
}

func TestSchedReaderWaitsForReader(t *testing.T) {
	for seed := int64(1); seed <= 20; seed++ {
		if got, report := schedRun(t, seed, readerWaitsForReader(false)); report != "" || (got != "a b" && got != "b a") {
			t.Fatalf("seed %d without a writer: %q %s", seed, got, report)
		}
	}
	deadlocks := 0
	for seed := int64(1); seed <= 20; seed++ {
		got, report := schedRun(t, seed, readerWaitsForReader(true))
		if report == "" {
			continue
		}
		deadlocks++
		for _, want := range []string{"XO-F001: deadlock", "blocked in Mutex.with", "held for reading by task", "blocked in Mutex.with_read"} {
			if !strings.Contains(report, want) {
				t.Errorf("seed %d: report lacks %q:\n%s", seed, want, report)
			}
		}
		if deadlocks == 1 {
			t.Logf("seed %d (%q):\n%s", seed, got, report)
		}
	}
	if deadlocks == 0 {
		t.Error("no seed of 20 found the reader to reader deadlock")
	}
}

// TestMutexReadWriteStress mixes readers and writers of a list in a real
// run; with -race it checks that readers sharing the value do not race
// with each other or with the write back.
func TestMutexReadWriteStress(t *testing.T) {
	root := NewRootTask()
	s := NewScope(root)
	body := s.Body()
	m := Mutex_new[List[int64]](body, ListOf[int64]())
	for i := 0; i < 12; i++ {
		write := i%3 == 0
		Spawn(body, s, func(t *Task) Result[Unit, Never] {
			for j := 0; j < 300; j++ {
				if write {
					Mutex_with(t, m, func(t *Task, v *List[int64]) Result[Unit, Never] {
						v.M_push(t, int64(j))
						return unit()
					})
				} else {
					Mutex_with_read(t, m, func(t *Task, v List[int64]) Result[Unit, Never] {
						// a reader's own copy may grow without touching m
						w := v
						w.M_push(t, -1)
						if w.M_len(t) != v.M_len(t)+1 {
							panic("copy")
						}
						return unit()
					})
				}
			}
			return unit()
		}, false, true)
	}
	s.Close(nil, true, 0)
	if n := m.v.M_len(nil); n != 4*300 {
		t.Fatalf("len %d, want %d", n, 4*300)
	}
}
