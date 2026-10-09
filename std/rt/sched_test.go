package rt

import (
	"strings"
	"sync"
	"testing"
	"time"
)

// schedTrace runs three tasks that log and pass values through a channel
// in a scheduler world and returns the log: the interleaving.
func schedTrace(t *testing.T, seed int64) string {
	t.Helper()
	w := NewSchedWorld(seed)
	tk := w.NewTask()
	var mu sync.Mutex
	var log []string
	add := func(s string) {
		mu.Lock()
		log = append(log, s)
		mu.Unlock()
	}
	s := NewScope(tk)
	body := s.Body()
	ch := Chan_new[int64](body, 2)
	m := Mutex_new[int64](body, 0)
	for i := int64(0); i < 3; i++ {
		Spawn(body, s, func(t *Task) Result[Unit, Never] {
			for j := int64(0); j < 3; j++ {
				Mutex_with(t, m, func(t *Task, v *int64) Result[Unit, Never] {
					*v += i
					return Ok[Unit, Never](Unit{})
				})
				add(string(rune('a'+i)) + string(rune('0'+j)))
				Chan_send(t, ch, i)
			}
			return Ok[Unit, Never](Unit{})
		}, false, true)
	}
	for k := 0; k < 9; k++ {
		v := Chan_recv(body, ch)
		add("r" + string(rune('0'+v.V)))
	}
	s.Close(nil, true, 0)
	w.EndTask(tk)
	mu.Lock()
	defer mu.Unlock()
	return strings.Join(log, " ")
}

func TestSchedSameSeedSameInterleaving(t *testing.T) {
	distinct := map[string]bool{}
	for seed := int64(1); seed <= 6; seed++ {
		first := schedTrace(t, seed)
		for i := 0; i < 3; i++ {
			if again := schedTrace(t, seed); again != first {
				t.Fatalf("seed %d: %q then %q", seed, first, again)
			}
		}
		distinct[first] = true
	}
	if len(distinct) < 3 {
		t.Errorf("6 seeds gave %d interleavings", len(distinct))
	}
}

func TestSchedVirtualClock(t *testing.T) {
	w := NewSchedWorld(3)
	tk := w.NewTask()
	start := w.Now()
	s := NewScope(tk)
	body := s.Body()
	for i := 1; i <= 3; i++ {
		Spawn(body, s, func(t *Task) Result[Unit, Never] {
			w.Sleep(t, Duration(i)*Duration(time.Second))
			return Ok[Unit, Never](Unit{})
		}, false, true)
	}
	w.Advance(body, Duration(500*time.Millisecond))
	if got := w.Now() - start; got != Time(500*time.Millisecond) {
		t.Errorf("after advance: %v", Duration(got))
	}
	s.Close(nil, true, 0)
	if got := w.Now() - start; got != Time(3*time.Second) {
		t.Errorf("after the sleeps: %v", Duration(got))
	}
	w.EndTask(tk)
}

func TestSchedDeadlockCycle(t *testing.T) {
	w := NewSchedWorld(1)
	done := make(chan struct{})
	go func() {
		defer close(done)
		tk := w.NewTask()
		s := NewScope(tk)
		body := s.Body()
		a := Mutex_new[int64](body, 0)
		b := Mutex_new[int64](body, 0)
		lock2 := func(x, y *Mutex[int64]) func(*Task) Result[Unit, Never] {
			return func(t *Task) Result[Unit, Never] {
				Mutex_with(t, x, func(t *Task, _ *int64) Result[Unit, Never] {
					w.Sleep(t, 1)
					Mutex_with(t, y, func(*Task, *int64) Result[Unit, Never] { return Ok[Unit, Never](Unit{}) })
					return Ok[Unit, Never](Unit{})
				})
				return Ok[Unit, Never](Unit{})
			}
		}
		Spawn(body, s, lock2(a, b), false, true)
		Spawn(body, s, lock2(b, a), false, true)
		s.Close(nil, true, 0)
	}()
	select {
	case <-w.Stuck():
	case <-done:
		t.Fatal("the deadlock was not detected")
	case <-time.After(10 * time.Second):
		t.Fatal("no verdict")
	}
	f := w.DeadlockFault()
	text := f.Error()
	for _, want := range []string{"XO-F001: deadlock: a wait cycle of 2 tasks", "task 2 blocked in Mutex.with", "held by task 3", "task 3 blocked in Mutex.with", "held by task 2", "also blocked:", "test blocked in scope end"} {
		if !strings.Contains(text, want) {
			t.Errorf("report lacks %q:\n%s", want, text)
		}
	}
}

func TestFindCycle(t *testing.T) {
	cases := []struct {
		edges [][]int
		want  []int
	}{
		{[][]int{{1}, {2}, {}}, nil},
		{[][]int{{1}, {0}}, []int{0, 1}},
		{[][]int{{1}, {2}, {1}}, []int{1, 2}},
		{[][]int{{0}}, []int{0}},
		{[][]int{{}, {2}, {3}, {1}}, []int{1, 2, 3}},
	}
	for _, c := range cases {
		got := findCycle(c.edges)
		if sprintInts(got) != sprintInts(c.want) {
			t.Errorf("findCycle(%v) = %v, want %v", c.edges, got, c.want)
		}
	}
}

func sprintInts(xs []int) string {
	var b strings.Builder
	for i, x := range xs {
		if i > 0 {
			b.WriteByte(' ')
		}
		b.WriteString(string(rune('0' + x)))
	}
	return b.String()
}

// TestMonitorDeadlockAndDump runs real (not test world) tasks: one blocked
// on a channel, one on a mutex held by the other, the root at the end of
// the scope. The dump shows each, and the monitor reports the deadlock.
func TestMonitorDeadlockAndDump(t *testing.T) {
	root := NewRootTask()
	mon := &monitor{quiet: 50 * time.Millisecond, root: root.node}
	reported := make(chan *FaultErr, 1)
	mon.report = func(f *FaultErr) { reported <- f }
	root.th.mon = mon
	mon.started()
	s := NewScope(root)
	body := s.Body()
	ch := Chan_new[int64](body, 0)
	m := Mutex_new[int64](body, 0)
	held := Chan_new[int64](body, 1)
	Spawn(body, s, func(t *Task) Result[Unit, Never] {
		Mutex_with(t, m, func(t *Task, _ *int64) Result[Unit, Never] {
			Chan_send(t, held, 1)
			Chan_recv(t, ch)
			return Ok[Unit, Never](Unit{})
		})
		return Ok[Unit, Never](Unit{})
	}, false, true)
	Spawn(body, s, func(t *Task) Result[Unit, Never] {
		Chan_recv(t, held)
		Mutex_with(t, m, func(*Task, *int64) Result[Unit, Never] { return Ok[Unit, Never](Unit{}) })
		return Ok[Unit, Never](Unit{})
	}, false, true)
	closed := make(chan struct{})
	go func() {
		defer close(closed)
		defer func() { recover() }()
		s.Close(nil, true, 0)
	}()
	var f *FaultErr
	select {
	case f = <-reported:
	case <-time.After(10 * time.Second):
		t.Fatal("no deadlock reported")
	}
	if f.Code != "XO-F001" || !strings.Contains(f.Detail, "blocked in receive") || !strings.Contains(f.Detail, "blocked in Mutex.with") {
		t.Errorf("report:\n%s", f.Error())
	}
	text := dumpText(root.node)
	for _, want := range []string{"3 tasks", "main: waiting for its tasks", "task 2", "blocked in receive", "task 3", "held by task 2", "end of xo dump"} {
		if !strings.Contains(text, want) {
			t.Errorf("dump lacks %q:\n%s", want, text)
		}
	}
	// let the tasks go: cancel the scope's tasks so Close returns
	s.tasksNode.cancel()
	select {
	case <-closed:
	case <-time.After(10 * time.Second):
		t.Fatal("the scope did not close after cancellation")
	}
}
