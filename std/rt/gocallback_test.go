package rt

import (
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"
)

// TestCurG: curG is stable on one goroutine and differs between live
// goroutines (decision 0067: it tells a synchronous Go callback from one
// on another goroutine).
func TestCurG(t *testing.T) {
	g := curG()
	if g == 0 || curG() != g {
		t.Fatalf("curG = %x, then %x", g, curG())
	}
	const n = 8
	ids := make(chan uintptr, n)
	stop := make(chan struct{})
	for i := 0; i < n; i++ {
		go func() {
			ids <- curG()
			<-stop
		}()
	}
	seen := map[uintptr]bool{g: true}
	for i := 0; i < n; i++ {
		id := <-ids
		if seen[id] {
			t.Errorf("two live goroutines share curG %x", id)
		}
		seen[id] = true
	}
	close(stop)
}

// TestGoCallbackForeign: a callback called on another goroutine while its
// task is in a Go call runs as a child task with its own thread and held
// set; one called on the task's goroutine during a Go call runs as the
// task (decision 0067).
func TestGoCallbackForeign(t *testing.T) {
	tk := NewRootTask()
	tk.held = new(heldSet)
	cb := NewGoCallback(tk)
	GoEnter(tk)
	done := make(chan *Task)
	go func() {
		ctk := cb.Begin(nil)
		defer cb.End(ctk)
		done <- ctk
	}()
	ctk := <-done
	if ctk == tk || ctk.th == tk.th || ctk.held == tk.held || ctk.node.parent != tk.node {
		t.Errorf("foreign call shares the task: %p %p", ctk, tk)
	}
	if got := cb.Begin(nil); got != tk {
		t.Errorf("synchronous call got another task")
	} else {
		cb.End(got)
	}
	GoExit(tk, "pkg.F")
	if n := tk.th.inGo.Load(); n != 0 {
		t.Errorf("inGo = %d after the call", n)
	}
	// after the Go call, a call on the task's goroutine is not synchronous
	// (a stored callback called later): it gets a task of its own
	late := cb.Begin(nil)
	if late == tk {
		t.Errorf("call outside a Go call shares the task")
	}
	cb.End(late)
}

// TestMonitorGoCallDeadlock: a real run's monitor reports a task blocked
// in a Go call (sync.WaitGroup.Wait) for a foreign callback that waits
// for a Mutex the task holds (XO-F001 with the Go call edge), and does
// not report the same shape when the Go call sleeps (decision 0073).
func TestMonitorGoCallDeadlock(t *testing.T) {
	cases := []struct {
		name string
		wait func(*sync.WaitGroup)
		want bool
	}{
		{"wait", func(wg *sync.WaitGroup) { wg.Wait() }, true},
		{"sleep", func(*sync.WaitGroup) { Go_test_sleep(400 * time.Millisecond) }, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := NewRootTask()
			mon := &monitor{quiet: 50 * time.Millisecond, root: root.node}
			reported := make(chan *FaultErr, 1)
			mon.report = func(f *FaultErr) { reported <- f }
			root.th.mon = mon
			mon.started()
			m := Mutex_new[int64](root, 0)
			var wg sync.WaitGroup
			wg.Add(1)
			done := make(chan struct{})
			go func() {
				defer close(done)
				Mutex_with(root, m, func(tk *Task, _ *int64) Result[Unit, Never] {
					cb := NewGoCallback(tk)
					GoEnter(tk)
					defer GoExit(tk, "test")
					go func() {
						defer wg.Done()
						ctk := cb.Begin(nil)
						defer cb.End(ctk)
						Mutex_with(ctk, m, func(*Task, *int64) Result[Unit, Never] { return Ok[Unit, Never](Unit{}) })
					}()
					tc.wait(&wg)
					return Ok[Unit, Never](Unit{})
				})
			}()
			select {
			case f := <-reported:
				if !tc.want {
					t.Fatalf("reported:\n%s", f.Error())
				}
				for _, want := range []string{"XO-F001", "a wait cycle of 2 tasks", "main blocked in Go call", "for callback 2", "callback 2 blocked in Mutex.with", "held by main"} {
					if !strings.Contains(f.Error(), want) {
						t.Errorf("report lacks %q:\n%s", want, f.Error())
					}
				}
				// let the callback unwind so the goroutines end
				root.node.cancel()
			case <-done:
				if tc.want {
					t.Fatal("the Go call returned")
				}
			case <-time.After(10 * time.Second):
				t.Fatal("nothing reported")
			}
			select {
			case <-done:
			case <-time.After(10 * time.Second):
				t.Fatal("the task did not end")
			}
		})
	}
}

// TestGoParked classifies goroutines of a stack dump for goQuiet.
func TestGoParked(t *testing.T) {
	rt := rtPrefix
	cases := []struct {
		name, g string
		want    bool
	}{
		{"xo wait", "goroutine 7 [chan receive]:\n" + rt + "(*lock).wait(0x1)\n\tx/rt/sync.go:1 +0x1\nxoprog/m.F_f()\n\tmain.xo:3 +0x1", true},
		{"go call in sync wait", "goroutine 8 [sync.WaitGroup.Wait]:\nsync.runtime_SemacquireWaitGroup(0x0)\n\truntime/sema.go:114 +0x38\nsync.(*WaitGroup).Wait(0x1)\n\tsync/waitgroup.go:1 +0x1\n" + rt + "Go_sync_WaitGroup.M_wait()\n\tx/rt/b.go:1 +0x1\nxoprog/m.F_f()\n\tmain.xo:5 +0x1", true},
		{"go call on a channel", "goroutine 9 [chan receive]:\nnet/http.(*Client).Do(0x1)\n\tnet/http/client.go:1 +0x1\n" + rt + "Go_net_http_Client.M_do()\n\tx/rt/b.go:1 +0x1\nxoprog/m.F_f()\n\tmain.xo:5 +0x1", false},
		{"go call reading the network", "goroutine 10 [IO wait]:\ninternal/poll.runtime_pollWait(0x1)\n\truntime/netpoll.go:1 +0x1\nnet.(*conn).Read(0x1)\n\tnet/net.go:1 +0x1\n" + rt + "Go_net_Conn.M_read()\n\tx/rt/b.go:1 +0x1\nxoprog/m.F_f()\n\tmain.xo:5 +0x1", false},
		{"go call sleeping", "goroutine 11 [sleep]:\ntime.Sleep(0x1)\n\truntime/time.go:1 +0x1\n" + rt + "Go_time_sleep()\n\tx/rt/b.go:1 +0x1\nxoprog/m.F_f()\n\tmain.xo:5 +0x1", false},
		{"foreign goroutine", "goroutine 12 [sync.Mutex.Lock]:\nsync.(*Mutex).Lock(0x1)\n\tsync/mutex.go:1 +0x1\nmain.worker()\n\tw.go:1 +0x1", false},
		{"go call sleeping inside Mutex.with", "goroutine 15 [sleep]:\ntime.Sleep(0x1)\n\truntime/time.go:1 +0x1\n" + rt + "Go_time_sleep()\n\tx/rt/b.go:1 +0x1\nxoprog/m.F_f.func1()\n\tmain.xo:5 +0x1\n" + rt + "Mutex_with[...]()\n\tx/rt/sync.go:1 +0x1\nxoprog/m.F_f()\n\tmain.xo:4 +0x1", false},
		{"runtime watcher sleeping", "goroutine 16 [sleep]:\ntime.Sleep(0x1)\n\truntime/time.go:1 +0x1\n" + rt + "(*replayer).watch()\n\tx/rt/replay.go:1 +0x1", true},
		{"running", "goroutine 13 [runnable]:\n" + rt + "x()\n\tx/rt/a.go:1 +0x1", false},
		{"signals", "goroutine 14 [syscall]:\nos/signal.signal_recv()\n\truntime/sigqueue.go:149 +0x108\nos/signal.loop()\n\tos/signal/signal_unix.go:23 +0x1c", true},
	}
	for _, c := range cases {
		if got := goParked(c.g); got != c.want {
			t.Errorf("%s: goParked = %v, want %v", c.name, got, c.want)
		}
	}
}

// schedCallbackOrder runs a test world task that hands three callbacks to
// goroutines of its own, as sync.WaitGroup.Go does, and logs beside them
// at scheduling points; it returns the log.
func schedCallbackOrder(seed int64) string {
	w := NewSchedWorld(seed)
	tk := w.NewTask()
	var mu sync.Mutex
	var log []string
	add := func(s string) {
		mu.Lock()
		log = append(log, s)
		mu.Unlock()
	}
	var wg sync.WaitGroup
	for i := 0; i < 3; i++ {
		cb := NewGoCallback(tk)
		GoEnter(tk)
		wg.Add(1)
		go func() {
			defer wg.Done()
			ctk := cb.Begin(func() []any { return []any{int64(i)} })
			defer cb.End(ctk)
			for j := 0; j < 3; j++ {
				add(fmt.Sprintf("cb%d.%d", i, j))
				ctk.Yield()
			}
		}()
		GoExit(tk, "go")
		add(fmt.Sprintf("main%d", i))
		tk.Yield()
	}
	GoEnter(tk)
	wg.Wait()
	GoExit(tk, "wait")
	w.EndTask(tk)
	return strings.Join(log, " ")
}

// TestSchedGoCallbackSeed: under the deterministic scheduler, callbacks
// that Go calls on goroutines of its own interleave with the task as the
// seed says: the same seed gives the same order every run (decision
// 0073).
func TestSchedGoCallbackSeed(t *testing.T) {
	orders := map[string]bool{}
	for seed := int64(1); seed <= 4; seed++ {
		first := schedCallbackOrder(seed)
		orders[first] = true
		for i := 0; i < 10; i++ {
			if got := schedCallbackOrder(seed); got != first {
				t.Fatalf("seed %d run %d: %s\nfirst: %s", seed, i+2, got, first)
			}
		}
	}
	if len(orders) < 2 {
		t.Errorf("4 seeds gave one order: %v", orders)
	}
}

// Go_test_sleep stands for a Go adapter (named Go_...) that sleeps in Go.
//
//go:noinline
func Go_test_sleep(d time.Duration) { time.Sleep(d) }
